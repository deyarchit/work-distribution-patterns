package contracts

import (
	"context"

	"work-distribution-patterns/shared/models"
)

// WorkerSource is the worker-side view of the execution substrate.
// Connect sets up any transport subscriptions (non-blocking).
// Receive blocks until a task is available or ctx is cancelled.
// ReportResult and ReportProgress send status and progress back to the manager.
type WorkerSource interface {
	Connect(ctx context.Context) error
	Receive(ctx context.Context) (models.Task, error)
	ReportResult(ctx context.Context, taskID string, status models.TaskStatus) error
	ReportProgress(ctx context.Context, event models.ProgressEvent) error
}
