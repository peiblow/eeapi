package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/peiblow/eeapi/internal/service"
)

type EnqueueEventApiResponse struct {
	EventID   string `json:"event_id"`
	ContextID string `json:"context_id"`
	StreamID  string `json:"stream_id"`
}

func EnqueueEventHandler(svc service.EventService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentHash := chi.URLParam(r, "agentHash")

		var in service.EnqueueEventInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			slog.Error("Invalid event payload", "error", err)
			return
		}

		result, err := svc.EnqueueAgentEvent(r.Context(), agentHash, &in)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrPayloadRequired):
				http.Error(w, err.Error(), http.StatusBadRequest)
			case errors.Is(err, service.ErrAgentNotFound):
				http.Error(w, "Agent not found", http.StatusNotFound)
			default:
				http.Error(w, "Failed to enqueue event: "+err.Error(), http.StatusInternalServerError)
			}
			slog.Error("Failed to enqueue agent event", "agent_hash", agentHash, "error", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(EnqueueEventApiResponse{
			EventID:   result.EventID,
			ContextID: result.ContextID,
			StreamID:  result.StreamID,
		})
	}
}
