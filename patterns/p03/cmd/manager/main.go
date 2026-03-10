package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p03/internal/app"
)

type config struct {
	Addr             string `envconfig:"addr" default:":8081"`
	WorkersQueueSize int    `envconfig:"workers_queue_size" default:"20"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		slog.Error("Config error", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	e, err := app.NewManager(ctx, app.ManagerConfig{WorkersQueueSize: cfg.WorkersQueueSize})
	if err != nil {
		slog.Error("Setup error", "error", err)
		os.Exit(1)
	}

	slog.Info("Pattern 3 (WebSocket Hub) Manager listening", "addr", cfg.Addr)
	if err := e.Start(cfg.Addr); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
