package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// PutArtifact writes a compiled artifact to the content store as <hash>.snxb.
// The write is atomic (temp file + rename) so a concurrent reader never sees a
// partially written file. Re-deploying the same hash overwrites with identical
// content, so it is idempotent.
func PutArtifact(dir, hash string, blob []byte) error {
	if dir == "" {
		return fmt.Errorf("artifact dir not configured")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating artifact dir: %w", err)
	}

	final := filepath.Join(dir, hash+".snxb")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o644); err != nil {
		return fmt.Errorf("writing artifact: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("finalizing artifact: %w", err)
	}
	return nil
}

// GetArtifact reads the raw artifact bytes for the given hash from the store.
func GetArtifact(dir, hash string) ([]byte, error) {
	if dir == "" {
		return nil, fmt.Errorf("artifact dir not configured")
	}
	blob, err := os.ReadFile(filepath.Join(dir, hash+".snxb"))
	if err != nil {
		return nil, fmt.Errorf("reading artifact %s: %w", hash, err)
	}
	return blob, nil
}
