package main

import (
	"context"
	"html/template"
	"log"
	"os"
	"strconv"
	"time"

	"work-distribution-patterns/patterns/01-goroutine-pool/internal/bus"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
	"work-distribution-patterns/shared/templates"
)

func main() {
	addr := envOr("ADDR", ":8080")
	workers := envInt("WORKERS", 5)
	queueSize := envInt("QUEUE_SIZE", 20)
	maxStageMs := envInt("MAX_STAGE_DURATION", 500)

	ctx := context.Background()

	hub := sse.NewHub()
	taskStore := store.NewMemoryStore()
	exec := &executor.Executor{MaxStageDuration: time.Duration(maxStageMs) * time.Millisecond}

	channelBus := bus.New(queueSize)
	mgr := manager.New(taskStore, channelBus, hub, 0) // deadline=0 disables re-dispatch
	mgr.Start(ctx)

	for i := 0; i < workers; i++ {
		go bus.RunWorker(ctx, channelBus, exec)
	}

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(taskStore, hub, tpl, mgr)
	log.Printf("Pattern 1 (Goroutine Pool) listening on %s [workers=%d, queue=%d, maxStage=%s]",
		addr, workers, queueSize, exec.MaxStageDuration)
	log.Fatal(e.Start(addr))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
