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
var _ dispatch.TaskConsumer = (*PubSubConsumer)(nil)

// PubSubConsumer implements TaskConsumer over Go Cloud PubSub.
type PubSubConsumer struct {
	tasksSub    *pubsub.Subscription
	eventsTopic *pubsub.Topic
}

// NewPubSubConsumer creates a PubSubConsumer.
func NewPubSubConsumer(tasksSub *pubsub.Subscription, eventsTopic *pubsub.Topic) *PubSubConsumer {
	return &PubSubConsumer{
		tasksSub:    tasksSub,
		eventsTopic: eventsTopic,
	}
}

// Connect is a no-op as Go Cloud handles connection state internally.
func (c *PubSubConsumer) Connect(_ context.Context) error {
	return nil
}

// Receive blocks until a task is available from the subscription.
func (c *PubSubConsumer) Receive(ctx context.Context) (models.Task, error) {
	msg, err := c.tasksSub.Receive(ctx)
	if err != nil {
		return models.Task{}, err
	}

	var task models.Task
	if err := json.Unmarshal(msg.Body, &task); err != nil {
		log.Printf("unmarshal task error: %v", err)
		msg.Ack() // Ack bad messages so they don't clog the queue.
		return c.Receive(ctx)
	}

	// For work distribution, we acknowledge after receiving.
	// In a real system, we might want to use at-least-once delivery with
	// acknowledgement only after successful processing, but following
	// the pattern in P5 for consistency.
	msg.Ack()
	return task, nil
}

// Emit publishes a TaskEvent to the events topic.
func (c *PubSubConsumer) Emit(ctx context.Context, event models.TaskEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return c.eventsTopic.Send(ctx, &pubsub.Message{
		Body: body,
		Metadata: map[string]string{
			"task_id": event.TaskID,
			"type":    event.Type,
		},
	})
}

// Shutdown closes the underlying Go Cloud resources.
func (c *PubSubConsumer) Shutdown(ctx context.Context) {
	if c.tasksSub != nil {
		_ = c.tasksSub.Shutdown(ctx)
	}
	if c.eventsTopic != nil {
		_ = c.eventsTopic.Shutdown(ctx)
	}
}
