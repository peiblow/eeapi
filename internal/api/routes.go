package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/peiblow/eeapi/internal/api/handlers"
	"github.com/peiblow/eeapi/internal/auth"
	"github.com/peiblow/eeapi/internal/service"
	"github.com/peiblow/eeapi/internal/trigger"
)

func (s *Server) mount(mgr *trigger.EventManager) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	mgr.MountRoutes(r)

	behaviourSvc := service.NewBehaviourService(s.db)
	contractSvc := service.NewContractService(s.svm, s.rdb, s.db, behaviourSvc, s.priv, s.pub, s.locker, s.cfg.ArtifactDir)

	r.Route("/", func(r chi.Router) {
		r.Use(auth.JWTMiddleware(s.clientPub))

		r.Post("/contracts/deploy", handlers.DeployHandler(contractSvc))
		r.Post("/licenses", handlers.IssueLicenseHandler(s.licenseSvc))
		r.Post("/contracts/{id}/execute", handlers.ExecHandler(contractSvc))
		r.Get("/trace/{contextId}", handlers.TraceHandler(contractSvc))
		r.Get("/agent/{agentHash}/tools", handlers.GetAgentToolsHandler(contractSvc))
		r.Get("/agent/{agentHash}/definition", handlers.GetAgentDefinitionHandler(contractSvc))
	})

	return r
}
