package schema

type Contract struct {
	ID string `json:"id"`

	Name    string `json:"contract_name"`
	Version string `json:"version"`
	Owner   string `json:"owner"`

	ArtifactHash string `json:"artifact_hash"`

	CreatedAt int64 `json:"created_at"`
}
