package main

import (
	"context"
	"html/template"
	"log"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/client"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
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

	taskManager := client.NewRemoteTaskManager(cfg.ManagerURL, nil)
	hub := sse.NewHub()

	// Pump manager SSE events into the local hub so browser clients connected
	// to this API process receive real-time progress updates.
	ch, _ := taskManager.Subscribe(ctx)
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
	log.Printf("Pattern 3 (WebSocket Hub) API listening on %s [manager=%s]", cfg.Addr, cfg.ManagerURL)
	log.Fatal(e.Start(cfg.Addr))
}
