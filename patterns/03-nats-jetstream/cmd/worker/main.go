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

	natsinternal "work-distribution-patterns/patterns/03-nats-jetstream/internal/nats"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
)

func main() {
	natsURL := envOr("NATS_URL", nats.DefaultURL)
	maxStageMs := envInt("MAX_STAGE_DURATION", 500)

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sink := natsinternal.NewNATSSink(nc)
	source := natsinternal.NewNATSTaskSource(js, sink)
	exec := &executor.Executor{MaxStageDuration: time.Duration(maxStageMs) * time.Millisecond}

	log.Printf("Pattern 3 worker listening on NATS %s", natsURL)

	go source.Connect(ctx)

	for {
		task, progressSink, resultSink, err := source.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		// Synchronous: exec.Run completes before we receive the next task,
		// preserving NATS at-least-once delivery (ACK happens in Connect after
		// the task is delivered to Receive).
		_ = resultSink.Record(task.ID, models.TaskRunning)
		status := exec.Run(ctx, task, progressSink)
		_ = resultSink.Record(task.ID, status)
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
