package app

import (
	"context"
	"log"
	"time"

	pubsubinternal "work-distribution-patterns/patterns/p06/internal/pubsub"
	"work-distribution-patterns/shared/executor"
)

// WorkerConfig holds runtime parameters for the Pattern 6 worker process.
type WorkerConfig struct {
	BrokerURL        string
	MaxStageDuration int // milliseconds
}

// RunWorker connects to the broker and runs the worker loop.
// Blocks until ctx is cancelled.
func RunWorker(ctx context.Context, cfg WorkerConfig) {
	tasksSub, eventsTopic, err := pubsubinternal.OpenWorkerResources(ctx, cfg.BrokerURL)
	if err != nil {
		log.Printf("p06 worker: pubsub setup: %v", err)
		return
	}

	consumer := pubsubinternal.NewPubSubConsumer(tasksSub, eventsTopic)
	defer consumer.Shutdown(ctx)

	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	_ = consumer.Connect(ctx)

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			return
		}
		exec.Run(ctx, task, consumer)
	}
}
