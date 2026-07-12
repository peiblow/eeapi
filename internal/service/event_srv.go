package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/database/redis"
	"github.com/peiblow/eeapi/internal/repository"
	"github.com/peiblow/eeapi/internal/schema"
	goredis "github.com/redis/go-redis/v9"
)

const InboxStream = "synx:inbox"

var (
	ErrPayloadRequired = errors.New("payload is required")
	ErrAgentNotFound   = errors.New("agent not found")
)

type EventService interface {
	EnqueueAgentEvent(ctx context.Context, agentHash string, in *EnqueueEventInput) (*EnqueueEventResult, error)
}

type EnqueueEventInput struct {
	ContextID     string          `json:"context_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Source        string          `json:"source,omitempty"`
	Key           string          `json:"key,omitempty"`
	Channel       string          `json:"channel,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

type AgentEvent struct {
	EventID       string          `json:"event_id"`
	AgentHash     string          `json:"agent_hash"`
	ContextID     string          `json:"context_id"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Source        string          `json:"source,omitempty"`
	Payload       json.RawMessage `json:"payload"`
	EnqueuedAt    int64           `json:"enqueued_at"`
}

type EnqueueEventResult struct {
	EventID   string
	ContextID string
	StreamID  string
}

type eventService struct {
	rdb    *redis.Client
	agents repository.AgentRepository
}

func NewEventService(rdb *redis.Client, db *postgres.DB) EventService {
	return &eventService{
		rdb:    rdb,
		agents: repository.NewPsqlAgentRepository(db),
	}
}

func (s *eventService) EnqueueAgentEvent(ctx context.Context, agentHash string, in *EnqueueEventInput) (*EnqueueEventResult, error) {
	if len(in.Payload) == 0 {
		return nil, ErrPayloadRequired
	}

	if in.Channel != "" {
		if _, err := schema.ParseChannel(in.Channel, in.Payload); err != nil {
			return nil, fmt.Errorf("invalid channel: %w", err)
		}
	}

	exists, err := s.agents.AgentExists(ctx, agentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to verify agent: %w", err)
	}
	if !exists {
		return nil, ErrAgentNotFound
	}

	contextID := in.ContextID
	if contextID == "" {
		contextID = uuid.New().String()
	}

	correlationID := in.CorrelationID
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	ev := AgentEvent{
		EventID:       uuid.New().String(),
		AgentHash:     agentHash,
		ContextID:     contextID,
		CorrelationID: correlationID,
		Source:        in.Source,
		Payload:       in.Payload,
		EnqueuedAt:    time.Now().UTC().UnixMilli(),
	}

	data, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	streamID, err := s.rdb.XAdd(ctx, &goredis.XAddArgs{
		Stream: InboxStream,
		Values: map[string]any{"data": data},
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue event: %w", err)
	}

	return &EnqueueEventResult{
		EventID:   ev.EventID,
		ContextID: contextID,
		StreamID:  streamID,
	}, nil
}
