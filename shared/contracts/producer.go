package contracts

import (
	"context"
	"errors"

	"work-distribution-patterns/shared/models"
)

// TaskDispatcher is the manager-side view of the execution substrate.
// Start sets up any transport subscriptions (non-blocking).
// Dispatch enqueues a task for a worker to process.
// ReceiveEvent blocks until an event arrives or ctx is cancelled.
type TaskDispatcher interface {
	Start(ctx context.Context) error
	Dispatch(ctx context.Context, task models.Task) error
	ReceiveEvent(ctx context.Context) (models.TaskEvent, error)
}

// Sentinel errors returned by Dispatch; Manager maps these to HTTP status codes.
var (
	ErrDispatchFull = errors.New("dispatch queue full")  // → 429
	ErrNoWorkers    = errors.New("no workers available") // → 503
)
