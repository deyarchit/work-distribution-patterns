package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	pubsubinternal "work-distribution-patterns/patterns/p06/internal/pubsub"
	"work-distribution-patterns/shared/executor"
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

	// 1. Setup Transport (Go Cloud PubSub)
	tasksSub, eventsTopic, err := pubsubinternal.OpenWorkerResources(ctx, cfg.BrokerURL)
	if err != nil {
		log.Fatalf("pubsub setup: %v", err)
	}

	consumer := pubsubinternal.NewPubSubConsumer(tasksSub, eventsTopic)
	defer consumer.Shutdown(ctx)

	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	// 2. Run Worker Loop
	log.Printf("Pattern 06 Worker starting [broker=%s]", cfg.BrokerURL)
	_ = consumer.Connect(ctx)

	health.StartServer(ctx, cfg.HealthAddr)

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		exec.Run(ctx, task, consumer)
	}
}
