package app

import (
	"context"
	"log/slog"
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
		slog.Error("NATS connect error", "error", err)
		return
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		slog.Error("JetStream error", "error", err)
		return
	}

	_ = natsinternal.SetupJetStream(js)

	consumer := natsinternal.NewNATSConsumer(nc, js)
	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	if err := consumer.Connect(ctx); err != nil {
		slog.Error("Connect error", "error", err)
		return
	}

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			return
		}
		// Synchronous: preserves NATS at-least-once delivery semantics.
		exec.Run(ctx, task, consumer)
	}
}
