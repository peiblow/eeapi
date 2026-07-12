package trigger

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/peiblow/eeapi/internal/schema"
	"github.com/peiblow/eeapi/internal/service"
	"github.com/peiblow/eeapi/internal/swp"
)

type fakeEvents struct {
	calls []captured
}

type captured struct {
	agentHash string
	input     *service.EnqueueEventInput
}

func (f *fakeEvents) EnqueueAgentEvent(ctx context.Context, agentHash string, in *service.EnqueueEventInput) (*service.EnqueueEventResult, error) {
	f.calls = append(f.calls, captured{agentHash, in})
	return &service.EnqueueEventResult{EventID: "ev1", ContextID: "ctx1", StreamID: "s1"}, nil
}

func TestHTTPProviderMountsDeclaredRoute(t *testing.T) {
	fe := &fakeEvents{}
	r := chi.NewRouter()

	NewHTTPProvider().Mount(r, fe, []Binding{{
		Agent:  schema.AgentDefinition{Hash: "0xIRIS"},
		Config: map[string]any{"route": "/notas", "method": "POST"},
	}})

	req := httptest.NewRequest(http.MethodPost, "/notas", strings.NewReader(`{"text":"oi"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if len(fe.calls) != 1 {
		t.Fatalf("enqueue calls = %d, want 1", len(fe.calls))
	}
	if fe.calls[0].agentHash != "0xIRIS" {
		t.Errorf("agent = %q, want 0xIRIS", fe.calls[0].agentHash)
	}
	if string(fe.calls[0].input.Payload) != `{"text":"oi"}` {
		t.Errorf("payload = %s", fe.calls[0].input.Payload)
	}
}

func TestHTTPProviderMapsNotionChannel(t *testing.T) {
	fe := &fakeEvents{}
	r := chi.NewRouter()

	NewHTTPProvider().Mount(r, fe, []Binding{{
		Agent:  schema.AgentDefinition{Hash: "0xPLANNING"},
		Config: map[string]any{"route": "/notion/cards", "channel": "notion"},
	}})

	notion := `{"data":{"properties":{"Name":{"type":"title","title":[{"plain_text":"Implementar Trigger"}]}}}}`
	req := httptest.NewRequest(http.MethodPost, "/notion/cards", strings.NewReader(notion))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if len(fe.calls) != 1 {
		t.Fatalf("enqueue calls = %d, want 1", len(fe.calls))
	}

	var p struct {
		Text string          `json:"text"`
		Raw  json.RawMessage `json:"raw"`
	}
	if err := json.Unmarshal(fe.calls[0].input.Payload, &p); err != nil {
		t.Fatalf("enqueued payload not canonical: %v (%s)", err, fe.calls[0].input.Payload)
	}
	if p.Text != "Implementar Trigger" {
		t.Errorf("mapped text = %q, want %q", p.Text, "Implementar Trigger")
	}
	if len(p.Raw) == 0 {
		t.Error("raw payload not preserved")
	}
}

func TestHTTPProviderHMACAdmission(t *testing.T) {
	t.Setenv("WPP_SECRET", "topsecret")
	fe := &fakeEvents{}
	r := chi.NewRouter()

	NewHTTPProvider().Mount(r, fe, []Binding{{
		Agent:  schema.AgentDefinition{Hash: "0xIRIS"},
		Config: map[string]any{"route": "/wh", "auth": "hmac", "secret_ref": "WPP_SECRET"},
	}})

	body := `{"text":"hi"}`

	req := httptest.NewRequest(http.MethodPost, "/wh", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned status = %d, want 401", w.Code)
	}
	if len(fe.calls) != 0 {
		t.Fatalf("unsigned request enqueued %d events, want 0", len(fe.calls))
	}

	mac := hmac.New(sha256.New, []byte("topsecret"))
	mac.Write([]byte(body))
	sig := hex.EncodeToString(mac.Sum(nil))

	req2 := httptest.NewRequest(http.MethodPost, "/wh", strings.NewReader(body))
	req2.Header.Set("X-Hub-Signature-256", "sha256="+sig)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("signed status = %d, want 202", w2.Code)
	}
	if len(fe.calls) != 1 {
		t.Fatalf("signed request enqueued %d events, want 1", len(fe.calls))
	}
}

func TestCronProviderRegistersAndFires(t *testing.T) {
	fe := &fakeEvents{}
	p := NewCronProvider()

	err := p.Start(fe, []Binding{{
		Agent: schema.AgentDefinition{
			Hash:   "0xIRIS",
			Skills: []swp.SkillStmt{{Name: "morning_review", Content: "do the review"}},
		},
		Config: map[string]any{
			"schedule": "0 8 * * *",
			"timezone": "America/Sao_Paulo",
			"skill":    "morning_review",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	if n := len(p.cron.Entries()); n != 1 {
		t.Fatalf("cron entries = %d, want 1", n)
	}

	fire(fe, "0xIRIS", "cron:morning_review", "do the review")

	if len(fe.calls) != 1 {
		t.Fatalf("fire enqueue calls = %d, want 1", len(fe.calls))
	}
	if fe.calls[0].input.Source != "cron:morning_review" {
		t.Errorf("source = %q", fe.calls[0].input.Source)
	}
	if string(fe.calls[0].input.Payload) != `{"text":"do the review"}` {
		t.Errorf("payload = %s", fe.calls[0].input.Payload)
	}
}

func TestCronProviderSkipsInvalidSchedule(t *testing.T) {
	fe := &fakeEvents{}
	p := NewCronProvider()

	p.Start(fe, []Binding{{
		Agent: schema.AgentDefinition{
			Hash:   "0xX",
			Skills: []swp.SkillStmt{{Name: "s", Content: "c"}},
		},
		Config: map[string]any{"schedule": "0 0 0 3 * 0", "skill": "s"},
	}})
	defer p.Stop()

	if n := len(p.cron.Entries()); n != 0 {
		t.Fatalf("cron entries = %d, want 0 (6-field schedule must be rejected)", n)
	}
}
