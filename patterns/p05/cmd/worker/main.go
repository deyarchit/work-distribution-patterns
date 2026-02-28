package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p05/internal/app"
	"work-distribution-patterns/shared/health"
)

type config struct {
	NATSURL          string `envconfig:"nats_url" default:"nats://127.0.0.1:4222"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
	HealthAddr       string `envconfig:"health_addr" default:":8082"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("Pattern 5 worker listening on NATS %s", cfg.NATSURL)
	health.StartServer(ctx, cfg.HealthAddr)

	app.RunWorker(ctx, app.WorkerConfig{
		NATSURL:          cfg.NATSURL,
		MaxStageDuration: cfg.MaxStageDuration,
	})
}
