package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/peiblow/eeapi/internal/license"
)

type licenseRequest struct {
	AgentHash    string   `json:"agent_hash"`
	ContractHash string   `json:"contract_hash"`
	TenantID     string   `json:"tenant_id"`
	Subject      string   `json:"subject"`
	Features     []string `json:"features"`
}

func IssueLicenseHandler(svc *license.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			http.Error(w, "license issuance not configured", http.StatusServiceUnavailable)
			return
		}

		var req licenseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}

		id, jwt, err := svc.Issue(r.Context(), license.IssueInput{
			AgentHash:    req.AgentHash,
			ContractHash: req.ContractHash,
			TenantID:     req.TenantID,
			Subject:      req.Subject,
			Features:     req.Features,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			slog.Error("license issue failed", "agent", req.AgentHash, "tenant", req.TenantID, "error", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": id, "jwt": jwt})
		slog.Info("license issued", "id", id, "agent", req.AgentHash, "tenant", req.TenantID)
	}
}
