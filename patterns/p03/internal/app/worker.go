package app

import (
	"context"
	"time"

	wsinternal "work-distribution-patterns/patterns/p03/internal/websocket"
	"work-distribution-patterns/shared/executor"
)

// WorkerConfig holds runtime parameters for the Pattern 3 worker process.
type WorkerConfig struct {
	// ManagerWSURL is the full WebSocket URL for worker registration,
	// e.g. "ws://localhost:8081/ws/register".
	ManagerWSURL     string
	MaxStageDuration int // milliseconds
}

// RunWorker connects to the manager via WebSocket and runs the worker loop.
// Blocks until ctx is cancelled.
func RunWorker(ctx context.Context, cfg WorkerConfig) {
	consumer := wsinternal.NewWebSocketConsumer(cfg.ManagerWSURL)
	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	_ = consumer.Connect(ctx)

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			return
		}
		go exec.Run(ctx, task, consumer)
	}
}
