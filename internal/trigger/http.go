package trigger

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/peiblow/eeapi/internal/service"
)

const defaultSigHeader = "X-Hub-Signature-256"

type verifier func(body []byte, r *http.Request) bool

type HTTPProvider struct{}

func NewHTTPProvider() *HTTPProvider { return &HTTPProvider{} }

func (h *HTTPProvider) Type() string { return "http" }

func (h *HTTPProvider) Mount(r chi.Router, events service.EventService, bindings []Binding) {
	seen := make(map[string]bool)
	mounted := 0

	for _, b := range bindings {
		route, _ := b.Config["route"].(string)
		if route == "" || !strings.HasPrefix(route, "/") {
			slog.Error("skipping http trigger with invalid route", "agent", b.Agent.Hash, "route", route)
			continue
		}

		method, _ := b.Config["method"].(string)
		if method == "" {
			method = http.MethodPost
		}
		method = strings.ToUpper(method)

		key := method + " " + route
		if seen[key] {
			slog.Error("skipping duplicate http route", "route", route, "method", method)
			continue
		}
		seen[key] = true

		channel, _ := b.Config["channel"].(string)
		verify := buildVerifier(b)
		r.Method(method, route, webhookHandler(events, b.Agent.Hash, channel, verify))
		mounted++
		slog.Info("http trigger mounted", "agent", b.Agent.Hash, "method", method, "route", route, "auth", strConfig(b.Config, "auth"))
	}

	slog.Info("http provider ready", "routes", mounted)
}

func buildVerifier(b Binding) verifier {
	if !strings.EqualFold(strConfig(b.Config, "auth"), "hmac") {
		return nil
	}

	secretRef := strConfig(b.Config, "secret_ref")
	secret := os.Getenv(secretRef)
	header := strConfig(b.Config, "sig_header")
	if header == "" {
		header = defaultSigHeader
	}

	if secret == "" {
		slog.Error("http trigger auth=hmac but secret env is empty; rejecting all requests", "agent", b.Agent.Hash, "secret_ref", secretRef)
		return func([]byte, *http.Request) bool { return false }
	}
	return hmacVerifier(secret, header)
}

func hmacVerifier(secret, header string) verifier {
	return func(body []byte, r *http.Request) bool {
		got := strings.TrimPrefix(r.Header.Get(header), "sha256=")
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		want := hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(got), []byte(want))
	}
}

func webhookHandler(events service.EventService, agentHash, channel string, verify verifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if verify != nil && !verify(body, r) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			slog.Warn("webhook signature rejected", "agent", agentHash, "route", r.URL.Path)
			return
		}

		source := "http:" + r.URL.Path
		if channel != "" {
			source = "http:" + channel + ":" + r.URL.Path
		}

		res, err := events.EnqueueAgentEvent(r.Context(), agentHash, &service.EnqueueEventInput{
			ContextID: uuid.New().String(),
			Source:    source,
			Payload:   body,
		})
		if err != nil {
			switch {
			case errors.Is(err, service.ErrPayloadRequired):
				http.Error(w, err.Error(), http.StatusBadRequest)
			case errors.Is(err, service.ErrAgentNotFound):
				http.Error(w, "agent not found", http.StatusNotFound)
			default:
				http.Error(w, "failed to enqueue event: "+err.Error(), http.StatusInternalServerError)
			}
			slog.Error("webhook enqueue failed", "agent", agentHash, "route", r.URL.Path, "error", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"event_id":   res.EventID,
			"context_id": res.ContextID,
			"stream_id":  res.StreamID,
		})
	}
}
