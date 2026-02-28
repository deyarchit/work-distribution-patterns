package main

import (
	"context"
	"log"
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
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("Pattern 06 Worker starting [broker=%s]", cfg.BrokerURL)
	health.StartServer(ctx, cfg.HealthAddr)

	app.RunWorker(ctx, app.WorkerConfig{
		BrokerURL:        cfg.BrokerURL,
		MaxStageDuration: cfg.MaxStageDuration,
	})
}
