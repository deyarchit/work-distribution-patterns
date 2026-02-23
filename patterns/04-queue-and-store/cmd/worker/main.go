package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"

	natsinternal "work-distribution-patterns/patterns/04-queue-and-store/internal/nats"
	"work-distribution-patterns/shared/executor"
)

type config struct {
	NATSURL          string `envconfig:"nats_url" default:"nats://127.0.0.1:4222"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	nc, err := nats.Connect(cfg.NATSURL,
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
	)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	if err := natsinternal.SetupJetStream(js); err != nil {
		log.Printf("setup warning: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	source := natsinternal.NewNATSConsumer(nc, js)
	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	log.Printf("Pattern 4 worker listening on NATS %s", cfg.NATSURL)

	_ = source.Connect(ctx)

	for {
		task, err := source.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		// Synchronous: exec.Run completes before we receive the next task,
		// preserving NATS at-least-once delivery (ACK happens in Connect after
		// the task is delivered to Receive).
		exec.Run(ctx, task, source)
	}
}
