package api

import (
	"context"
	"crypto/ed25519"
	"log"
	"net/http"
	"time"

	"github.com/peiblow/eeapi/internal/config"
	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/database/redis"
	"github.com/peiblow/eeapi/internal/service"
	"github.com/peiblow/eeapi/internal/swp"
	"github.com/peiblow/eeapi/internal/trigger"
)

type Server struct {
	cfg       config.Config
	svm       *swp.SwpClient
	db        *postgres.DB
	rdb       *redis.Client
	pub       ed25519.PublicKey
	priv      ed25519.PrivateKey
	clientPub ed25519.PublicKey

	locker *config.ContractLocker
}

func NewServer(cfg config.Config, svm *swp.SwpClient, db *postgres.DB, rdb *redis.Client, pub []byte, priv []byte, clientPub []byte, locker *config.ContractLocker) *Server {
	return &Server{
		cfg:       cfg,
		svm:       svm,
		db:        db,
		rdb:       rdb,
		pub:       pub,
		priv:      priv,
		clientPub: clientPub,
		locker:    locker,
	}
}

func (s *Server) Run() error {
	eventSvc := service.NewEventService(s.rdb, s.db)
	mgr := trigger.NewEventManager(s.rdb, eventSvc)
	mgr.Start(context.Background())
	defer mgr.Stop()

	srv := &http.Server{
		Addr:         s.cfg.Addr,
		Handler:      s.mount(mgr),
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("Server started at %s", srv.Addr)
	return srv.ListenAndServe()
}
