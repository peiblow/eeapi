package license

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Service struct {
	priv ed25519.PrivateKey
	db   *sql.DB
	iss  string
	ttl  time.Duration
}

func NewService(seedHex string, db *sql.DB) (*Service, error) {
	seed, err := hex.DecodeString(strings.TrimSpace(seedHex))
	if err != nil {
		return nil, fmt.Errorf("license key: invalid hex: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("license key: expected %d-byte seed, got %d", ed25519.SeedSize, len(seed))
	}
	return &Service{
		priv: ed25519.NewKeyFromSeed(seed),
		db:   db,
		iss:  "synx-eeapi",
		ttl:  90 * 24 * time.Hour,
	}, nil
}

type IssueInput struct {
	AgentHash    string
	ContractHash string
	TenantID     string
	Subject      string
	Features     []string
}

func (s *Service) Issue(ctx context.Context, in IssueInput) (id, jwt string, err error) {
	if in.TenantID == "" || in.AgentHash == "" {
		return "", "", fmt.Errorf("tenant_id and agent_hash are required")
	}
	features := in.Features
	if len(features) == 0 {
		features = []string{"mcp"}
	}

	id = uuidV4()
	now := time.Now()
	exp := now.Add(s.ttl)
	jwt = s.signJWT(map[string]any{
		"id":            id,
		"iss":           s.iss,
		"sub":           in.Subject,
		"tenant_id":     in.TenantID,
		"agent_hash":    in.AgentHash,
		"contract_hash": in.ContractHash,
		"features":      features,
		"iat":           now.Unix(),
		"exp":           exp.Unix(),
	})

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO licenses (id, tenant_id, subject, features, expires_at, issued_by, jwt)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, in.TenantID, in.Subject, pq.Array(features), exp, s.iss, jwt)
	if err != nil {
		return "", "", fmt.Errorf("insert license: %w", err)
	}
	return id, jwt, nil
}

func (s *Service) signJWT(claims map[string]any) string {
	hdr, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT"})
	pl, _ := json.Marshal(claims)
	si := b64(hdr) + "." + b64(pl)
	sig := ed25519.Sign(s.priv, []byte(si))
	return si + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func uuidV4() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]), binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]), binary.BigEndian.Uint16(b[8:10]), b[10:16])
}
