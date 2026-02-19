package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/peiblow/eeapi/internal/service"
	"github.com/peiblow/eeapi/internal/swp"
)

type DeployApiResponse struct {
	ContractHash    string `json:"contract_hash"`
	ContractName    string `json:"contract_name"`
	ContractOwner   string `json:"contract_owner"`
	ContractVersion string `json:"contract_version"`
}

func DeployHandler(svc service.ContractService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse multipart form (max 10MB)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
			slog.Error("Failed to parse form", "error", err)
			return
		}

		file, _, err := r.FormFile("source")
		if err != nil {
			http.Error(w, "Missing source file: "+err.Error(), http.StatusBadRequest)
			slog.Error("Missing source file", "error", err)
			return
		}
		defer file.Close()

		source, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read source file: "+err.Error(), http.StatusBadRequest)
			slog.Error("Failed to read source file", "error", err)
			return
		}

		req := swp.DeployPayload{
			Hash:         r.FormValue("hash"),
			ContractName: r.FormValue("contract_name"),
			Version:      r.FormValue("version"),
			Owner:        r.FormValue("owner"),
			Source:       source,
		}

		contract, err := svc.DeployContract(r.Context(), &req)
		if err != nil {
			http.Error(w, "Failed to deploy contract: "+err.Error(), http.StatusInternalServerError)
			slog.Error("Failed to deploy contract", "error", err)
			return
		}

		var resp DeployApiResponse
		if err := json.Unmarshal(contract.Data, &resp); err != nil {
			http.Error(w, "Failed to parse response: "+err.Error(), http.StatusInternalServerError)
			slog.Error("Failed to parse response", "error", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
