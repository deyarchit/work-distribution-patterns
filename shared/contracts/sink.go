package contracts

import "work-distribution-patterns/shared/models"

// ProgressSink receives stage-level progress from the executor.
// Purely for UX — events are best-effort. Missed events are acceptable.
// Task-level status (running, completed, failed) belongs to ResultSink.
type ProgressSink interface {
	Publish(event models.ProgressEvent)
}
