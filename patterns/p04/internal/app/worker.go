package app

import (
	"context"
	"log"
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
			log.Printf("p04 worker: close consumer: %v", err)
		}
	}()

	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	if err := consumer.Connect(ctx); err != nil {
		log.Printf("p04 worker: connect: %v", err)
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
