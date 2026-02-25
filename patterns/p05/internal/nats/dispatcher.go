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
var _ dispatch.TaskDispatcher = (*NATSDispatcher)(nil)

// NATSDispatcher implements TaskDispatcher using NATS JetStream for dispatch and
// NATS Core for receiving all task events from workers via a single subject.
// Every API replica subscribes to task.events.*, but workers emit to
// worker.events.*. The Manager acts as a gateway, processing worker events
// before republishing them to the API-facing subject.
type NATSDispatcher struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	events chan models.TaskEvent
}

// NewNATSDispatcher creates a NATSDispatcher. Call Start to register NATS Core subscriptions.
func NewNATSDispatcher(nc *nats.Conn, js nats.JetStreamContext) *NATSDispatcher {
	return &NATSDispatcher{
		nc:     nc,
		js:     js,
		events: make(chan models.TaskEvent, 256),
	}
}

// Start registers a single NATS Core subscription for all worker events.
// It is non-blocking; the subscription callback pushes events into the internal channel.
func (b *NATSDispatcher) Start(_ context.Context) error {
	_, err := b.nc.Subscribe("worker.events.*", func(msg *nats.Msg) {
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
func (b *NATSDispatcher) Dispatch(_ context.Context, task models.Task) error {
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
func (b *NATSDispatcher) ReceiveEvent(ctx context.Context) (models.TaskEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskEvent{}, ctx.Err()
	case ev := <-b.events:
		return ev, nil
	}
}
