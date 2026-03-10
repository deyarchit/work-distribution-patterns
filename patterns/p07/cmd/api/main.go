package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p07/internal/app"
)

type config struct {
	Addr       string `envconfig:"addr" default:":8080"`
	ManagerURL string `envconfig:"manager_url" default:"http://localhost:8081"`
	BrokerURL  string `envconfig:"broker_url" default:"nats://localhost:4222"`
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

	e, err := app.NewAPI(ctx, app.APIConfig{
		ManagerURL: cfg.ManagerURL,
		BrokerURL:  cfg.BrokerURL,
	})
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	slog.Info("Pattern 07 (Bootstrap-Driven) API listening",
		"addr", cfg.Addr,
		"manager", cfg.ManagerURL,
		"broker", cfg.BrokerURL)
	return e.Start(cfg.Addr)
}
