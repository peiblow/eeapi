package keys

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
)

func EncryptSHA256(data string) (string, error) {
	hashInput := []byte(data)
	hashBytes := sha256.Sum256(hashInput)
	hash := "0x" + hex.EncodeToString(hashBytes[:])

	return hash, nil
}

func deriveAESKey(priv ed25519.PrivateKey) []byte {
	hash := sha256.Sum256(priv)
	return hash[:]
}

func EncryptJournal(plain []byte, key []byte) ([]byte, error) {
	aesKey := deriveAESKey(key)

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		slog.Error("Failed to create cipher block", "error", err)
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		slog.Error("Failed to create GCM cipher", "error", err)
		return nil, err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		slog.Error("Failed to generate nonce", "error", err)
		return nil, err
	}

	ciphertext := aesGCM.Seal(nil, nonce, plain, nil)
	return append(nonce, ciphertext...), nil
}

func DecryptJournal(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, data := ciphertext[:nonceSize], ciphertext[nonceSize:]

	return aesGCM.Open(nil, nonce, data, nil)
}
