package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

var stageNames = []string{
	"Initialization", "Validation", "Processing", "Transformation",
	"Aggregation", "Optimization", "Finalization", "Cleanup",
}

// NewTask constructs a Task with a generated ID, named stages, and pending status.
// stageCount is clamped to [1, 8].
func NewTask(name string, stageCount int) Task {
	if stageCount < 1 {
		stageCount = 1
	}
	if stageCount > len(stageNames) {
		stageCount = len(stageNames)
	}

	stages := make([]Stage, stageCount)
	for i := range stages {
		stageName := fmt.Sprintf("Stage %d", i+1)
		if i < len(stageNames) {
			stageName = stageNames[i]
		}
		stages[i] = Stage{
			Index:    i,
			Name:     stageName,
			Status:   StagePending,
			Progress: 0,
		}
	}

	return Task{
		ID:          uuid.New().String(),
		Name:        name,
		Status:      TaskPending,
		SubmittedAt: time.Now(),
		Stages:      stages,
	}
}

type TaskStatus string
type StageStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
)

const (
	StagePending   StageStatus = "pending"
	StageRunning   StageStatus = "running"
	StageCompleted StageStatus = "completed"
	StageFailed    StageStatus = "failed"
)

type Task struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Status       TaskStatus `json:"status"`
	SubmittedAt  time.Time  `json:"submittedAt"`
	DispatchedAt *time.Time `json:"dispatchedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	Stages       []Stage    `json:"stages"`
}

type Stage struct {
	Index    int         `json:"index"`
	Name     string      `json:"name"`
	Status   StageStatus `json:"status"`
	Progress int         `json:"progress"`
}

type ProgressEvent struct {
	TaskID    string      `json:"taskID"`
	StageIdx  int         `json:"stageIdx"`
	StageName string      `json:"stageName"`
	Progress  int         `json:"progress"`
	Status    StageStatus `json:"status"`
}

// TaskStatusEvent is the payload for task_status transport channels
// (NATS Core "task_status.<id>", Redis Pub/Sub "task_status:<id>").
type TaskStatusEvent struct {
	TaskID string     `json:"taskID"`
	Status TaskStatus `json:"status"`
}
