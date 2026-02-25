package main

import (
	"context"
	"html/template"
	"log"

	"github.com/kelseyhightower/envconfig"

	grpcinternal "work-distribution-patterns/patterns/p04/internal/grpc"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
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

	// Connect to manager via gRPC
	taskManager, err := grpcinternal.NewClient(cfg.ManagerGRPCAddr)
	if err != nil {
		log.Fatalf("failed to connect to manager: %v", err)
	}
	defer func() {
		if err := taskManager.Close(); err != nil {
			log.Printf("error closing gRPC client: %v", err)
		}
	}()

	hub := sse.NewHub()

	// Subscribe to all events from manager via gRPC and pump into local SSE hub
	eventChan, err := taskManager.Subscribe(ctx, "")
	if err != nil {
		log.Fatalf("failed to subscribe to manager events: %v", err)
	}

	go func() {
		for ev := range eventChan {
			hub.Publish(ev)
		}
	}()

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(hub, tpl, taskManager)
	log.Printf("Pattern 4 (gRPC Streaming) API listening on %s [manager=%s]", cfg.Addr, cfg.ManagerGRPCAddr)
	log.Fatal(e.Start(cfg.Addr))
}
