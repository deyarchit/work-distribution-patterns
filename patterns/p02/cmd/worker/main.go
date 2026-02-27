package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	restinternal "work-distribution-patterns/patterns/p02/internal/rest"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/health"
)

type config struct {
	ManagerURL       string `envconfig:"manager_url" default:"http://localhost:8081"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
	HealthAddr       string `envconfig:"health_addr" default:":8082"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	consumer := restinternal.NewRESTConsumer(cfg.ManagerURL)
	_ = consumer.Connect(ctx)

	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	log.Printf("Pattern 2 (REST Polling) Worker connecting to %s", cfg.ManagerURL)

	health.StartServer(ctx, cfg.HealthAddr)

	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		go exec.Run(ctx, task, consumer)
	}
}
