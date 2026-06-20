package service

import (
	"context"
	"log/slog"
	"math"

	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/repository"
	"github.com/peiblow/eeapi/internal/schema"
)

type BehaviourService interface {
	ProcessEvent(ctx context.Context, event schema.BehaviourEvent) (*Decision, error)
	ComputeDrift(ctx context.Context, contractId string, agentId string) (float64, error)
	MarkEventAsDenied(ctx context.Context, eventId int64) error
}

type behaviourService struct {
	behaviourDb repository.BehaviourRepository
}

func NewBehaviourService(db *postgres.DB) BehaviourService {
	return &behaviourService{
		behaviourDb: repository.NewPsqlBehaviourRepository(db),
	}
}

type Decision struct {
	EventId     int64
	Action      string
	RuleHit     string
	ObserveOnly bool
	Score       *schema.BehaviourScore
}

func (s *behaviourService) ProcessEvent(ctx context.Context, event schema.BehaviourEvent) (*Decision, error) {
	slog.Info("Processing behaviour event",
		"contract_id", event.ContractId, "agent_id", event.AgentId,
		"tool", event.Tool, "denied", event.Denied)

	if err := s.behaviourDb.RecordEvent(ctx, &event); err != nil {
		slog.Error("Failed to record behaviour event",
			"contract_id", event.ContractId, "agent_id", event.AgentId, "error", err)
		return nil, err
	}

	vector, err := s.behaviourDb.BuildVector(ctx, event.ContractId, event.AgentId)
	if err != nil {
		slog.Error("Failed to build behaviour vector",
			"contract_id", event.ContractId, "agent_id", event.AgentId, "error", err)
		return nil, err
	}

	if event.TotalTools > 0 {
		vector[2] = vector[2] / float64(event.TotalTools)
	}

	short, err := s.behaviourDb.GetBaseline(ctx, event.ContractId, event.AgentId, "short")
	if err != nil {
		slog.Error("Failed to load baseline", "kind", "short",
			"contract_id", event.ContractId, "agent_id", event.AgentId, "error", err)
		return nil, err
	}

	long, err := s.behaviourDb.GetBaseline(ctx, event.ContractId, event.AgentId, "long")
	if err != nil {
		slog.Error("Failed to load baseline", "kind", "long",
			"contract_id", event.ContractId, "agent_id", event.AgentId, "error", err)
		return nil, err
	}

	NMin := 10
	if short.N < NMin {
		short.UpdateWelford(vector)
		if err := s.behaviourDb.UpsertBaseline(ctx, short); err != nil {
			slog.Error("Failed to upsert baseline", "kind", "short",
				"contract_id", event.ContractId, "agent_id", event.AgentId, "error", err)
			return nil, err
		}

		long.UpdateWelford(vector)
		if err := s.behaviourDb.UpsertBaseline(ctx, long); err != nil {
			slog.Error("Failed to upsert baseline", "kind", "long",
				"contract_id", event.ContractId, "agent_id", event.AgentId, "error", err)
			return nil, err
		}

		slog.Info("Behaviour baseline learning (observe-only)",
			"contract_id", event.ContractId, "agent_id", event.AgentId,
			"n", short.N, "n_min", NMin)
		return &Decision{
			EventId:     event.ID,
			Action:      "ALLOW",
			RuleHit:     "insufficient_data",
			ObserveOnly: true,
			Score:       nil,
		}, nil
	}

	scoreRisk := computeRiskLevel(vector, short.MU, short.Sigma())
	if err := s.behaviourDb.RecordScore(ctx, &schema.BehaviourScore{
		ContractId: event.ContractId,
		AgentId:    event.AgentId,
		Vector:     roundSlice(vector, 2),
		ZVector:    roundSlice(scoreRisk.ZVector, 2),
		RiskScore:  round(scoreRisk.RiskScore, 2),
		RiskLevel:  scoreRisk.RiskLevel,
		TS:         event.Timestamp,
	}); err != nil {
		slog.Error("Failed to record behaviour score",
			"contract_id", event.ContractId, "agent_id", event.AgentId, "error", err)
		return nil, err
	}

	logScore := slog.Info
	if scoreRisk.RiskLevel == "HIGH" || scoreRisk.RiskLevel == "CRITICAL" {
		logScore = slog.Warn
	}
	logScore("Behaviour score computed",
		"contract_id", event.ContractId, "agent_id", event.AgentId,
		"risk_score", scoreRisk.RiskScore, "risk_level", scoreRisk.RiskLevel)

	short.UpdateWelford(vector)
	if !long.Frozen {
		long.Frozen = true
		if err := s.behaviourDb.UpsertBaseline(ctx, long); err != nil {
			slog.Error("Failed to freeze long baseline",
				"contract_id", event.ContractId, "agent_id", event.AgentId, "error", err)
			return nil, err
		}
		slog.Info("Long baseline frozen (plan reference fixed)",
			"contract_id", event.ContractId, "agent_id", event.AgentId, "n", long.N)
	}

	if err := s.behaviourDb.UpsertBaseline(ctx, short); err != nil {
		slog.Error("Failed to upsert baseline", "kind", "short",
			"contract_id", event.ContractId, "agent_id", event.AgentId, "error", err)
		return nil, err
	}

	return &Decision{
		EventId:     event.ID,
		Action:      "ALLOW",
		RuleHit:     "",
		ObserveOnly: false,
		Score:       scoreRisk,
	}, nil
}

func (s *behaviourService) ComputeDrift(ctx context.Context, contractId string, agentId string) (float64, error) {
	N_MIN := 30

	short, err := s.behaviourDb.GetBaseline(ctx, contractId, agentId, "short")
	if err != nil {
		return float64(0), err
	}

	long, err := s.behaviourDb.GetBaseline(ctx, contractId, agentId, "long")
	if err != nil {
		return float64(0), err
	}

	if !long.Frozen {
		return 0, nil
	}

	if short.N < N_MIN {
		return 0, nil
	}

	score := computeRiskLevel(short.MU, long.MU, long.Sigma())
	if err := s.behaviourDb.RecordDrift(ctx, &schema.BehaviourDrift{
		ContractId:    contractId,
		AgentId:       agentId,
		DriftDistance: score.RiskScore,
	}); err != nil {
		slog.Error("Error to save Record Drift", "contract_id", contractId)
	}

	return score.RiskScore, nil
}

func (s *behaviourService) MarkEventAsDenied(ctx context.Context, eventId int64) error {
	if err := s.behaviourDb.MarkEventAsDenied(ctx, eventId); err != nil {
		return err
	}

	return nil
}

func round(v float64, places int) float64 {
	f := math.Pow(10, float64(places))
	return math.Round(v*f) / f
}

func roundSlice(s []float64, places int) []float64 {
	out := make([]float64, len(s))
	for i, v := range s {
		out[i] = round(v, places)
	}
	return out
}

func computeRiskLevel(vector, baseline, baselineSigma []float64) *schema.BehaviourScore {
	zScore := make([]float64, len(vector))

	var sigmaFloor = []float64{1, 1, 0.05, 0.05, 0.05, 1}
	for i := range vector {
		sigma := math.Max(baselineSigma[i], sigmaFloor[i])
		zScore[i] = (vector[i] - baseline[i]) / sigma
	}

	riskScore := 0.0
	for _, z := range zScore {
		riskScore += z * z
	}
	riskScore = math.Sqrt(riskScore)

	return &schema.BehaviourScore{
		Vector:     vector,
		ZVector:    zScore,
		RiskScore:  riskScore,
		RiskLevel:  determineRiskLevel(riskScore),
		DenialRate: vector[3],
	}
}

func determineRiskLevel(score float64) string {
	switch {
	case score < 2:
		return "LOW"
	case score < 3:
		return "MEDIUM"
	case score < 4:
		return "HIGH"
	default:
		return "CRITICAL"
	}
}
