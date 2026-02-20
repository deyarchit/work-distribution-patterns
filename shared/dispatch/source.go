package dispatch

import (
	"context"

	"work-distribution-patterns/shared/models"
)

// TaskSource is the worker-side complement to TaskManager.
// Receive blocks until a task is available or ctx is cancelled.
// The returned sinks are transport-appropriate for the task:
//   - same instance for every call in P3/P4 (fixed per process)
//   - connection-scoped in P2
type TaskSource interface {
	Receive(ctx context.Context) (models.Task, ProgressSink, ResultSink, error)
}
