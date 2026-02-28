package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p03/internal/app"
	"work-distribution-patterns/shared/health"
)

type config struct {
	ManagerURL       string `envconfig:"manager_url" default:"ws://localhost:8081/ws/register"`
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

	log.Printf("Pattern 3 (WebSocket Hub) Worker connecting to %s", cfg.ManagerURL)
	health.StartServer(ctx, cfg.HealthAddr)

	app.RunWorker(ctx, app.WorkerConfig{
		ManagerWSURL:     cfg.ManagerURL,
		MaxStageDuration: cfg.MaxStageDuration,
	})
}
