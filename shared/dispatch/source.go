package dispatch

import (
	"context"

	"work-distribution-patterns/shared/models"
)

// TaskSource is the worker-side complement to TaskManager.
// It connects to the task transport and delivers tasks one at a time.
// Receive blocks until a task is available or ctx is cancelled.
type TaskSource interface {
	Receive(ctx context.Context) (models.Task, error)
}
