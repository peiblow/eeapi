package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/peiblow/eeapi/internal/service"
)

type DeployApiResponse struct {
	ContractHash    string `json:"contract_hash"`
	ContractName    string `json:"contract_name"`
	ContractOwner   string `json:"contract_owner"`
	ContractVersion string `json:"contract_version"`
	AgentHash       string `json:"agent_hash"`
}

func DeployHandler(svc service.ContractService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse multipart form (max 10MB)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
			slog.Error("Failed to parse form", "error", err)
			return
		}

		// The CLI uploads a prebuilt artifact (.snxb) — EEAPI no longer compiles.
		file, _, err := r.FormFile("artifact")
		if err != nil {
			http.Error(w, "Missing artifact file: "+err.Error(), http.StatusBadRequest)
			slog.Error("Missing artifact file", "error", err)
			return
		}
		defer file.Close()

		artifact, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read artifact file: "+err.Error(), http.StatusBadRequest)
			slog.Error("Failed to read artifact file", "error", err)
			return
		}

		in := &service.DeployInput{
			ContractName: r.FormValue("contract_name"),
			Version:      r.FormValue("version"),
			Owner:        r.FormValue("owner"),
			Artifact:     artifact,
			AgentHash:    r.FormValue("agent_hash"),
			AgentName:    r.FormValue("agent_name"),
			AgentVersion: r.FormValue("agent_version"),
		}

		result, err := svc.DeployContract(r.Context(), in)
		if err != nil {
			http.Error(w, "Failed to deploy contract: "+err.Error(), http.StatusInternalServerError)
			slog.Error("Failed to deploy contract", "error", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DeployApiResponse{
			ContractHash:    result.ContractHash,
			ContractName:    result.ContractName,
			ContractOwner:   result.ContractOwner,
			ContractVersion: result.ContractVersion,
			AgentHash:       result.AgentHash,
		})
	}
}
