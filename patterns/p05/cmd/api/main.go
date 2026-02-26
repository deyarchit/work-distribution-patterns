package main

import (
	"context"
	"html/template"
	"log"

	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"

	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/client"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
)

type config struct {
	Addr       string `envconfig:"addr" default:":8080"`
	ManagerURL string `envconfig:"manager_url" default:"http://localhost:8081"`
	NATSURL    string `envconfig:"nats_url" default:"nats://localhost:4222"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()

	taskManager := client.NewTaskManager(cfg.ManagerURL)
	hub := sse.NewHub()

	// Subscribe directly to NATS for events (distributed pub/sub).
	bus := events.NewNATSBridge(nc, "task.events")
	ch, _ := bus.Subscribe(ctx)
	go func() {
		for ev := range ch {
			hub.Publish(ev)
		}
	}()

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(hub, tpl, taskManager)
	log.Printf("Pattern 5 (Queue-and-Store) API listening on %s [manager=%s]", cfg.Addr, cfg.ManagerURL)
	log.Fatal(e.Start(cfg.Addr))
}
