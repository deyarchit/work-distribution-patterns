package store

import "work-distribution-patterns/shared/models"

// TaskStore is the persistence interface for tasks.
type TaskStore interface {
	Create(task models.Task) error
	Get(id string) (models.Task, bool)
	List() []models.Task
	SetStatus(id string, status models.TaskStatus) error
	UpdateStage(id string, idx int, stage models.Stage) error
}
