package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	pubsubinternal "work-distribution-patterns/patterns/p06/internal/pubsub"
	"work-distribution-patterns/shared/executor"
)

type config struct {
	BrokerURL        string `envconfig:"broker_url" default:"nats://localhost:4222"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
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

	// Start a simple health check server
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		log.Printf("Worker health check listening on :8082")
		if err := http.ListenAndServe(":8082", mux); err != nil {
			log.Printf("health check server error: %v", err)
		}
	}()

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		exec.Run(ctx, task, consumer)
	}
}
