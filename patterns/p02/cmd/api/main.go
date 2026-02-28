package main

import (
	"context"
	"log"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p02/internal/app"
)

type config struct {
	Addr       string `envconfig:"addr" default:":8080"`
	ManagerURL string `envconfig:"manager_url" default:"http://localhost:8081"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	e, err := app.NewAPI(ctx, app.APIConfig{ManagerURL: cfg.ManagerURL})
	if err != nil {
		log.Fatalf("setup: %v", err)
	}

	log.Printf("Pattern 2 (REST Polling) API listening on %s [manager=%s]", cfg.Addr, cfg.ManagerURL)
	log.Fatal(e.Start(cfg.Addr))
}
