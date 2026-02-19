package repository

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/schema"
)

type BlockRepository interface {
	SaveBlock(ctx context.Context, block *schema.Block) error
	GetBlockByID(ctx context.Context, id string) (*schema.Block, error)
	GetLastBlock(ctx context.Context) (*schema.Block, error)
}

type PsqlBlockRepository struct {
	db *postgres.DB
}

func NewPsqlBlockRepository(db *postgres.DB) BlockRepository {
	return &PsqlBlockRepository{db: db}
}

func (r *PsqlBlockRepository) SaveBlock(ctx context.Context, block *schema.Block) error {
	query := `
		INSERT INTO blocks (hash, timestamp, previous_hash, journal_hash, signature, contract_id, function_name, journal)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.ExecContext(ctx, query,
		block.Hash,
		block.Timestamp,
		block.PreviousHash,
		block.JournalHash,
		block.Signature,
		block.ContractID,
		block.FunctionName,
		block.Journal,
	)

	return err
}

func (r *PsqlBlockRepository) GetBlockByID(ctx context.Context, id string) (*schema.Block, error) {
	query := `SELECT hash, timestamp, previous_hash, journal_hash, signature, contract_id, function_name, journal FROM blocks WHERE id = $1`

	row := r.db.QueryRowContext(ctx, query, id)

	var block schema.Block
	err := row.Scan(
		&block.Hash,
		&block.Timestamp,
		&block.PreviousHash,
		&block.JournalHash,
		&block.Signature,
		&block.ContractID,
		&block.FunctionName,
		&block.Journal,
	)
	if err != nil {
		return nil, err
	}

	return &block, nil
}

func (r *PsqlBlockRepository) GetLastBlock(ctx context.Context) (*schema.Block, error) {
	query := `SELECT hash, timestamp, previous_hash, journal_hash, signature, contract_id, function_name, journal FROM blocks ORDER BY timestamp DESC LIMIT 1`

	row := r.db.QueryRowContext(ctx, query)

	var block schema.Block
	err := row.Scan(
		&block.Hash,
		&block.Timestamp,
		&block.PreviousHash,
		&block.JournalHash,
		&block.Signature,
		&block.ContractID,
		&block.FunctionName,
		&block.Journal,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Info("No blocks found in database, creating genesis block")
			return r.createGenesisBlock(ctx)
		}
		return nil, err
	}

	return &block, nil
}

func (r *PsqlBlockRepository) createGenesisBlock(ctx context.Context) (*schema.Block, error) {
	slog.Info("No blocks found in database, creating genesis block")
	genesis := &schema.Block{
		Hash:         "0xGENESIS_HASH",
		Timestamp:    time.Now().Unix(),
		PreviousHash: "0",
		JournalHash:  "0",
		Signature:    []byte("GENESIS_SIGNATURE"),
		ContractID:   uuid.NewString(),
		FunctionName: "genesis",
		Journal:      []byte{},
	}

	query := `
		INSERT INTO blocks (
			hash, timestamp, previous_hash, journal_hash, signature, contract_id, function_name, journal
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`

	_, err := r.db.ExecContext(ctx, query,
		genesis.Hash,
		genesis.Timestamp,
		genesis.PreviousHash,
		genesis.JournalHash,
		genesis.Signature,
		genesis.ContractID,
		genesis.FunctionName,
		genesis.Journal,
	)
	if err != nil {
		return nil, err
	}

	return genesis, nil
}
