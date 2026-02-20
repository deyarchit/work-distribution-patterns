package contracts

import (
	"context"
	"errors"

	"work-distribution-patterns/shared/models"
)

// TaskProducer is the manager-side view of the execution substrate.
// Start sets up any transport subscriptions (non-blocking).
// Dispatch enqueues a task for a worker to process.
// ReceiveResult and ReceiveProgress block until an event arrives or ctx is cancelled.
type TaskProducer interface {
	Start(ctx context.Context) error
	Dispatch(ctx context.Context, task models.Task) error
	ReceiveResult(ctx context.Context) (models.TaskStatusEvent, error)
	ReceiveProgress(ctx context.Context) (models.ProgressEvent, error)
}

// Sentinel errors returned by Dispatch; Manager maps these to HTTP status codes.
var (
	ErrDispatchFull = errors.New("dispatch queue full")  // → 429
	ErrNoWorkers    = errors.New("no workers available") // → 503
)
