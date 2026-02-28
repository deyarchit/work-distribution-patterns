package app

import (
	"context"
	"html/template"
	"time"

	"work-distribution-patterns/patterns/p01/internal/goroutine"
	"work-distribution-patterns/patterns/p01/internal/worker"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
	"work-distribution-patterns/shared/templates"

	"github.com/labstack/echo/v4"
)

// Config holds all runtime parameters for Pattern 1.
type Config struct {
	Workers          int
	QueueSize        int
	MaxStageDuration int // milliseconds
}

// New wires all Pattern 1 components and returns a configured Echo router.
// The caller is responsible for starting the server (e.g. e.Start(addr) or e.Server.Serve(ln)).
// Component goroutines (event pump, worker pool) are tied to ctx.
func New(ctx context.Context, cfg Config) (*echo.Echo, error) {
	bus := events.NewMemoryBridge()
	hub := sse.NewHub()
	taskStore := store.NewMemoryStore()
	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	dispatcher, consumer := goroutine.New(cfg.QueueSize)
	mgr := manager.New(taskStore, dispatcher, bus, 0)
	mgr.Start(ctx)

	ch, err := bus.Subscribe(ctx)
	if err != nil {
		return nil, err
	}
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
		return nil, err
	}

	return api.NewRouter(hub, tpl, mgr), nil
}
