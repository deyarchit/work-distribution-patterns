package main

import (
	"context"
	"html/template"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	redisbus "work-distribution-patterns/patterns/04-nats-redis/internal/bus"
	natsinternal "work-distribution-patterns/patterns/04-nats-redis/internal/nats"
	redisinternal "work-distribution-patterns/patterns/04-nats-redis/internal/redis"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
)

func main() {
	addr := envOr("ADDR", ":8080")
	natsURL := envOr("NATS_URL", nats.DefaultURL)
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")

	// Connect to NATS for JetStream task submission
	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
	)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	if err := natsinternal.SetupJetStream(js); err != nil {
		log.Printf("setup warning: %v", err)
	}

	// Connect to Redis for task store and SSE fan-out
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis connect: %v", err)
	}

	hub := sse.NewHub()
	taskStore := redisinternal.NewRedisTaskStore(rdb)
	workerBus := redisbus.New(js, rdb)
	mgr := manager.New(taskStore, workerBus, hub, 30*time.Second)
	mgr.Start(ctx)

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(taskStore, hub, tpl, mgr)
	log.Printf("Pattern 4 (NATS + Redis) API listening on %s", addr)
	log.Fatal(e.Start(addr))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
