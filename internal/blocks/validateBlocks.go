package blocks

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/peiblow/eeapi/internal/schema"
)

func VerifyBlock(lastBlock, newBlock schema.Block, journalBytes []byte, pub ed25519.PublicKey) error {
	if newBlock.PreviousHash != lastBlock.Hash {
		return fmt.Errorf("invalid previous hash: expected %s, got %s", lastBlock.Hash, newBlock.PreviousHash)
	}

	if newBlock.Timestamp <= lastBlock.Timestamp {
		return fmt.Errorf("invalid timestamp: new block timestamp %d is not greater than last block timestamp %d", newBlock.Timestamp, lastBlock.Timestamp)
	}

	journalHashRaw := sha256.Sum256(append(journalBytes, []byte(fmt.Sprintf("%d", newBlock.Timestamp))...))
	journalHash := "0x" + hex.EncodeToString(journalHashRaw[:])
	if journalHash != newBlock.JournalHash {
		return fmt.Errorf("invalid journal hash: expected %s, got %s", newBlock.JournalHash, journalHash)
	}

	hashBytes, _ := hex.DecodeString(strings.TrimPrefix(newBlock.Hash, "0x"))
	if !ed25519.Verify(pub, hashBytes, newBlock.Signature) {
		return fmt.Errorf("invalid block signature")
	}

	return nil
}
