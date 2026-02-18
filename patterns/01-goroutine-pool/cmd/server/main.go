package main

import (
	"html/template"
	"log"
	"os"
	"strconv"
	"time"

	"work-distribution-patterns/patterns/01-goroutine-pool/internal/pool"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
	"work-distribution-patterns/shared/templates"
)

func main() {
	addr          := envOr("ADDR", ":8080")
	workers       := envInt("WORKERS", 5)
	queueSize     := envInt("QUEUE_SIZE", 20)
	stageDurSecs  := envInt("STAGE_DURATION_SECS", 3)

	hub := sse.NewHub()
	taskStore := store.NewMemoryStore()
	exec := &executor.Executor{StageDuration: time.Duration(stageDurSecs) * time.Second}
	p := pool.New(workers, queueSize)
	defer p.Stop()

	dispatcher := pool.NewPoolDispatcher(p, hub, exec)

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(taskStore, hub, tpl, dispatcher)
	log.Printf("Pattern 1 (Goroutine Pool) listening on %s [workers=%d, queue=%d, stageDur=%s]", addr, workers, queueSize, exec.StageDuration)
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
