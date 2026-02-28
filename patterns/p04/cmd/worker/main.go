package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p04/internal/app"
	"work-distribution-patterns/shared/health"
)

type config struct {
	ManagerGRPCAddr  string `envconfig:"manager_grpc_addr" default:"localhost:9091"`
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

	log.Printf("Pattern 4 (gRPC Streaming) Worker connecting to %s", cfg.ManagerGRPCAddr)
	health.StartServer(ctx, cfg.HealthAddr)

	app.RunWorker(ctx, app.WorkerConfig{
		ManagerGRPCAddr:  cfg.ManagerGRPCAddr,
		MaxStageDuration: cfg.MaxStageDuration,
	})
}
