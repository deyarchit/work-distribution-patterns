package app

import (
	"context"
	"log/slog"
	"time"

	grpcinternal "work-distribution-patterns/patterns/p04/internal/grpc"
	"work-distribution-patterns/shared/executor"
)

// WorkerConfig holds runtime parameters for the Pattern 4 worker process.
type WorkerConfig struct {
	ManagerGRPCAddr  string
	MaxStageDuration int // milliseconds
}

// RunWorker connects to the manager via gRPC and runs the worker loop.
// Blocks until ctx is cancelled.
func RunWorker(ctx context.Context, cfg WorkerConfig) {
	consumer := grpcinternal.NewConsumer(cfg.ManagerGRPCAddr)
	defer func() {
		if err := consumer.Close(); err != nil {
			slog.Error("Close consumer error", "error", err)
		}
	}()

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
		go exec.Run(ctx, task, consumer)
	}
}
