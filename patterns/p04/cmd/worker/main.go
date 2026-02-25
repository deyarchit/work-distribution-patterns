package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	grpcinternal "work-distribution-patterns/patterns/p04/internal/grpc"
	"work-distribution-patterns/shared/executor"
)

type config struct {
	ManagerGRPCAddr  string `envconfig:"manager_grpc_addr" default:"localhost:9091"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	consumer := grpcinternal.NewConsumer(cfg.ManagerGRPCAddr)
	defer func() {
		if err := consumer.Close(); err != nil {
			log.Printf("error closing consumer: %v", err)
		}
	}()

	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	if err := consumer.Connect(ctx); err != nil {
		log.Fatalf("failed to connect to manager: %v", err)
	}

	log.Printf("Worker connected to manager at %s", cfg.ManagerGRPCAddr)

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}

		log.Printf("Received task %s, executing...", task.ID)
		go exec.Run(ctx, task, consumer)
	}
}
