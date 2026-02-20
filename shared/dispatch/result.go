package dispatch

import "work-distribution-patterns/shared/models"

// ResultSink publishes task-level status transitions (running → completed/failed).
// Unlike ProgressSink, results must be delivered reliably — they determine final task state.
// Implementations may also route to a different backing store than the default TaskStore.
//
// Method is named Record (not Publish) to avoid a Go method-name collision with
// ProgressSink.Publish, allowing the same transport type to implement both interfaces.
type ResultSink interface {
	Record(taskID string, status models.TaskStatus) error
}
