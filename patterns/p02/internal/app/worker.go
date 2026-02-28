package app

import (
	"context"
	"time"

	restinternal "work-distribution-patterns/patterns/p02/internal/rest"
	"work-distribution-patterns/shared/executor"
)

// WorkerConfig holds runtime parameters for the Pattern 2 worker process.
type WorkerConfig struct {
	ManagerURL       string
	MaxStageDuration int // milliseconds
}

// RunWorker connects to the manager and runs the REST polling worker loop.
// Blocks until ctx is cancelled.
func RunWorker(ctx context.Context, cfg WorkerConfig) {
	consumer := restinternal.NewRESTConsumer(cfg.ManagerURL)
	_ = consumer.Connect(ctx)

	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			return
		}
		go exec.Run(ctx, task, consumer)
	}
}
