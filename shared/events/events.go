package events

import (
	"context"
	"work-distribution-patterns/shared/models"
)

// TaskEventBus is the abstraction for publishing and receiving task events.
// All implementations use native pub/sub (channels, SSE, NATS) - no polling.
type TaskEventBus interface {
	// Publish broadcasts an event to all subscribers.
	Publish(event models.TaskEvent)

	// Subscribe returns a channel that receives live events.
	// The channel is closed when ctx is cancelled.
	Subscribe(ctx context.Context) (<-chan models.TaskEvent, error)
}
