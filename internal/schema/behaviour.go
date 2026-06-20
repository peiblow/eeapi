package schema

import "math"

type BehaviourEvent struct {
	ID          int64
	ContractId  string
	AgentId     string
	TotalTools  int
	Tool        string
	PayloadHash string
	Denied      bool
	Timestamp   int64
}

type BehaviourBaseline struct {
	ContractId string
	AgentId    string
	Kind       string
	State      string
	N          int
	MU         []float64
	M2         []float64
	Frozen     bool
	UpdatedAt  int64
}

type BehaviourScore struct {
	ContractId  string
	AgentId     string
	Vector      []float64
	ZVector     []float64
	RiskScore   float64
	RiskLevel   string
	DenialRate  float64
	JournalHash string
	TS          int64
}

type BehaviourDrift struct {
	ContractId    string
	AgentId       string
	DriftDistance float64
	TS            int64
}

type GovernanceMemory struct {
	ContractId string
	AgentId    string
	RiskScore  float64
	Decision   string
	Outcome    string
	TS         int64
}

func (b *BehaviourBaseline) Sigma() []float64 {
	sigma := make([]float64, len(b.M2))
	if b.N < 2 {
		return sigma
	}
	for i := range b.M2 {
		sigma[i] = math.Sqrt(b.M2[i] / float64(b.N))
	}
	return sigma
}

func (b *BehaviourBaseline) UpdateWelford(vector []float64) {
	b.N++
	for i := range vector {
		delta := vector[i] - b.MU[i]
		b.MU[i] += delta / float64(b.N)
		delta2 := vector[i] - b.MU[i]
		b.M2[i] += delta * delta2
	}
}
