package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p06/internal/app"
	"work-distribution-patterns/shared/health"
)

type config struct {
	BrokerURL        string `envconfig:"broker_url" default:"nats://localhost:4222"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
	HealthAddr       string `envconfig:"health_addr" default:":8082"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		slog.Error("Config error", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("Pattern 06 Worker starting", "broker", cfg.BrokerURL)
	health.StartServer(ctx, cfg.HealthAddr)

	app.RunWorker(ctx, app.WorkerConfig{
		BrokerURL:        cfg.BrokerURL,
		MaxStageDuration: cfg.MaxStageDuration,
	})
}
