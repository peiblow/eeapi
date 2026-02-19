package main

import (
	"log/slog"
	"os"

	"github.com/peiblow/eeapi/internal/api"
	"github.com/peiblow/eeapi/internal/config"
	"github.com/peiblow/eeapi/internal/database/postgres"
	"github.com/peiblow/eeapi/internal/keys"
	"github.com/peiblow/eeapi/internal/swp"
)

func main() {
	svm := swp.NewSwpClient("localhost:8332")
	defer svm.Close()

	if err := svm.Connect(); err != nil {
		slog.Error("Failed to connect to SVM server", "error", err)
		os.Exit(1)
	}

	slog.Info("-> Connected to SVM server!")

	db, err := postgres.Open()
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("-> Connected to database!")

	cfg := config.Config{
		Addr: ":8080",
		DB:   config.DBConfig{},
	}

	pub, priv, err := keys.LoadOrCreateKeys("keysStore/keys.dat")
	if err != nil {
		slog.Error("Failed to load or create keys", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	server := api.NewServer(cfg, svm, db, pub, priv)

	if err := server.Run(); err != nil {
		slog.Error("Server failed to start", "error", err)
		os.Exit(1)
	}
}
