package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"log/slog"
	"os"
)

func LoadOrCreateKeys(path string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	if fileExists(path) {
		slog.Info("Key file found, loading keys", "path", path)
		return loadKeyFromFile(path)
	}

	pub, priv, err := GenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}

	if err := saveKeyToFile(path, priv); err != nil {
		return nil, nil, err
	}

	slog.Info("New key pair generated and saved to file", "path", path)
	return pub, priv, nil
}

func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func SignBlock(hash []byte, privateKey ed25519.PrivateKey) []byte {
	return ed25519.Sign(privateKey, hash)
}

func VerifyBlockSignature(hash []byte, signature []byte, publicKey ed25519.PublicKey) bool {
	return ed25519.Verify(publicKey, hash, signature)
}

// Helper functions for file operations
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func loadKeyFromFile(path string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	if len(data) != ed25519.PrivateKeySize {
		return nil, nil, os.ErrInvalid
	}

	pub := ed25519.PrivateKey(data).Public().(ed25519.PublicKey)
	priv := ed25519.PrivateKey(data)

	return pub, priv, nil
}

func saveKeyToFile(path string, priv ed25519.PrivateKey) error {
	return os.WriteFile(path, priv, 0600)
}
