package repository

import (
	"context"
	"time"

	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/schema"
	contracts "github.com/peiblow/eeapi/internal/schema"
	"github.com/peiblow/eeapi/internal/swp"
)

type ContractRepository interface {
	SaveContract(ctx context.Context, contract *contracts.Contract) error
	SaveContractArtifact(ctx context.Context, artifactHash string, agentHash string) error
	SaveAgentMeta(ctx context.Context, agent *swp.AgentMeta) error
	GetContractByID(ctx context.Context, id string) (*contracts.Contract, error)
}

type PsqlContractRepository struct {
	db *postgres.DB
}

func NewPsqlContractRepository(db *postgres.DB) ContractRepository {
	return &PsqlContractRepository{db: db}
}

func (r *PsqlContractRepository) SaveContract(ctx context.Context, contract *contracts.Contract) error {
	query := `
		INSERT INTO contracts (name, owner, artifact_hash, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := r.db.ExecContext(ctx, query, contract.Name, contract.Owner, contract.ArtifactHash, contract.CreatedAt)

	return err
}

// SaveContractArtifact records the artifact in the registry. The bytecode and
// metadata live in the content store (.snxb file), not the DB — this row only
// links the artifact hash to its agent and creation time.
func (r *PsqlContractRepository) SaveContractArtifact(ctx context.Context, artifactHash string, agentHash string) error {
	query := `INSERT INTO contract_artifacts (_hash, created_at, agent_hash) VALUES ($1, $2, $3)`

	_, err := r.db.ExecContext(
		ctx,
		query,
		artifactHash,
		time.Now().UTC().UnixMilli(),
		agentHash,
	)

	return err
}

func (s *PsqlContractRepository) SaveAgentMeta(ctx context.Context, agent *swp.AgentMeta) error {
	query := `INSERT INTO contract_agents (_hash, name, version) VALUES ($1, $2, $3)`

	_, err := s.db.ExecContext(
		ctx,
		query,
		agent.Hash,
		agent.Name,
		agent.Version,
	)

	return err
}

func (r *PsqlContractRepository) GetContractByID(ctx context.Context, artifactHash string) (*contracts.Contract, error) {
	query := `
		SELECT id, name, owner, artifact_hash, created_at
		FROM contracts
		WHERE artifact_hash = $1
	`
	row := r.db.QueryRowContext(ctx, query, artifactHash)

	var contract contracts.Contract
	if err := row.Scan(&contract.ID, &contract.Name, &contract.Owner, &contract.ArtifactHash, &contract.CreatedAt); err != nil {
		return nil, err
	}

	return &contract, nil
}

func (r *PsqlContractRepository) GetBlocksByContextID(ctx context.Context, contextID string) ([]*schema.Block, error) {
	query := `
		SELECT hash, context_id, block_index, timestamp
		FROM blocks
		WHERE context_id = $1
		ORDER BY block_index ASC
	`
	rows, err := r.db.QueryContext(ctx, query, contextID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []*schema.Block
	for rows.Next() {
		var block schema.Block
		if err := rows.Scan(&block.Hash, &block.ContextID, &block.BlockIndex, &block.Timestamp); err != nil {
			return nil, err
		}
		blocks = append(blocks, &block)
	}

	return blocks, nil
}
