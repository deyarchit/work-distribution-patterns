package natsinternal

import (
	"context"
	"encoding/json"
	"log"

	"github.com/nats-io/nats.go"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.WorkerBus = (*NATSBus)(nil)

// NATSBus implements WorkerBus using NATS JetStream for dispatch and
// NATS Core subjects for receiving progress and task status from workers.
// Every API replica subscribes to these subjects, so all SSE hubs receive
// all events regardless of which replica the browser is connected to.
type NATSBus struct {
	nc       *nats.Conn
	js       nats.JetStreamContext
	results  chan models.TaskStatusEvent
	progress chan models.ProgressEvent
}

// NewNATSBus creates a NATSBus. Call Start to register NATS Core subscriptions.
func NewNATSBus(nc *nats.Conn, js nats.JetStreamContext) *NATSBus {
	return &NATSBus{
		nc:       nc,
		js:       js,
		results:  make(chan models.TaskStatusEvent, 256),
		progress: make(chan models.ProgressEvent, 256),
	}
}

// Start registers NATS Core subscriptions for progress and task_status subjects.
// It is non-blocking; subscription callbacks push events into internal channels.
func (b *NATSBus) Start(_ context.Context) error {
	if _, err := b.nc.Subscribe("progress.*", func(msg *nats.Msg) {
		var ev models.ProgressEvent
		if err := json.Unmarshal(msg.Data, &ev); err == nil {
			select {
			case b.progress <- ev:
			default:
			}
		}
	}); err != nil {
		return err
	}

	if _, err := b.nc.Subscribe("task_status.*", func(msg *nats.Msg) {
		var payload models.TaskStatusEvent
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			b.results <- payload
		}
	}); err != nil {
		return err
	}

	return nil
}

// Dispatch publishes the task to the "tasks.new" JetStream subject.
func (b *NATSBus) Dispatch(_ context.Context, task models.Task) error {
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	if _, err := b.js.Publish("tasks.new", payload); err != nil {
		log.Printf("dispatch error: %v", err)
		return err
	}
	return nil
}

// ReceiveResult blocks until a task status event is available or ctx is cancelled.
func (b *NATSBus) ReceiveResult(ctx context.Context) (models.TaskStatusEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskStatusEvent{}, ctx.Err()
	case ev := <-b.results:
		return ev, nil
	}
}

// ReceiveProgress blocks until a progress event is available or ctx is cancelled.
func (b *NATSBus) ReceiveProgress(ctx context.Context) (models.ProgressEvent, error) {
	select {
	case <-ctx.Done():
		return models.ProgressEvent{}, ctx.Err()
	case ev := <-b.progress:
		return ev, nil
	}
}
