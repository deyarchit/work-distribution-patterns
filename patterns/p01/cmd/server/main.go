package main

import (
	"context"
	"html/template"
	"log"
	"time"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p01/internal/goroutine"
	"work-distribution-patterns/patterns/p01/internal/worker"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
	"work-distribution-patterns/shared/templates"
)

type config struct {
	Addr             string `envconfig:"addr" default:":8080"`
	Workers          int    `envconfig:"workers" default:"5"`
	QueueSize        int    `envconfig:"queue_size" default:"20"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	bus := events.NewMemoryEventBus()
	hub := sse.NewHub()
	taskStore := store.NewMemoryStore()
	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	dispatcher, consumer := goroutine.New(cfg.QueueSize)
	mgr := manager.New(taskStore, dispatcher, bus, 0) // deadline=0 disables re-dispatch
	mgr.Start(ctx)

	// Pump events from the bus into the local hub for the browser
	ch, _ := bus.Subscribe(ctx)
	go func() {
		for ev := range ch {
			hub.Publish(ev)
		}
	}()

	for i := 0; i < cfg.Workers; i++ {
		go worker.RunWorker(ctx, consumer, exec)
	}

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(hub, tpl, mgr)
	log.Printf("Pattern 1 (Goroutine Pool) listening on %s [workers=%d, queue=%d, maxStage=%s]",
		cfg.Addr, cfg.Workers, cfg.QueueSize, exec.MaxStageDuration)
	log.Fatal(e.Start(cfg.Addr))
}
