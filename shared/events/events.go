package events

import (
	"context"
	"work-distribution-patterns/shared/models"
)

// TaskEventPublisher is the abstraction for broadcasting task events.
type TaskEventPublisher interface {
	// Publish broadcasts an event to all subscribers.
	Publish(event models.TaskEvent)
}

// TaskEventSubscriber is the abstraction for receiving task events.
type TaskEventSubscriber interface {
	// Subscribe returns a channel that receives live events.
	// The channel is closed when ctx is cancelled.
	Subscribe(ctx context.Context) (<-chan models.TaskEvent, error)
}

// TaskEventBridge is a unified interface for both publishing and subscribing.
// It acts as the cross-process or in-memory meeting point for event flow.
type TaskEventBridge interface {
	TaskEventPublisher
	TaskEventSubscriber
}
