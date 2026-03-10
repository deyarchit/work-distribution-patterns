package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p05/internal/app"
)

type config struct {
	Addr        string `envconfig:"addr" default:":8081"`
	NATSURL     string `envconfig:"nats_url" default:"nats://127.0.0.1:4222"`
	DatabaseURL string `envconfig:"database_url" default:"postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		slog.Error("Config error", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	e, err := app.NewManager(ctx, app.ManagerConfig{
		NATSURL:     cfg.NATSURL,
		DatabaseURL: cfg.DatabaseURL,
	})
	if err != nil {
		slog.Error("Setup error", "error", err)
		os.Exit(1)
	}

	slog.Info("Pattern 5 (Queue-and-Store) Manager listening", "addr", cfg.Addr)
	if err := e.Start(cfg.Addr); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
