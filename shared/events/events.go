package events

import (
	"context"
	"work-distribution-patterns/shared/models"
)

// StoredEvent wraps a TaskEvent with an incremental sequence ID for polling.
type StoredEvent struct {
	ID    int64            `json:"id"`
	Event models.TaskEvent `json:"event"`
}

// TaskEventBus is the abstraction for publishing and receiving task events.
type TaskEventBus interface {
	// Publish broadcasts an event.
	Publish(event models.TaskEvent)

	// Subscribe returns a channel for live events. Used by Pattern 1 and SSE streaming.
	Subscribe(ctx context.Context) (<-chan models.TaskEvent, error)

	// Poll returns events after the given ID. If none are available, it waits
	// until ctx is cancelled or a new event arrives. Used by /events/poll endpoint.
	Poll(ctx context.Context, afterID int64) ([]StoredEvent, error)
}
