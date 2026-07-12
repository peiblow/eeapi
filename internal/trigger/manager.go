package trigger

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/peiblow/eeapi/internal/database/redis"
	"github.com/peiblow/eeapi/internal/schema"
	"github.com/peiblow/eeapi/internal/service"
)

const agentKeyPattern = "synx:agent:*"

// EventManager is the single owner of every trigger provider. It walks one
// registry at two lifecycle points: MountRoutes wires request-driven providers
// onto the router; Start boots the self-driven daemons.
type EventManager struct {
	rdb      *redis.Client
	events   service.EventService
	registry map[string]Provider
}

func NewEventManager(rdb *redis.Client, events service.EventService) *EventManager {
	m := &EventManager{
		rdb:      rdb,
		events:   events,
		registry: make(map[string]Provider),
	}
	m.register(NewHTTPProvider())
	m.register(NewCronProvider())
	m.register(NewFSProvider(rdb))
	return m
}

func (m *EventManager) register(p Provider) {
	m.registry[p.Type()] = p
}

func (m *EventManager) MountRoutes(r chi.Router) {
	bindings := m.bindings(context.Background())
	for typ, p := range m.registry {
		rp, ok := p.(RouteProvider)
		if !ok {
			continue
		}
		rp.Mount(r, m.events, bindings[typ])
	}
}

func (m *EventManager) Start(ctx context.Context) {
	bindings := m.bindings(ctx)
	for typ, p := range m.registry {
		dp, ok := p.(DaemonProvider)
		if !ok {
			continue
		}
		if err := dp.Start(m.events, bindings[typ]); err != nil {
			slog.Error("provider failed to start", "type", typ, "error", err)
		}
	}
}

func (m *EventManager) Stop() {
	for _, p := range m.registry {
		if dp, ok := p.(DaemonProvider); ok {
			dp.Stop()
		}
	}
}

func (m *EventManager) bindings(ctx context.Context) map[string][]Binding {
	out := make(map[string][]Binding)

	defs, err := m.loadAgentDefinitions(ctx)
	if err != nil {
		slog.Error("loading agent definitions for triggers", "error", err)
		return out
	}

	for _, def := range defs {
		for _, tr := range def.Triggers {
			out[tr.Type] = append(out[tr.Type], Binding{Agent: def, Config: tr.Config})
		}
	}
	return out
}

func (m *EventManager) loadAgentDefinitions(ctx context.Context) ([]schema.AgentDefinition, error) {
	keys, err := m.rdb.Keys(ctx, agentKeyPattern).Result()
	if err != nil {
		return nil, err
	}

	defs := make([]schema.AgentDefinition, 0, len(keys))
	for _, key := range keys {
		raw, err := m.rdb.Get(ctx, key)
		if err != nil {
			slog.Error("reading agent definition", "key", key, "error", err)
			continue
		}

		var def schema.AgentDefinition
		if err := json.Unmarshal([]byte(raw), &def); err != nil {
			slog.Error("unmarshaling agent definition", "key", key, "error", err)
			continue
		}
		defs = append(defs, def)
	}
	return defs, nil
}
