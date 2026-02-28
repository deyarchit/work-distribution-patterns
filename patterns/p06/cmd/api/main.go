package main

import (
	"context"
	"fmt"
	"log"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p06/internal/app"
)

type config struct {
	Addr       string `envconfig:"addr" default:":8080"`
	ManagerURL string `envconfig:"manager_url" default:"http://localhost:8081"`
	BrokerURL  string `envconfig:"broker_url" default:"nats://localhost:4222"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e, err := app.NewAPI(ctx, app.APIConfig{
		ManagerURL: cfg.ManagerURL,
		BrokerURL:  cfg.BrokerURL,
	})
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	log.Printf("Pattern 06 (Cloud-Agnostic) API listening on %s [manager=%s, broker=%s]",
		cfg.Addr, cfg.ManagerURL, cfg.BrokerURL)
	return e.Start(cfg.Addr)
}
