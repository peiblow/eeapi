package service

import (
	"context"
	"math"
	"testing"

	"github.com/peiblow/eeapi/internal/schema"
)

const floatTol = 1e-9

type fakeBehaviourRepo struct {
	vector    []float64
	baselines map[string]*schema.BehaviourBaseline

	recordedEvents []schema.BehaviourEvent
	recordedScores []schema.BehaviourScore
	upsertCount    int
}

func newFakeRepo(vector []float64, short, long *schema.BehaviourBaseline) *fakeBehaviourRepo {
	return &fakeBehaviourRepo{
		vector:    vector,
		baselines: map[string]*schema.BehaviourBaseline{"short": short, "long": long},
	}
}

func (f *fakeBehaviourRepo) RecordEvent(ctx context.Context, event *schema.BehaviourEvent) error {
	f.recordedEvents = append(f.recordedEvents, *event)
	return nil
}

func (f *fakeBehaviourRepo) MarkEventAsDenied(ctx context.Context, eventId int64) error {
	return nil
}

func (f *fakeBehaviourRepo) BuildVector(ctx context.Context, contractId, agentId string) ([]float64, error) {
	return f.vector, nil
}

func (f *fakeBehaviourRepo) GetBaseline(ctx context.Context, contractId, agentId, kind string) (*schema.BehaviourBaseline, error) {
	return f.baselines[kind], nil
}

func (f *fakeBehaviourRepo) UpsertBaseline(ctx context.Context, baseline *schema.BehaviourBaseline) error {
	f.upsertCount++
	return nil
}

func (f *fakeBehaviourRepo) PromoteBaseline(ctx context.Context, contractId, agentId, kind string) error {
	return nil
}

func (f *fakeBehaviourRepo) RecordScore(ctx context.Context, score *schema.BehaviourScore) error {
	f.recordedScores = append(f.recordedScores, *score)
	return nil
}

func (f *fakeBehaviourRepo) GetLatestScore(ctx context.Context, contractId, agentId string) (*schema.BehaviourScore, error) {
	return nil, nil
}

func (f *fakeBehaviourRepo) RecordDrift(ctx context.Context, drift *schema.BehaviourDrift) error {
	return nil
}

func (f *fakeBehaviourRepo) GetLatestDrift(ctx context.Context, contractId, agentId string) (*schema.BehaviourDrift, error) {
	return nil, nil
}

func newBaseline(kind string, n int, mu, m2 []float64, frozen bool) *schema.BehaviourBaseline {
	return &schema.BehaviourBaseline{
		ContractId: "c1",
		AgentId:    "a1",
		Kind:       kind,
		State:      "LEARNING",
		N:          n,
		MU:         mu,
		M2:         m2,
		Frozen:     frozen,
	}
}

func sixZeros() []float64 { return make([]float64, 6) }

func sixOnes() []float64 { return []float64{1, 1, 1, 1, 1, 1} }

// ── determineRiskLevel ─────────────────────────────────────────────────────
func TestDetermineRiskLevel(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, "LOW"},
		{1.99, "LOW"},
		{2, "MEDIUM"}, // limite: 2 já é MEDIUM
		{2.99, "MEDIUM"},
		{3, "HIGH"},
		{3.99, "HIGH"},
		{4, "CRITICAL"},
		{5.5, "CRITICAL"},
	}
	for _, c := range cases {
		if got := determineRiskLevel(c.score); got != c.want {
			t.Errorf("determineRiskLevel(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}

// ── computeRiskLevel ───────────────────────────────────────────────────────
// O vetor comportamental tem 6 features fixas (DenialRate vem de vector[3]).
// z = (x-μ)/max(σ, floor) por feature; risk = sqrt(Σ z²).
func TestComputeRiskLevel_ZAndDistance(t *testing.T) {
	vector := []float64{3, 3, 3, 3, 3, 3}
	baseline := []float64{1, 1, 1, 1, 1, 1}
	sigma := []float64{1, 1, 1, 1, 1, 1}

	got := computeRiskLevel(vector, baseline, sigma)

	for i, z := range got.ZVector {
		if math.Abs(z-2) > floatTol {
			t.Errorf("z[%d] = %v, want 2", i, z)
		}
	}
	// sqrt(6 × 2²) = sqrt(24)
	want := math.Sqrt(24)
	if math.Abs(got.RiskScore-want) > floatTol {
		t.Errorf("RiskScore = %v, want %v", got.RiskScore, want)
	}
	if math.Abs(got.DenialRate-3) > floatTol {
		t.Errorf("DenialRate = %v, want vector[3] = 3", got.DenialRate)
	}
}

// Caso de aceitação do brief: risk 5.5 → CRITICAL.
func TestComputeRiskLevel_CriticalAcceptance(t *testing.T) {
	// Só a feature 0 desvia: z = (6.5-1)/1 = 5.5; demais z = 0.
	got := computeRiskLevel(
		[]float64{6.5, 1, 1, 1, 1, 1},
		[]float64{1, 1, 1, 1, 1, 1},
		[]float64{1, 1, 1, 1, 1, 1},
	)
	if math.Abs(got.RiskScore-5.5) > floatTol {
		t.Fatalf("RiskScore = %v, want 5.5", got.RiskScore)
	}
	if got.RiskLevel != "CRITICAL" {
		t.Errorf("RiskLevel = %q, want CRITICAL", got.RiskLevel)
	}
}

// Sigma=0 cai no piso por feature (sigmaFloor): z fica finito em vez de
// NaN/Inf por /0 — e sem explodir como no antigo epsilon.
func TestComputeRiskLevel_ZeroSigmaUsesFloor(t *testing.T) {
	got := computeRiskLevel(sixOnes(), sixZeros(), sixZeros())
	if math.IsNaN(got.RiskScore) || math.IsInf(got.RiskScore, 0) {
		t.Fatalf("RiskScore = %v, want finite", got.RiskScore)
	}
	// Feature 0 tem floor 1: z = (1-0)/1 = 1.
	if math.Abs(got.ZVector[0]-1) > floatTol {
		t.Errorf("z[0] = %v, want 1 (floor 1)", got.ZVector[0])
	}
	// Feature 2 tem floor 0.05: z = (1-0)/0.05 = 20.
	if math.Abs(got.ZVector[2]-20) > floatTol {
		t.Errorf("z[2] = %v, want 20 (floor 0.05)", got.ZVector[2])
	}
}

// ── ProcessEvent: cold start ───────────────────────────────────────────────
// short.N < 30 → só aprende: ObserveOnly, sem Score, ambos baselines atualizados.
func TestProcessEvent_ColdStartObservesOnly(t *testing.T) {
	short := newBaseline("short", 0, sixZeros(), sixZeros(), false)
	long := newBaseline("long", 0, sixZeros(), sixZeros(), false)
	repo := newFakeRepo([]float64{1, 1, 1, 0, 0, 1}, short, long)
	svc := &behaviourService{behaviourDb: repo}

	dec, err := svc.ProcessEvent(context.Background(), schema.BehaviourEvent{
		ContractId: "c1", AgentId: "a1", Tool: "x",
	})
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if !dec.ObserveOnly {
		t.Error("ObserveOnly = false, want true em cold start")
	}
	if dec.Score != nil {
		t.Error("Score != nil, want nil em cold start")
	}
	if dec.Action != "ALLOW" {
		t.Errorf("Action = %q, want ALLOW", dec.Action)
	}
	if len(repo.recordedEvents) != 1 {
		t.Errorf("RecordEvent chamado %dx, want 1", len(repo.recordedEvents))
	}
	if len(repo.recordedScores) != 0 {
		t.Errorf("RecordScore chamado %dx, want 0 em cold start", len(repo.recordedScores))
	}
	// short + long upsertados
	if repo.upsertCount != 2 {
		t.Errorf("upsertCount = %d, want 2 (short+long)", repo.upsertCount)
	}
	if short.N != 1 || long.N != 1 {
		t.Errorf("N short=%d long=%d, want 1/1 (UpdateWelford aplicado)", short.N, long.N)
	}
}

// ── ProcessEvent: active ───────────────────────────────────────────────────
// short.N >= 30 → calcula score, grava, congela long e atualiza short.
func TestProcessEvent_ActiveScoresAndFreezesLong(t *testing.T) {
	// sigma = 1 em todas: M2 = N (var pop = M2/N = 1)
	m2 := []float64{30, 30, 30, 30, 30, 30}
	mu := []float64{1, 1, 1, 1, 1, 1}
	short := newBaseline("short", 30, mu, m2, false)
	long := newBaseline("long", 30, append([]float64{}, mu...), append([]float64{}, m2...), false)

	repo := newFakeRepo([]float64{3, 3, 3, 3, 3, 3}, short, long)
	svc := &behaviourService{behaviourDb: repo}

	dec, err := svc.ProcessEvent(context.Background(), schema.BehaviourEvent{
		ContractId: "c1", AgentId: "a1", Tool: "x",
	})
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if dec.ObserveOnly {
		t.Error("ObserveOnly = true, want false no estado active")
	}
	if dec.Score == nil {
		t.Fatal("Score = nil, want preenchido")
	}
	// z=(3-1)/1=2 em 6 features → sqrt(24)
	want := math.Sqrt(24)
	if math.Abs(dec.Score.RiskScore-want) > floatTol {
		t.Errorf("RiskScore = %v, want %v", dec.Score.RiskScore, want)
	}
	if len(repo.recordedScores) != 1 {
		t.Errorf("RecordScore chamado %dx, want 1", len(repo.recordedScores))
	}
	// long deve congelar
	if !long.Frozen {
		t.Error("long.Frozen = false, want true após primeiro score")
	}
	// short avança (UpdateWelford), long não
	if short.N != 31 {
		t.Errorf("short.N = %d, want 31", short.N)
	}
	if long.N != 30 {
		t.Errorf("long.N = %d, want 30 (congelado, sem update)", long.N)
	}
	// upsert: long (freeze) + short
	if repo.upsertCount != 2 {
		t.Errorf("upsertCount = %d, want 2", repo.upsertCount)
	}
}
