package main

import (
	"context"
	"log"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p06/internal/app"
)

type config struct {
	Addr        string `envconfig:"addr" default:":8081"`
	BrokerURL   string `envconfig:"broker_url" default:"nats://localhost:4222"`
	DatabaseURL string `envconfig:"database_url" default:"postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e, err := app.NewManager(ctx, app.ManagerConfig{
		BrokerURL:   cfg.BrokerURL,
		DatabaseURL: cfg.DatabaseURL,
	})
	if err != nil {
		log.Fatalf("setup: %v", err)
	}

	log.Printf("Pattern 06 (Cloud-Agnostic) Manager listening on %s [broker=%s]", cfg.Addr, cfg.BrokerURL)
	log.Fatal(e.Start(cfg.Addr))
}
