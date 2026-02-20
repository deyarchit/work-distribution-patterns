package contracts

import (
	"context"

	"work-distribution-patterns/shared/models"
)

// ProgressSink receives stage-level progress from the executor.
// Purely for UX — events are best-effort. Missed events are acceptable.
// Task-level status (running, completed, failed) belongs to ResultSink.
// TaskConsumer satisfies this interface, so it can be passed directly to the executor.
type ProgressSink interface {
	ReportProgress(ctx context.Context, event models.ProgressEvent) error
}
