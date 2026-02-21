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
var _ dispatch.TaskProducer = (*NATSProducer)(nil)

// NATSProducer implements TaskProducer using NATS JetStream for dispatch and
// NATS Core for receiving all task events from workers via a single subject.
// Every API replica subscribes to task.events.*, so all SSE hubs receive
// all events regardless of which replica the browser is connected to.
type NATSProducer struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	events chan models.TaskEvent
}

// NewNATSProducer creates a NATSProducer. Call Start to register NATS Core subscriptions.
func NewNATSProducer(nc *nats.Conn, js nats.JetStreamContext) *NATSProducer {
	return &NATSProducer{
		nc:     nc,
		js:     js,
		events: make(chan models.TaskEvent, 256),
	}
}

// Start registers a single NATS Core subscription for all task events.
// It is non-blocking; the subscription callback pushes events into the internal channel.
func (b *NATSProducer) Start(_ context.Context) error {
	_, err := b.nc.Subscribe("task.events.*", func(msg *nats.Msg) {
		var ev models.TaskEvent
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			return
		}
		isTerminal := ev.Type == models.EventTaskStatus &&
			(ev.Status == string(models.TaskCompleted) || ev.Status == string(models.TaskFailed))
		if isTerminal {
			b.events <- ev // blocking: must not lose terminal status
		} else {
			select {
			case b.events <- ev:
			default:
			}
		}
	})
	return err
}

// Dispatch publishes the task to the "tasks.new" JetStream subject.
func (b *NATSProducer) Dispatch(_ context.Context, task models.Task) error {
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

// ReceiveEvent blocks until an event is available or ctx is cancelled.
func (b *NATSProducer) ReceiveEvent(ctx context.Context) (models.TaskEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskEvent{}, ctx.Err()
	case ev := <-b.events:
		return ev, nil
	}
}
