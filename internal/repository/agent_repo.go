package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/swp"
)

type AgentRepository interface {
	SaveAgent(ctx context.Context, hash, name, version string, info swp.AgentInfo) error
	SaveAgentTools(ctx context.Context, agentHash string, tools []swp.ToolStmt) error
	SaveAgentSkills(ctx context.Context, agentHash string, skills []swp.SkillStmt) error
	GetAgentTools(ctx context.Context, agentHash string) ([]swp.ToolStmt, error)
	AgentExists(ctx context.Context, agentHash string) (bool, error)
}

type PsqlAgentRepository struct {
	db *postgres.DB
}

func NewPsqlAgentRepository(db *postgres.DB) AgentRepository {
	return &PsqlAgentRepository{db: db}
}

func (r *PsqlAgentRepository) SaveAgent(ctx context.Context, hash, name, version string, info swp.AgentInfo) error {
	modelJSON, err := json.Marshal(info.Model)
	if err != nil {
		return fmt.Errorf("failed to marshal agent model: %w", err)
	}

	behaviorJSON, err := json.Marshal(struct {
		MaxSteps int    `json:"max_steps"`
		OnDeny   string `json:"on_deny"`
		OnError  string `json:"on_error"`
	}{
		MaxSteps: info.Behavior.MaxSteps,
		OnDeny:   info.Behavior.OnDeny,
		OnError:  info.Behavior.OnError,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal agent behavior: %w", err)
	}

	query := `
		INSERT INTO contract_agents (_hash, name, version, purpose, model, behavior, system_prompt)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (_hash) DO NOTHING
	`
	_, err = r.db.ExecContext(ctx, query,
		hash, name, version, info.Purpose, modelJSON, behaviorJSON, info.Behavior.SystemPrompt,
	)
	return err
}

func (r *PsqlAgentRepository) SaveAgentTools(ctx context.Context, agentHash string, tools []swp.ToolStmt) error {
	query := `INSERT INTO agent_tools (agent_hash, name, description, steps) VALUES ($1, $2, $3, $4) ON CONFLICT (agent_hash, name) DO NOTHING`

	for _, tool := range tools {
		stepsJson, err := json.Marshal(tool.Steps)
		if err != nil {
			return fmt.Errorf("failed to marshal tool steps: %w", err)
		}

		if _, err := r.db.ExecContext(ctx, query, agentHash, tool.Name, tool.Description, stepsJson); err != nil {
			return fmt.Errorf("failed to save tool '%s': %w", tool.Name, err)
		}
	}

	return nil
}

func (r *PsqlAgentRepository) SaveAgentSkills(ctx context.Context, agentHash string, skills []swp.SkillStmt) error {
	query := `INSERT INTO agent_skills (agent_hash, name, content, uses) VALUES ($1, $2, $3, $4) ON CONFLICT (agent_hash, name) DO NOTHING`

	for _, skill := range skills {
		uses := skill.Uses
		if uses == nil {
			uses = []string{}
		}
		usesJson, err := json.Marshal(uses)
		if err != nil {
			return fmt.Errorf("failed to marshal skill uses: %w", err)
		}

		if _, err := r.db.ExecContext(ctx, query, agentHash, skill.Name, skill.Content, usesJson); err != nil {
			return fmt.Errorf("failed to save skill '%s': %w", skill.Name, err)
		}
	}

	return nil
}

func (r *PsqlAgentRepository) GetAgentTools(ctx context.Context, agentHash string) ([]swp.ToolStmt, error) {
	query := `SELECT name, description, steps FROM agent_tools WHERE agent_hash = $1`
	rows, err := r.db.QueryContext(ctx, query, agentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to query agent tools: %w", err)
	}
	defer rows.Close()

	var tools []swp.ToolStmt
	for rows.Next() {
		var tool swp.ToolStmt
		var stepsJson []byte

		if err := rows.Scan(&tool.Name, &tool.Description, &stepsJson); err != nil {
			return nil, fmt.Errorf("failed to scan tool row: %w", err)
		}

		if err := json.Unmarshal(stepsJson, &tool.Steps); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool steps: %w", err)
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

func (r *PsqlAgentRepository) AgentExists(ctx context.Context, agentHash string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM contract_agents WHERE _hash = $1)`
	if err := r.db.QueryRowContext(ctx, query, agentHash).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
