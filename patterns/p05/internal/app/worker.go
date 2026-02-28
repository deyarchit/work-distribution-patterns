package app

import (
	"context"
	"log"
	"time"

	"github.com/nats-io/nats.go"

	natsinternal "work-distribution-patterns/patterns/p05/internal/nats"
	"work-distribution-patterns/shared/executor"
)

// WorkerConfig holds runtime parameters for the Pattern 5 worker process.
type WorkerConfig struct {
	NATSURL          string
	MaxStageDuration int // milliseconds
}

// RunWorker connects to NATS and runs the worker loop.
// Blocks until ctx is cancelled.
func RunWorker(ctx context.Context, cfg WorkerConfig) {
	nc, err := nats.Connect(cfg.NATSURL,
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
	)
	if err != nil {
		log.Printf("p05 worker: nats connect: %v", err)
		return
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Printf("p05 worker: jetstream: %v", err)
		return
	}

	if err := natsinternal.SetupJetStream(js); err != nil {
		// Non-fatal: streams may already exist.
		_ = err
	}

	consumer := natsinternal.NewNATSConsumer(nc, js)
	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	_ = consumer.Connect(ctx)

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			return
		}
		// Synchronous: preserves NATS at-least-once delivery semantics.
		exec.Run(ctx, task, consumer)
	}
}
