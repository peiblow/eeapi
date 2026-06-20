package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/schema"
)

const featureCount = 6
const behaviourWindow = "5 minutes"

type BehaviourRepository interface {
	RecordEvent(ctx context.Context, event *schema.BehaviourEvent) error
	MarkEventAsDenied(ctx context.Context, eventId int64) error

	// ── janela + agregação ──────────────────────────────────────────────────────────
	BuildVector(ctx context.Context, contractId string, agentId string) ([]float64, error)

	// ── baseline ──────────────────────────────────────────────────────────
	GetBaseline(ctx context.Context, contractId string, agentId string, kind string) (*schema.BehaviourBaseline, error)
	UpsertBaseline(ctx context.Context, baseline *schema.BehaviourBaseline) error
	PromoteBaseline(ctx context.Context, contractId string, agentId string, kind string) error

	// ── Score ──────────────────────────────────────────────────────────
	RecordScore(ctx context.Context, score *schema.BehaviourScore) error
	GetLatestScore(ctx context.Context, contractId string, agentId string) (*schema.BehaviourScore, error)

	// ── Drift ──────────────────────────────────────────────────────────
	RecordDrift(ctx context.Context, drift *schema.BehaviourDrift) error
	GetLatestDrift(ctx context.Context, contractId string, agentId string) (*schema.BehaviourDrift, error)
}

type PsqlBehaviourRepository struct {
	db *postgres.DB
}

func NewPsqlBehaviourRepository(db *postgres.DB) BehaviourRepository {
	return &PsqlBehaviourRepository{db: db}
}

func (r *PsqlBehaviourRepository) RecordEvent(ctx context.Context, event *schema.BehaviourEvent) error {
	query := `
		INSERT INTO behavior_event (contract_id, agent_id, tool, payload_hash, denied)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query,
		event.ContractId, event.AgentId, event.Tool, event.PayloadHash, event.Denied,
	).Scan(&event.ID)

	return err
}

func (r *PsqlBehaviourRepository) MarkEventAsDenied(ctx context.Context, decisionEventId int64) error {
	query := `
		UPDATE behavior_event
		SET denied = true
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, decisionEventId)
	if err != nil {
		return err
	}

	return nil
}

func (r *PsqlBehaviourRepository) BuildVector(ctx context.Context, contractId string, agentId string) ([]float64, error) {
	query := `
		SELECT
			COUNT(*)                                  AS total,
			COALESCE(MAX(tool_count), 0)              AS top_tool,
			COUNT(DISTINCT tool)                      AS distinct_tools,
			COUNT(*) FILTER (WHERE denied)            AS denials,
			COUNT(*) - COUNT(DISTINCT payload_hash)   AS repeated,
			EXTRACT(EPOCH FROM (MAX(ts) - MIN(ts)))   AS span_seconds
		FROM (
			SELECT tool, payload_hash, denied, ts,
			       COUNT(*) OVER (PARTITION BY tool) AS tool_count
			FROM behavior_event
			WHERE contract_id = $1 AND agent_id = $2
			  AND ts > now() - ($3)::interval
		) w
	`

	var total, topTool, distinctTools, denials, repeated int64
	var spanSeconds sql.NullFloat64
	err := r.db.QueryRowContext(ctx, query, contractId, agentId, behaviourWindow).
		Scan(&total, &topTool, &distinctTools, &denials, &repeated, &spanSeconds)
	if err != nil {
		return nil, err
	}

	vector := make([]float64, featureCount)
	vector[0] = float64(total)
	vector[1] = float64(topTool)
	vector[2] = float64(distinctTools)
	if total > 0 {
		vector[3] = float64(denials) / float64(total)
		vector[4] = float64(repeated) / float64(total)
		if spanSeconds.Valid && spanSeconds.Float64 > 0 {
			vector[5] = float64(total) / (spanSeconds.Float64 / 60.0)
		}
	}

	return vector, nil
}

func (r *PsqlBehaviourRepository) GetBaseline(ctx context.Context, contractId string, agentId string, kind string) (*schema.BehaviourBaseline, error) {
	query := `
		SELECT state, n, mu, m2, frozen, updated_at
		FROM behavior_baseline
		WHERE contract_id = $1 AND agent_id = $2 AND kind = $3
	`

	b := &schema.BehaviourBaseline{ContractId: contractId, AgentId: agentId, Kind: kind}
	var updatedAt time.Time

	err := r.db.QueryRowContext(ctx, query, contractId, agentId, kind).
		Scan(&b.State, &b.N, pq.Array(&b.MU), pq.Array(&b.M2), &b.Frozen, &updatedAt)

	if err == sql.ErrNoRows {
		return &schema.BehaviourBaseline{
			ContractId: contractId,
			AgentId:    agentId,
			Kind:       kind,
			State:      "LEARNING",
			N:          0,
			MU:         make([]float64, featureCount),
			M2:         make([]float64, featureCount),
			Frozen:     false,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	b.UpdatedAt = updatedAt.UnixMilli()
	return b, nil
}

func (r *PsqlBehaviourRepository) UpsertBaseline(ctx context.Context, baseline *schema.BehaviourBaseline) error {
	state := baseline.State
	if state == "" {
		state = "LEARNING"
	}

	query := `
		INSERT INTO behavior_baseline (contract_id, agent_id, kind, state, n, mu, m2, frozen, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (contract_id, agent_id, kind) DO UPDATE SET
			state      = EXCLUDED.state,
			n          = EXCLUDED.n,
			mu         = EXCLUDED.mu,
			m2         = EXCLUDED.m2,
			frozen     = EXCLUDED.frozen,
			updated_at = now()
	`
	_, err := r.db.ExecContext(ctx, query,
		baseline.ContractId, baseline.AgentId, baseline.Kind, state,
		baseline.N, pq.Array(baseline.MU), pq.Array(baseline.M2), baseline.Frozen,
	)
	return err
}

func (r *PsqlBehaviourRepository) PromoteBaseline(ctx context.Context, contractId string, agentId string, kind string) error {
	query := `
		UPDATE behavior_baseline
		SET state = 'ACTIVE', updated_at = now()
		WHERE contract_id = $1 AND agent_id = $2 AND kind = $3
	`
	_, err := r.db.ExecContext(ctx, query, contractId, agentId, kind)
	return err
}

func (r *PsqlBehaviourRepository) RecordScore(ctx context.Context, score *schema.BehaviourScore) error {
	query := `
		INSERT INTO behavior_score (contract_id, agent_id, vector, z_vector, risk_score, risk_level, journal_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query,
		score.ContractId, score.AgentId,
		pq.Array(score.Vector), pq.Array(score.ZVector),
		score.RiskScore, score.RiskLevel, score.JournalHash,
	)
	return err
}

func (r *PsqlBehaviourRepository) GetLatestScore(ctx context.Context, contractId string, agentId string) (*schema.BehaviourScore, error) {
	query := `
		SELECT vector, z_vector, risk_score, risk_level, journal_hash, ts
		FROM behavior_score
		WHERE contract_id = $1 AND agent_id = $2
		ORDER BY ts DESC
		LIMIT 1
	`

	s := &schema.BehaviourScore{ContractId: contractId, AgentId: agentId}
	var ts time.Time
	var journalHash sql.NullString

	err := r.db.QueryRowContext(ctx, query, contractId, agentId).
		Scan(pq.Array(&s.Vector), pq.Array(&s.ZVector), &s.RiskScore, &s.RiskLevel, &journalHash, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.JournalHash = journalHash.String
	s.TS = ts.UnixMilli()
	return s, nil
}

func (r *PsqlBehaviourRepository) RecordDrift(ctx context.Context, drift *schema.BehaviourDrift) error {
	query := `
		INSERT INTO behavior_drift (contract_id, agent_id, drift_distance)
		VALUES ($1, $2, $3)
	`
	_, err := r.db.ExecContext(ctx, query, drift.ContractId, drift.AgentId, drift.DriftDistance)
	return err
}

func (r *PsqlBehaviourRepository) GetLatestDrift(ctx context.Context, contractId string, agentId string) (*schema.BehaviourDrift, error) {
	query := `
		SELECT drift_distance, ts
		FROM behavior_drift
		WHERE contract_id = $1 AND agent_id = $2
		ORDER BY ts DESC
		LIMIT 1
	`

	d := &schema.BehaviourDrift{ContractId: contractId, AgentId: agentId}
	var ts time.Time

	err := r.db.QueryRowContext(ctx, query, contractId, agentId).
		Scan(&d.DriftDistance, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	d.TS = ts.UnixMilli()
	return d, nil
}
