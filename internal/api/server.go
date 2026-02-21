package api

import (
	"crypto/ed25519"
	"log"
	"net/http"
	"time"

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
