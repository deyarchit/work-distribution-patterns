package contracts

import (
	"context"

	"work-distribution-patterns/shared/models"
)

// TaskConsumer is the worker-side transport view.
// Connect establishes the transport; Receive blocks for the next task;
// Emit sends a TaskEvent back to the manager (progress or status).
// TaskConsumer automatically satisfies EventSink via Emit.
type TaskConsumer interface {
	Connect(ctx context.Context) error
	Receive(ctx context.Context) (models.Task, error)
	Emit(ctx context.Context, event models.TaskEvent) error
}
