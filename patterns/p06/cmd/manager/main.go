package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p06/internal/app"
)

type config struct {
	Addr        string `envconfig:"addr" default:":8081"`
	BrokerURL   string `envconfig:"broker_url" default:"nats://localhost:4222"`
	DatabaseURL string `envconfig:"database_url" default:"postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable"`
}

func main() {
	if err := run(); err != nil {
		slog.Error("Fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e, err := app.NewManager(ctx, app.ManagerConfig{
		BrokerURL:   cfg.BrokerURL,
		DatabaseURL: cfg.DatabaseURL,
	})
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	slog.Info("Pattern 06 (Cloud-Agnostic) Manager listening", "addr", cfg.Addr, "broker", cfg.BrokerURL)
	return e.Start(cfg.Addr)
}
