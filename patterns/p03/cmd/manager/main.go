package main

import (
	"context"
	"log"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p03/internal/app"
)

type config struct {
	Addr             string `envconfig:"addr" default:":8081"`
	WorkersQueueSize int    `envconfig:"workers_queue_size" default:"20"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	e, err := app.NewManager(ctx, app.ManagerConfig{WorkersQueueSize: cfg.WorkersQueueSize})
	if err != nil {
		log.Fatalf("setup: %v", err)
	}

	log.Printf("Pattern 3 (WebSocket Hub) Manager listening on %s", cfg.Addr)
	log.Fatal(e.Start(cfg.Addr))
}
