package main

import (
	"context"
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
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e, err := app.NewAPI(ctx, app.APIConfig{
		ManagerURL: cfg.ManagerURL,
		BrokerURL:  cfg.BrokerURL,
	})
	if err != nil {
		log.Fatalf("setup: %v", err)
	}

	log.Printf("Pattern 06 (Cloud-Agnostic) API listening on %s [manager=%s, broker=%s]",
		cfg.Addr, cfg.ManagerURL, cfg.BrokerURL)
	log.Fatal(e.Start(cfg.Addr))
}
