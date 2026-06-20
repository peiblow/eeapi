package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/peiblow/eeapi/internal/blocks"
	"github.com/peiblow/eeapi/internal/config"
	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/keys"
	"github.com/peiblow/eeapi/internal/repository"
	"github.com/peiblow/eeapi/internal/schema"
	"github.com/peiblow/eeapi/internal/store"
	"github.com/peiblow/eeapi/internal/swp"
)

type ContractService interface {
	DeployContract(ctx context.Context, in *DeployInput) (*DeployResult, error)
	ExecuteContract(ctx context.Context, contractID string, payload *swp.ExecPayload) (*ExecuteResult, error)
	TraceContext(ctx context.Context, contextID string) (*TraceOutput, error)
}

type contractService struct {
	swpClient    *swp.SwpClient
	db           repository.ContractRepository
	blockDB      repository.BlockRepository
	behaviourSrv BehaviourService
	privKey      []byte
	pubKey       []byte
	locker       *config.ContractLocker
	artifactDir  string
}

func NewContractService(swpClient *swp.SwpClient, db *postgres.DB, behaviourSrv BehaviourService, privKey []byte, pubKey []byte, locker *config.ContractLocker, artifactDir string) ContractService {
	return &contractService{
		swpClient:    swpClient,
		db:           repository.NewPsqlContractRepository(db),
		blockDB:      repository.NewPsqlBlockRepository(db),
		behaviourSrv: behaviourSrv,
		privKey:      privKey,
		pubKey:       pubKey,
		locker:       locker,
		artifactDir:  artifactDir,
	}
}

// DeployInput is a prebuilt contract submitted to /deploy. The artifact is
// compiled by the synx CLI; EEAPI no longer compiles.
type DeployInput struct {
	ContractName string
	Version      string
	Owner        string
	Artifact     []byte // the marshaled ContractArtifact (.snxb)
	AgentHash    string
	AgentName    string
	AgentVersion string
}

type DeployResult struct {
	ContractHash    string
	ContractName    string
	ContractOwner   string
	ContractVersion string
}

type ArtifactMetadata struct {
	ConstPool    []interface{}          `json:"const_pool"`
	Functions    map[string]interface{} `json:"functions"`
	FunctionName map[int]string         `json:"function_name"`
	Types        map[string]interface{} `json:"types"`
	InitStorage  map[string]interface{} `json:"init_storage"`
}

type ExecuteResult struct {
	BlockHash    string
	Events       []interface{}
	Response     *swp.WireResponse
	FailedReason string
}

type TraceStep struct {
	Function      string `json:"function"`
	ExecutionHash string `json:"executionHash"`
	ParentHash    string `json:"parentHash"`
	ContractID    string `json:"contract"`
	Timestamp     int64  `json:"executedAt"`
}

type TraceOutput struct {
	ContextID string      `json:"contextId"`
	Status    string      `json:"status"`
	Steps     []TraceStep `json:"steps"`
}

type EnrichedJournal struct {
	Events []interface{}          `json:"events"`
	Args   map[string]interface{} `json:"args"`
	Trace  []swp.TraceLog         `json:"trace"`
	Audit  []swp.AuditLog         `json:"audit"`
}

func (s *contractService) DeployContract(ctx context.Context, in *DeployInput) (*DeployResult, error) {
	createdAt := time.Now().UTC()
	hashInput := fmt.Sprintf("%v:%v:%v:%v", in.Owner, in.ContractName, in.Version, createdAt.UnixMilli())
	hashBytes := sha256.Sum256([]byte(hashInput))
	hash := "0x" + hex.EncodeToString(hashBytes[:])

	// The artifact is already compiled by the CLI — validate it parses before
	// we store it; the bytes themselves go to the content store, not the DB.
	var probe swp.ArtifactMetadata
	if err := json.Unmarshal(in.Artifact, &probe); err != nil {
		return nil, fmt.Errorf("invalid artifact: %w", err)
	}

	// 1) Write the artifact to the shared content store first (file before DB
	//    row, so we never have a contract row pointing at a missing artifact).
	if err := store.PutArtifact(s.artifactDir, hash, in.Artifact); err != nil {
		return nil, err
	}
	slog.Info("Artifact stored", "hash", hash, "dir", s.artifactDir)

	// 2) Persist metadata. The artifact blob is still written to Postgres
	//    because ExecuteContract currently loads it from there; once exec reads
	//    the .snxb file, the contract_artifacts blob can be dropped.
	if err := s.db.SaveAgentMeta(ctx, &swp.AgentMeta{
		Hash:    in.AgentHash,
		Name:    in.AgentName,
		Version: in.AgentVersion,
	}); err != nil {
		return nil, err
	}

	if err := s.db.SaveContractArtifact(ctx, hash, in.AgentHash); err != nil {
		return nil, err
	}

	if err := s.db.SaveContract(ctx, &schema.Contract{
		Name:         in.ContractName,
		Owner:        in.Owner,
		ArtifactHash: hash,
		CreatedAt:    createdAt.UnixMilli(),
	}); err != nil {
		return nil, err
	}
	slog.Info("Contract deployed successfully", "contract_hash", hash, "name", in.ContractName)

	return &DeployResult{
		ContractHash:    hash,
		ContractName:    in.ContractName,
		ContractOwner:   in.Owner,
		ContractVersion: in.Version,
	}, nil
}

func (s *contractService) ExecuteContract(ctx context.Context, contractID string, payload *swp.ExecPayload) (*ExecuteResult, error) {
	s.locker.Lock(contractID)
	defer s.locker.Unlock(contractID)

	executionStart := time.Now().UTC()
	auditLogs := []swp.AuditLog{{
		Time:  executionStart.Format("15:04:05.000"),
		Event: "Execution requested",
		Actor: payload.CallerID,
	}}
	traceLogs := []swp.TraceLog{}

	slog.Info("Executing contract", "contract_id", contractID, "function", payload.Function)
	timestamp := time.Now().UTC().UnixMilli()

	// ── contract.load ─────────────────────────────────────────────────────────
	stepStart := time.Now()
	contract, err := s.db.GetContractByID(ctx, contractID)
	if err != nil {
		return &ExecuteResult{
			BlockHash: "",
			Response:  &swp.WireResponse{Type: swp.EXEC, ID: uuid.New().String(), Success: false, Error: "Failed to retrieve contract: " + err.Error()},
		}, err
	}
	traceLogs = append(traceLogs, swp.TraceLog{Step: "contract.load", Msg: fmt.Sprintf("Contract loaded: %s", contractID), DurationMs: time.Since(stepStart).Milliseconds()})
	auditLogs = append(auditLogs, swp.AuditLog{Time: time.Now().UTC().Format("15:04:05.000"), Event: fmt.Sprintf("Contract loaded: %s", contract.ArtifactHash), Actor: "system"})

	// ── artifact.load ─────────────────────────────────────────────────────────
	stepStart = time.Now()
	artifactBlob, err := store.GetArtifact(s.artifactDir, contract.ArtifactHash)
	if err != nil {
		slog.Error("Failed to retrieve contract artifact", "artifact_hash", contract.ArtifactHash, "error", err)
		return &ExecuteResult{
			BlockHash: "",
			Response:  &swp.WireResponse{Type: swp.EXEC, ID: uuid.New().String(), Success: false, Error: "Failed to retrieve contract artifact: " + err.Error()},
		}, err
	}
	var artifact swp.ArtifactMetadata
	if err := json.Unmarshal(artifactBlob, &artifact); err != nil {
		slog.Error("Failed to parse contract artifact", "artifact_hash", contract.ArtifactHash, "error", err)
		return &ExecuteResult{
			BlockHash: "",
			Response:  &swp.WireResponse{Type: swp.EXEC, ID: uuid.New().String(), Success: false, Error: "Corrupt contract artifact: " + err.Error()},
		}, err
	}
	traceLogs = append(traceLogs, swp.TraceLog{Step: "artifact.load", Msg: fmt.Sprintf("Artifact loaded: %s", contract.ArtifactHash), DurationMs: time.Since(stepStart).Milliseconds()})

	argsBytes, _ := json.Marshal(payload.Args)
	sum := sha256.Sum256(argsBytes)
	payloadHash := hex.EncodeToString(sum[:])

	// if score, err := s.behaviourSrv.ProcessEvent(ctx, schema.BehaviourEvent{
	// 	ContractId:  contractID,
	// 	AgentId:     payload.CallerID,
	// 	TotalTools:  len(artifact.Functions),
	// 	Tool:        payload.Function,
	// 	PayloadHash: payloadHash,
	// 	Denied:      false,
	// 	Timestamp:   timestamp,
	// }); err != nil {
	// 	slog.Error("behaviour gate failed, allowing execution", "contract_id", contractID, "error", err)
	// } else if score != nil && score.Score != nil {
	// 	auditLogs = append(auditLogs, swp.AuditLog{Time: time.Now().UTC().Format("15:04:05.000"), Event: fmt.Sprintf("Behaviour score calculated: %.4f (%s)", score.Score.RiskScore, score.Score.RiskLevel), Actor: "behaviour_service"})

	// 	payload.Args["risk_score"] = score.Score.RiskScore
	// 	payload.Args["risk_level"] = score.Score.RiskLevel

	// 	// if score.Score.RiskLevel == "CRITICAL" {
	// 	// 	auditLogs = append(auditLogs, swp.AuditLog{Time: time.Now().UTC().Format("15:04:05.000"), Event: "Execution denied due to high risk score", Actor: "behaviour_service"})
	// 	// 	return &ExecuteResult{
	// 	// 		BlockHash:    "",
	// 	// 		Events:       nil,
	// 	// 		Response:     &swp.WireResponse{Type: swp.EXEC, ID: uuid.New().String(), Success: false, Error: "Execution denied due to high risk score"},
	// 	// 		FailedReason: "high risk score",
	// 	// 	}, nil
	// 	// }
	// }

	decision, err := s.behaviourSrv.ProcessEvent(ctx, schema.BehaviourEvent{
		ContractId:  contractID,
		AgentId:     payload.CallerID,
		TotalTools:  len(artifact.Functions),
		Tool:        payload.Function,
		PayloadHash: payloadHash,
		Denied:      false,
		Timestamp:   timestamp,
	})

	if err != nil {
		slog.Error("behaviour gate failed, allowing execution", "contract_id", contractID, "error", err)
	}

	// ── VVM execute ───────────────────────────────────────────────────────────
	if payload.ContextId != "" {
		finalBlock, err := s.blockDB.GetFinalBlockByContextID(ctx, payload.ContextId)
		if err == nil && finalBlock != nil {
			reason := ""
			if finalBlock.FunctionName == "pow" {
				reason = "context already finalized with 'pow'"
			} else if finalBlock.Status == "rejected" {
				reason = "context already rejected — start a new context"
			}
			if reason != "" {
				return &ExecuteResult{
					BlockHash: "",
					Response:  &swp.WireResponse{Type: swp.EXEC, ID: uuid.New().String(), Success: false, Error: reason},
				}, fmt.Errorf("%s", reason)
			}
		}
	}

	msg := swp.WireMesage{
		Type: swp.EXEC,
		ID:   uuid.New().String(),
		Data: swp.ExecPayload{
			ContractArtifact: artifact,
			ArtifactHash:     contract.ArtifactHash,
			Function:         payload.Function,
			Args:             payload.Args,
			CoreArgs: map[string]any{
				"riskScore":  decision.Score.RiskScore,
				"riskLevel":  decision.Score.RiskLevel,
				"denialRate": decision.Score.DenialRate,
			},
		},
	}

	stepStart = time.Now()
	var resp swp.WireResponse
	if err := s.swpClient.Send(msg, &resp); err != nil {
		return &ExecuteResult{
			BlockHash: "",
			Response:  &swp.WireResponse{Type: swp.EXEC, ID: msg.ID, Success: false, Error: "Failed to execute contract: " + err.Error()},
		}, err
	}
	execDuration := time.Since(stepStart).Milliseconds()

	previousBlock, err := s.blockDB.GetLastContractBlock(ctx, contractID)
	if err != nil {
		slog.Error("Failed to retrieve last block", "error", err)
		return nil, err
	}

	// ── journal (parse antes do status para inspecionar events) ───────────────
	var journalEvents []interface{}
	var artifactHash string
	if resp.Success {
		var respData swp.ExecResponse
		if err := json.Unmarshal(resp.Data, &respData); err != nil {
			return nil, err
		}
		journalEvents = respData.Journal
		artifactHash = respData.ArtifactHash
	}

	// ── determina status ──────────────────────────────────────────────────────
	// VVM Success=true significa apenas que a função executou sem panic. Um
	// contrato com try/catch que captura um require e emite "*Rejected" também
	// volta como Success=true. Por isso, qualquer evento *Rejected no journal
	// equivale a uma rejeição do contrato.
	blockStatus := "approved"
	failedReason := ""

	if !resp.Success {
		blockStatus = "rejected"
		failedReason = extractFailedReason(resp.ErrorString())
	} else if reason := rejectionFromEvents(journalEvents); reason != "" {
		blockStatus = "rejected"
		failedReason = reason
	}

	if blockStatus == "rejected" {
		auditLogs = append(auditLogs, swp.AuditLog{Time: time.Now().UTC().Format("15:04:05.000"), Event: fmt.Sprintf("Function rejected: %s — %s", payload.Function, failedReason), Actor: "vvm"})
		traceLogs = append(traceLogs, swp.TraceLog{Step: payload.Function, Msg: fmt.Sprintf("Function '%s' rejected: %s", payload.Function, failedReason), DurationMs: execDuration})

		if decision != nil && decision.EventId != 0 {
			eventId := decision.EventId
			if err := s.behaviourSrv.MarkEventAsDenied(ctx, eventId); err != nil {
				slog.Error("Error to MARK EVENT as DENIED", "EventID", eventId)
			}
		}
	} else {
		auditLogs = append(auditLogs, swp.AuditLog{Time: time.Now().UTC().Format("15:04:05.000"), Event: fmt.Sprintf("Function executed: %s", payload.Function), Actor: "vvm"})
		traceLogs = append(traceLogs, swp.TraceLog{Step: payload.Function, Msg: fmt.Sprintf("Function '%s' executed successfully", payload.Function), DurationMs: execDuration})
	}

	enrichedJournal := EnrichedJournal{
		Events: journalEvents,
		Args:   payload.Args,
		Trace:  traceLogs,
		Audit:  auditLogs,
	}

	journalBytes, err := json.Marshal(enrichedJournal)
	if err != nil {
		slog.Error("Failed to marshal journal", "error", err)
		return nil, err
	}

	// ── hashes + assinatura ───────────────────────────────────────────────────
	journalHashRaw := sha256.Sum256(append(journalBytes, []byte(fmt.Sprintf("%d", timestamp))...))
	journalHash := "0x" + hex.EncodeToString(journalHashRaw[:])

	blockData := fmt.Sprintf("%d|%s|%s|%s|%s|%s",
		timestamp, previousBlock.Hash, journalHash, contractID, payload.Function, artifactHash,
	)
	blockHashRaw := sha256.Sum256([]byte(blockData))
	blockHash := "0x" + hex.EncodeToString(blockHashRaw[:])

	encryptedJournal, err := keys.EncryptJournal(journalBytes, s.privKey)
	if err != nil {
		slog.Error("Failed to encrypt journal", "error", err)
		return nil, err
	}

	signature := ed25519.Sign(s.privKey, blockHashRaw[:])

	// ── salva block (sempre — sucesso ou falha do VVM) ────────────────────────
	block := &schema.Block{
		BlockIndex:   previousBlock.BlockIndex + 1,
		Hash:         blockHash,
		Timestamp:    timestamp,
		PreviousHash: previousBlock.Hash,
		JournalHash:  journalHash,
		Signature:    signature,
		ContractID:   contractID,
		FunctionName: payload.Function,
		Journal:      encryptedJournal,
		ContextID:    payload.ContextId,
		Status:       blockStatus,
		FailedReason: failedReason,
	}

	if blockStatus == "approved" {
		if err := blocks.VerifyBlock(*previousBlock, *block, journalBytes, s.pubKey); err != nil {
			return nil, err
		}
		slog.Info("Block verification successful", "block_hash", block.Hash)
	}

	slog.Info("Saving execution block", "block_hash", blockHash, "status", blockStatus, "function", payload.Function)

	if err := s.blockDB.SaveBlock(ctx, block); err != nil {
		slog.Error("Failed to save execution block", "error", err)
		return nil, err
	}
	slog.Info("Execution block saved successfully", "block_hash", block.Hash)

	// ── retorno ───────────────────────────────────────────────────────────────
	// VVM crashou: sem events, sem journal.
	if !resp.Success {
		return &ExecuteResult{
			BlockHash:    blockHash,
			Events:       nil,
			Response:     &swp.WireResponse{Type: swp.EXEC, ID: msg.ID, Success: false, Error: resp.Error},
			FailedReason: failedReason,
		}, nil
	}

	// VVM executou: ainda assim podem ter sido emitidos eventos *Rejected.
	// Propaga blockStatus para Response.Success para clientes que olham só o
	// flag (a EEAPI continua enviando events na resposta HTTP em ambos os casos).
	finalSuccess := blockStatus == "approved"
	return &ExecuteResult{
		BlockHash:    blockHash,
		Events:       journalEvents,
		Response:     &swp.WireResponse{Type: swp.EXEC, ID: msg.ID, Success: finalSuccess, Data: resp.Data, Error: resp.Error},
		FailedReason: failedReason,
	}, nil
}

func (s *contractService) TraceContext(ctx context.Context, contextID string) (*TraceOutput, error) {
	blocks, err := s.blockDB.GetBlocksByContextID(ctx, contextID)
	if err != nil {
		return nil, err
	}

	steps := make([]TraceStep, len(blocks))
	for i, block := range blocks {
		steps[i] = TraceStep{
			Function:      block.FunctionName,
			ExecutionHash: block.Hash,
			ParentHash:    block.PreviousHash,
			ContractID:    block.ContractID,
			Timestamp:     block.Timestamp,
		}
	}

	return &TraceOutput{
		ContextID: contextID,
		Status:    "COMPLETED",
		Steps:     steps,
	}, nil
}

func extractFailedReason(rawError string) string {
	s := rawError
	if idx := strings.Index(s, "[execution error: "); idx != -1 {
		s = s[idx+len("[execution error: "):]
		s = strings.TrimSuffix(strings.TrimSpace(s), "]")
	}
	s = strings.TrimPrefix(s, "map[Value:")
	s = strings.TrimSuffix(s, "]")

	return strings.TrimSpace(s)
}

// rejectionFromEvents inspects the journal for any "*Rejected" event emitted
// by a contract's try/catch path and returns its reason (or the event Type as
// fallback). Returns "" when no rejection event is present.
func rejectionFromEvents(events []interface{}) string {
	for _, ev := range events {
		evMap, ok := ev.(map[string]interface{})
		if !ok {
			continue
		}
		evType, _ := evMap["Type"].(string)
		if !strings.HasSuffix(evType, "Rejected") {
			continue
		}
		if payload, ok := evMap["Payload"].(map[string]interface{}); ok {
			if data, ok := payload["data"].(map[string]interface{}); ok {
				if reason, _ := data["reason"].(string); reason != "" {
					return reason
				}
			}
		}
		return evType
	}
	return ""
}
