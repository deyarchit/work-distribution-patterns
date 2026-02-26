package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log"

	"github.com/kelseyhightower/envconfig"

	pubsubinternal "work-distribution-patterns/patterns/p06/internal/pubsub"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/client"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
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

	// 1. Subscribe to broker events (Go Cloud PubSub)
	eventsSub, err := pubsubinternal.OpenAPIResources(ctx, cfg.BrokerURL)
	if err != nil {
		log.Fatalf("pubsub setup: %v", err)
	}
	defer func() { _ = eventsSub.Shutdown(ctx) }()

	// 2. Use RemoteTaskManager to proxy Submit/Get/List to the Manager
	taskManager := client.NewTaskManager(cfg.ManagerURL)
	hub := sse.NewHub()

	// 3. Subscribe to broker events via Go Cloud and pump to SSE Hub
	go func() {
		for {
			msg, err := eventsSub.Receive(ctx)
			if err != nil {
				log.Printf("events subscription error: %v", err)
				return
			}

			var ev models.TaskEvent
			if err := json.Unmarshal(msg.Body, &ev); err != nil {
				log.Printf("unmarshal event error: %v", err)
				msg.Ack()
				continue
			}

			msg.Ack()
			hub.Publish(ev)
		}
	}()

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(hub, tpl, taskManager)
	log.Printf("Pattern 06 (Cloud-Agnostic) API listening on %s [manager=%s, broker=%s]",
		cfg.Addr, cfg.ManagerURL, cfg.BrokerURL)
	log.Fatal(e.Start(cfg.Addr))
}
