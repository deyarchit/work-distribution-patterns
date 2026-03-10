package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p02/internal/app"
)

type config struct {
	Addr       string `envconfig:"addr" default:":8080"`
	ManagerURL string `envconfig:"manager_url" default:"http://localhost:8081"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		slog.Error("Config error", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	e, err := app.NewAPI(ctx, app.APIConfig{ManagerURL: cfg.ManagerURL})
	if err != nil {
		slog.Error("Setup error", "error", err)
		os.Exit(1)
	}

	slog.Info("Pattern 2 (REST Polling) API listening", "addr", cfg.Addr, "manager", cfg.ManagerURL)
	if err := e.Start(cfg.Addr); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
