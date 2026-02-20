package bus

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.WorkerBus = (*NATSRedisBus)(nil)

const (
	progressPrefix   = "progress:"
	taskStatusPrefix = "task_status:"
)

// NATSRedisBus implements WorkerBus using NATS JetStream for task dispatch
// and Redis Pub/Sub for receiving progress and task status events.
// Workers publish directly to Redis, so every API replica's PSubscribe fires —
// giving each its own SSE delivery path without sticky sessions.
type NATSRedisBus struct {
	js       nats.JetStreamContext
	rdb      *redis.Client
	results  chan models.TaskStatusEvent
	progress chan models.ProgressEvent
}

// New creates a NATSRedisBus. Call Start to begin Redis Pub/Sub subscriptions.
func New(js nats.JetStreamContext, rdb *redis.Client) *NATSRedisBus {
	return &NATSRedisBus{
		js:       js,
		rdb:      rdb,
		results:  make(chan models.TaskStatusEvent, 256),
		progress: make(chan models.ProgressEvent, 256),
	}
}

// Start begins a Redis PSubscribe for progress:* and task_status:* channels.
// It is non-blocking; events are routed to internal channels.
func (b *NATSRedisBus) Start(ctx context.Context) error {
	pubsub := b.rdb.PSubscribe(ctx, progressPrefix+"*", taskStatusPrefix+"*")
	go func() {
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Printf("bus: pubsub close: %v", err)
			}
		}()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				switch {
				case strings.HasPrefix(msg.Channel, progressPrefix):
					var ev models.ProgressEvent
					if err := json.Unmarshal([]byte(msg.Payload), &ev); err == nil {
						select {
						case b.progress <- ev:
						default:
						}
					}
				case strings.HasPrefix(msg.Channel, taskStatusPrefix):
					var payload models.TaskStatusEvent
					if err := json.Unmarshal([]byte(msg.Payload), &payload); err == nil {
						b.results <- payload
					}
				}
			}
		}
	}()
	return nil
}

// Dispatch publishes the task to the "tasks.new" JetStream subject.
func (b *NATSRedisBus) Dispatch(_ context.Context, task models.Task) error {
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	if _, err := b.js.Publish("tasks.new", payload); err != nil {
		return err
	}
	return nil
}

// ReceiveResult blocks until a task status event is available or ctx is cancelled.
func (b *NATSRedisBus) ReceiveResult(ctx context.Context) (models.TaskStatusEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskStatusEvent{}, ctx.Err()
	case ev := <-b.results:
		return ev, nil
	}
}

// ReceiveProgress blocks until a progress event is available or ctx is cancelled.
func (b *NATSRedisBus) ReceiveProgress(ctx context.Context) (models.ProgressEvent, error) {
	select {
	case <-ctx.Done():
		return models.ProgressEvent{}, ctx.Err()
	case ev := <-b.progress:
		return ev, nil
	}
}
