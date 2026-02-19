package main

import (
	"encoding/json"
	"html/template"
	"log"
	"os"

	"github.com/nats-io/nats.go"

	natsinternal "work-distribution-patterns/patterns/03-nats-jetstream/internal/nats"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
)

func main() {
	addr    := envOr("ADDR", ":8080")
	natsURL := envOr("NATS_URL", nats.DefaultURL)

	// Connect to NATS with automatic reconnect
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

	// Idempotent setup
	if err := natsinternal.SetupJetStream(js); err != nil {
		log.Printf("setup warning: %v", err)
	}

	kv, err := js.KeyValue(natsinternal.KVBucket)
	if err != nil {
		log.Fatalf("open KV: %v", err)
	}

	hub       := sse.NewHub()
	taskStore := natsinternal.NewJetStreamStore(kv)
	manager   := natsinternal.NewNATSTaskManager(js, taskStore)

	// Subscribe to all progress events on NATS Core and forward to SSE hub.
	// Every API replica does this, so all SSE hubs receive all events regardless
	// of which replica the browser is connected to — no sticky sessions needed.
	nc.Subscribe("progress.*", func(msg *nats.Msg) {
		var ev models.ProgressEvent
		if err := json.Unmarshal(msg.Data, &ev); err == nil {
			hub.Publish(ev)
		}
	})
	// task_status.* events carry terminal and intermediate status from workers.
	// Both the SSE hub and the task store are updated here; workers never touch the store.
	nc.Subscribe("task_status.*", func(msg *nats.Msg) {
		var payload struct {
			TaskID string            `json:"taskID"`
			Status models.TaskStatus `json:"status"`
		}
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			hub.PublishTaskStatus(payload.TaskID, payload.Status)
			_ = taskStore.SetStatus(payload.TaskID, payload.Status)
		}
	})

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(taskStore, hub, tpl, manager)
	log.Printf("Pattern 3 (NATS JetStream) API listening on %s", addr)
	log.Fatal(e.Start(addr))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
