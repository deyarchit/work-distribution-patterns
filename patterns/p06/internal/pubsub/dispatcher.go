package pubsubinternal

import (
	"context"
	"encoding/json"
	"log"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"

	"gocloud.dev/pubsub"
)

// Compile-time interface check.
var _ dispatch.TaskDispatcher = (*PubSubDispatcher)(nil)

// PubSubDispatcher implements TaskDispatcher using Go Cloud PubSub topics and subscriptions.
type PubSubDispatcher struct {
	tasksTopic     *pubsub.Topic
	eventsSub      *pubsub.Subscription
	internalEvents chan models.TaskEvent
}

// NewPubSubDispatcher creates a PubSubDispatcher.
func NewPubSubDispatcher(tasksTopic *pubsub.Topic, eventsSub *pubsub.Subscription) *PubSubDispatcher {
	return &PubSubDispatcher{
		tasksTopic:     tasksTopic,
		eventsSub:      eventsSub,
		internalEvents: make(chan models.TaskEvent, 256),
	}
}

// Start launches a background goroutine to drain the events subscription.
func (d *PubSubDispatcher) Start(ctx context.Context) error {
	go d.receiveLoop(ctx)
	return nil
}

func (d *PubSubDispatcher) receiveLoop(ctx context.Context) {
	for {
		msg, err := d.eventsSub.Receive(ctx)
		if err != nil {
			// Context cancellation or subscription error.
			log.Printf("pubsub receive loop exit: %v", err)
			return
		}

		var ev models.TaskEvent
		if err := json.Unmarshal(msg.Body, &ev); err != nil {
			log.Printf("unmarshal worker event: %v", err)
			msg.Ack()
			continue
		}

		// Acknowledge receipt.
		msg.Ack()

		isTerminal := ev.Type == models.EventTaskStatus &&
			(ev.Status == string(models.TaskCompleted) || ev.Status == string(models.TaskFailed))

		if isTerminal {
			// Block briefly for terminal events to ensure they are processed.
			select {
			case d.internalEvents <- ev:
			case <-ctx.Done():
				return
			}
		} else {
			// Non-blocking for progress updates.
			select {
			case d.internalEvents <- ev:
			default:
			}
		}
	}
}

// Dispatch publishes the task to the tasks topic.
func (d *PubSubDispatcher) Dispatch(ctx context.Context, task models.Task) error {
	body, err := json.Marshal(task)
	if err != nil {
		return err
	}

	err = d.tasksTopic.Send(ctx, &pubsub.Message{
		Body: body,
		Metadata: map[string]string{
			"task_id": task.ID,
		},
	})
	if err != nil {
		log.Printf("pubsub dispatch error: %v", err)
		return err
	}
	return nil
}

// ReceiveEvent blocks until a worker event is available.
func (d *PubSubDispatcher) ReceiveEvent(ctx context.Context) (models.TaskEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskEvent{}, ctx.Err()
	case ev := <-d.internalEvents:
		return ev, nil
	}
}

// Shutdown closes the underlying Go Cloud resources.
func (d *PubSubDispatcher) Shutdown(ctx context.Context) {
	if d.tasksTopic != nil {
		_ = d.tasksTopic.Shutdown(ctx) //nolint:errcheck
	}
	if d.eventsSub != nil {
		_ = d.eventsSub.Shutdown(ctx) //nolint:errcheck
	}
}
