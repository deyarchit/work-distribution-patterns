package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"work-distribution-patterns/patterns/02-websocket-hub/internal/worker"
	"work-distribution-patterns/shared/executor"
)

func main() {
	apiURL     := envOr("API_URL", "ws://localhost:8080/ws/register")
	maxStageMs := envInt("MAX_STAGE_DURATION", 500)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	source := worker.NewWSTaskSource(apiURL)
	exec   := &executor.Executor{MaxStageDuration: time.Duration(maxStageMs) * time.Millisecond}

	go source.Connect(ctx)

	for {
		task, err := source.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		go exec.Run(ctx, task, source.Sink())
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
