package api

import (
	"crypto/ed25519"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/peiblow/eeapi/internal/config"
	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/swp"
)

type Server struct {
	cfg  config.Config
	svm  *swp.SwpClient
	db   *postgres.DB
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

func NewServer(cfg config.Config, svm *swp.SwpClient, db *postgres.DB, pub []byte, priv []byte) *Server {
	return &Server{
		cfg,
		svm,
		db,
		pub,
		priv,
	}
}

func (s *Server) Run() error {
	srv := &http.Server{
		Addr:         s.cfg.Addr,
		Handler:      s.mount(),
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("Server started at %s", srv.Addr)
	return srv.ListenAndServe()
}

func (s *Server) GenerateJWT() (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
		"iss": "bff-service",
		"aud": "eeapi",
		"exp": time.Now().Add(5 * time.Minute).Unix(),
		"iat": time.Now().Unix(),
	})

	signedToken, err := token.SignedString(s.priv)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}
