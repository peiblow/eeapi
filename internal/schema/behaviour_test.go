package schema

import (
	"math"
	"testing"
)

const floatTol = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= floatTol
}

// UpdateWelford deve produzir média e M2 incrementais idênticos ao cálculo direto.
// Caso conhecido (1 feature): amostras [2, 4] → MU=3, M2=2 (Σ(x-μ)²).
func TestUpdateWelford_KnownTwoSamples(t *testing.T) {
	b := &BehaviourBaseline{
		MU: make([]float64, 1),
		M2: make([]float64, 1),
	}

	b.UpdateWelford([]float64{2})
	b.UpdateWelford([]float64{4})

	if b.N != 2 {
		t.Fatalf("N = %d, want 2", b.N)
	}
	if !almostEqual(b.MU[0], 3) {
		t.Errorf("MU = %v, want 3", b.MU[0])
	}
	// Σ(x-μ)² para [2,4] com μ=3 → 1 + 1 = 2
	if !almostEqual(b.M2[0], 2) {
		t.Errorf("M2 = %v, want 2", b.M2[0])
	}
}

// A média e a variância populacional incrementais devem bater com o cálculo direto
// sobre o conjunto inteiro, para várias features simultâneas.
func TestUpdateWelford_MatchesBatch(t *testing.T) {
	samples := [][]float64{
		{1, 10},
		{2, 20},
		{3, 30},
		{4, 40},
		{5, 50},
	}

	b := &BehaviourBaseline{
		MU: make([]float64, 2),
		M2: make([]float64, 2),
	}
	for _, s := range samples {
		b.UpdateWelford(s)
	}

	// referência direta
	n := float64(len(samples))
	for f := 0; f < 2; f++ {
		var sum float64
		for _, s := range samples {
			sum += s[f]
		}
		mean := sum / n
		var m2 float64
		for _, s := range samples {
			d := s[f] - mean
			m2 += d * d
		}

		if !almostEqual(b.MU[f], mean) {
			t.Errorf("feature %d: MU = %v, want %v", f, b.MU[f], mean)
		}
		if !almostEqual(b.M2[f], m2) {
			t.Errorf("feature %d: M2 = %v, want %v", f, b.M2[f], m2)
		}
	}
}

// Sigma deve ser desvio-padrão POPULACIONAL (÷N, não ÷N-1).
func TestSigma_PopulationStdDev(t *testing.T) {
	b := &BehaviourBaseline{
		MU: make([]float64, 1),
		M2: make([]float64, 1),
	}
	b.UpdateWelford([]float64{2})
	b.UpdateWelford([]float64{4})

	got := b.Sigma()
	// var pop de [2,4] = 2/2 = 1 → sigma = 1
	if !almostEqual(got[0], 1) {
		t.Errorf("Sigma = %v, want 1", got[0])
	}
}

// Com menos de 2 amostras o desvio é indefinido: deve devolver zeros (e não NaN/Inf).
func TestSigma_ColdReturnsZeros(t *testing.T) {
	b := &BehaviourBaseline{MU: make([]float64, 3), M2: make([]float64, 3)}
	b.UpdateWelford([]float64{1, 2, 3})

	got := b.Sigma()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, v := range got {
		if v != 0 {
			t.Errorf("Sigma[%d] = %v, want 0", i, v)
		}
	}
}
