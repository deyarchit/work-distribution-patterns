package main

import (
	"context"
	"log"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p05/internal/app"
)

type config struct {
	Addr        string `envconfig:"addr" default:":8081"`
	NATSURL     string `envconfig:"nats_url" default:"nats://127.0.0.1:4222"`
	DatabaseURL string `envconfig:"database_url" default:"postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	e, err := app.NewManager(ctx, app.ManagerConfig{
		NATSURL:     cfg.NATSURL,
		DatabaseURL: cfg.DatabaseURL,
	})
	if err != nil {
		log.Fatalf("setup: %v", err)
	}

	log.Printf("Pattern 5 (Queue-and-Store) Manager listening on %s", cfg.Addr)
	log.Fatal(e.Start(cfg.Addr))
}
