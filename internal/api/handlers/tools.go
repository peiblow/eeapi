package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/peiblow/eeapi/internal/service"
)

func GetAgentToolsHandler(svc service.ContractService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentHash := chi.URLParam(r, "agentHash")

		tools, err := svc.GetAgentTools(r.Context(), agentHash)
		if err != nil {
			slog.Error("Failed to get agent tools", "error", err, "agent_hash", agentHash)
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tools)
	}
}

func GetAgentDefinitionHandler(svc service.ContractService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentHash := chi.URLParam(r, "agentHash")

		def, err := svc.GetAgentDefinition(r.Context(), agentHash)
		if err != nil {
			if errors.Is(err, service.ErrAgentNotFound) {
				http.Error(w, "Agent not found", http.StatusNotFound)
				return
			}
			slog.Error("Failed to get agent definition", "error", err, "agent_hash", agentHash)
			http.Error(w, "Failed to get agent definition", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(def)
	}
}
