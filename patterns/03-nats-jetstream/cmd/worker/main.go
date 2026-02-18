package main

import (
	"context"
	"encoding/json"
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

// natsSink publishes progress events back to the API via NATS Core subjects.
// All API replicas subscribe to these subjects, so all SSE hubs are updated.
type natsSink struct {
	nc *nats.Conn
}

func (s *natsSink) Publish(event models.ProgressEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	s.nc.Publish("progress."+event.TaskID, data)
}

func (s *natsSink) PublishTaskStatus(taskID string, status models.TaskStatus) {
	payload := struct {
		TaskID string           `json:"taskID"`
		Status models.TaskStatus `json:"status"`
	}{TaskID: taskID, Status: status}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	s.nc.Publish("task_status."+taskID, data)
}

func main() {
	natsURL      := envOr("NATS_URL", nats.DefaultURL)
	stageDurSecs := envInt("STAGE_DURATION_SECS", 3)

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

	// Idempotent setup (worker may start before API in some scenarios)
	if err := natsinternal.SetupJetStream(js); err != nil {
		log.Printf("setup warning: %v", err)
	}

	kv, err := js.KeyValue(natsinternal.KVBucket)
	if err != nil {
		log.Fatalf("open KV: %v", err)
	}

	taskStore := natsinternal.NewJetStreamStore(kv)
	sink      := &natsSink{nc: nc}
	exec      := &executor.Executor{StageDuration: time.Duration(stageDurSecs) * time.Second}

	// Durable queue subscriber — one message delivered to exactly one worker
	sub, err := js.QueueSubscribe(
		"tasks.new",
		natsinternal.ConsumerDur,
		func(msg *nats.Msg) {
			var task models.Task
			if err := json.Unmarshal(msg.Data, &task); err != nil {
				log.Printf("unmarshal task: %v", err)
				msg.Nak()
				return
			}
			log.Printf("executing task %s (%s, %d stages)", task.ID, task.Name, len(task.Stages))

			// Update status to running in KV
			taskStore.SetStatus(task.ID, models.TaskRunning)

			exec.Run(context.Background(), task, sink)

			// Update final status in KV
			taskStore.SetStatus(task.ID, models.TaskCompleted)

			// ACK only after full completion — crash before ACK → redelivery
			msg.Ack()
		},
		nats.Durable(natsinternal.ConsumerDur),
		nats.ManualAck(),
	)
	if err != nil {
		log.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	log.Printf("Pattern 3 worker listening on NATS %s", natsURL)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down worker")
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
