package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	restinternal "work-distribution-patterns/patterns/02-rest-polling/internal/rest"
	"work-distribution-patterns/shared/executor"
)

type config struct {
	ManagerURL       string `envconfig:"manager_url" default:"http://localhost:8081"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	source := restinternal.NewRESTConsumer(cfg.ManagerURL)
	_ = source.Connect(ctx)

	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	log.Printf("Pattern 2 (REST Polling) Worker connecting to %s", cfg.ManagerURL)

	for {
		task, err := source.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		go exec.Run(ctx, task, source)
	}
}
