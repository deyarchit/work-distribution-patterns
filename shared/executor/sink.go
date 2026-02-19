package executor

import "work-distribution-patterns/shared/models"

// StatusWriter persists a task's final status.
// store.TaskStore satisfies this interface.
type StatusWriter interface {
	SetStatus(id string, status models.TaskStatus) error
}

// SinkWithStore wraps a ProgressSink and additionally persists the task's
// final status (completed or failed) to a StatusWriter. Intermediate
// transitions (running) are broadcast via SSE only and not stored.
type SinkWithStore struct {
	Inner ProgressSink
	Store StatusWriter
}

func (s *SinkWithStore) Publish(event models.ProgressEvent) {
	s.Inner.Publish(event)
}

func (s *SinkWithStore) PublishTaskStatus(taskID string, status models.TaskStatus) {
	s.Inner.PublishTaskStatus(taskID, status)
	if status == models.TaskCompleted || status == models.TaskFailed {
		_ = s.Store.SetStatus(taskID, status)
	}
}
