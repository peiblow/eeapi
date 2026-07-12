package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/database/redis"
	"github.com/peiblow/eeapi/internal/repository"
)

func TestGetAgentDefinitionRehydratesFromPostgres(t *testing.T) {
	if os.Getenv("SYNX_IT") == "" {
		t.Skip("integration test; set SYNX_IT=1 with postgres+redis up")
	}

	db, err := postgres.Open()
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	rdb, err := redis.Open()
	if err != nil {
		t.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	ctx := context.Background()
	hash := "0x" + strings.Repeat("a", 64)
	key := "synx:agent:" + hash

	cleanup := func() {
		db.ExecContext(ctx, `DELETE FROM agent_skills WHERE agent_hash = $1`, hash)
		db.ExecContext(ctx, `DELETE FROM agent_tools WHERE agent_hash = $1`, hash)
		db.ExecContext(ctx, `DELETE FROM contract_agents WHERE _hash = $1`, hash)
		rdb.Del(ctx, key)
	}
	cleanup()
	defer cleanup()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO contract_agents (_hash, name, version, purpose, model, behavior, system_prompt)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		hash, "Iris", "1.0.0", "keeper of decisions",
		`{"provider":"anthropic","name":"claude-sonnet-4-6","temperature":0.2,"max_tokens":2048}`,
		`{"max_steps":8,"on_deny":"halt","on_error":"halt"}`,
		"You are Iris.",
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO agent_skills (agent_hash, name, content, uses) VALUES ($1, $2, $3, $4)`,
		hash, "morning_review", "do the review", `[]`,
	); err != nil {
		t.Fatalf("seed skill: %v", err)
	}

	if n, _ := rdb.Client.Exists(ctx, key).Result(); n != 0 {
		t.Fatalf("redis key should be absent before rehydration")
	}

	svc := &contractService{
		agentDB: repository.NewPsqlAgentRepository(db),
		rdb:     rdb,
	}

	def, err := svc.GetAgentDefinition(ctx, hash)
	if err != nil {
		t.Fatalf("GetAgentDefinition: %v", err)
	}

	if def.Name != "Iris" || def.Version != "1.0.0" || def.Purpose != "keeper of decisions" {
		t.Errorf("core mismatch: %+v", def)
	}
	if def.SystemPrompt != "You are Iris." || def.Behavior.SystemPrompt != "You are Iris." {
		t.Errorf("system prompt mismatch: def=%q behavior=%q", def.SystemPrompt, def.Behavior.SystemPrompt)
	}
	if def.Model.Provider != "anthropic" || def.Model.MaxTokens != 2048 {
		t.Errorf("model mismatch: %+v", def.Model)
	}
	if def.Behavior.MaxSteps != 8 {
		t.Errorf("behavior mismatch: %+v", def.Behavior)
	}
	if len(def.Skills) != 1 || def.Skills[0].Name != "morning_review" {
		t.Errorf("skills mismatch: %+v", def.Skills)
	}

	if n, _ := rdb.Client.Exists(ctx, key).Result(); n != 1 {
		t.Fatalf("redis key should exist after rehydration (re-cache)")
	}

	notFound := "0x" + strings.Repeat("b", 64)
	if _, err := svc.GetAgentDefinition(ctx, notFound); err != ErrAgentNotFound {
		t.Errorf("missing agent err = %v, want ErrAgentNotFound", err)
	}
}
