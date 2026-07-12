package trigger

import (
	"github.com/go-chi/chi/v5"
	"github.com/peiblow/eeapi/internal/schema"
	"github.com/peiblow/eeapi/internal/service"
)

// Binding is one declared trigger tied to the agent that owns it.
type Binding struct {
	Agent  schema.AgentDefinition
	Config map[string]any
}

// Provider knows how to materialize one trigger type into events on
// synx:inbox. Providers come in two shapes and the manager wires each at the
// lifecycle point that fits it.
type Provider interface {
	Type() string
}

// RouteProvider is request-driven: the world reaches the agent through HTTP
// routes it mounts (webhooks).
type RouteProvider interface {
	Provider
	Mount(r chi.Router, events service.EventService, bindings []Binding)
}

// DaemonProvider is self-driven: the manager starts it and it fires on its own
// (a clock, a filesystem watcher).
type DaemonProvider interface {
	Provider
	Start(events service.EventService, bindings []Binding) error
	Stop()
}
