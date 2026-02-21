package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	wsinternal "work-distribution-patterns/patterns/02-websocket-hub/internal/websocket"
	"work-distribution-patterns/shared/executor"
)

type config struct {
	APIURL           string `envconfig:"api_url" default:"ws://localhost:8080/ws/register"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	source := wsinternal.NewWebSocketConsumer(cfg.APIURL)
	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	_ = source.Connect(ctx)

	for {
		task, err := source.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		go exec.Run(ctx, task, source)
	}
}
