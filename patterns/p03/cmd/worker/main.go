package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	wsinternal "work-distribution-patterns/patterns/p03/internal/websocket"
	"work-distribution-patterns/shared/executor"
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

	consumer := wsinternal.NewWebSocketConsumer(cfg.ManagerURL)
	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	_ = consumer.Connect(ctx)

	health.StartServer(ctx, cfg.HealthAddr)

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		go exec.Run(ctx, task, consumer)
	}
}
