package main

import (
	"context"
	"log"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p04/internal/app"
)

type config struct {
	Addr            string `envconfig:"addr" default:":8080"`
	ManagerGRPCAddr string `envconfig:"manager_grpc_addr" default:"localhost:9091"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	e, err := app.NewAPI(ctx, app.APIConfig{ManagerGRPCAddr: cfg.ManagerGRPCAddr})
	if err != nil {
		log.Fatalf("setup: %v", err)
	}

	log.Printf("Pattern 4 (gRPC Streaming) API listening on %s [manager=%s]", cfg.Addr, cfg.ManagerGRPCAddr)
	log.Fatal(e.Start(cfg.Addr))
}
