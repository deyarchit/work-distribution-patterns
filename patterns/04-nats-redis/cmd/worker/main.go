package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	natsinternal "work-distribution-patterns/patterns/04-nats-redis/internal/nats"
	redisinternal "work-distribution-patterns/patterns/04-nats-redis/internal/redis"
	"work-distribution-patterns/shared/executor"
)

func main() {
	natsURL := envOr("NATS_URL", nats.DefaultURL)
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")
	maxStageMs := envInt("MAX_STAGE_DURATION", 500)

	// NATS: receive tasks from the queue (at-least-once via JetStream)
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

	// Redis: publish progress events so all API replicas can fan-out to SSE
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis connect: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	source := natsinternal.NewNATSTaskSource(js)
	sink := redisinternal.NewRedisSink(rdb)
	exec := &executor.Executor{MaxStageDuration: time.Duration(maxStageMs) * time.Millisecond}

	log.Printf("Pattern 4 worker: NATS %s | Redis %s", natsURL, redisAddr)

	go source.Connect(ctx)

	for {
		task, err := source.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		// Synchronous: exec.Run completes before we receive the next task,
		// preserving NATS at-least-once delivery (ACK happens in Connect after
		// the task is delivered to Receive).
		exec.Run(ctx, task, sink)
	}
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
