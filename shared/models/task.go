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
		stages[i] = Stage{Index: i, Name: stageName}
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

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
)

// TaskEvent is the single ordered event type flowing from worker to manager.
// Type is either EventProgress or EventTaskStatus.
//
//   - EventProgress events carry StageName and Progress; they are ephemeral
//     (forwarded to SSE clients only, never persisted to the store).
//   - EventTaskStatus events carry Status; terminal statuses (completed/failed)
//     are persisted; "running" is forwarded only.
const (
	EventProgress   = "progress"
	EventTaskStatus = "task_status"
)

type TaskEvent struct {
	Type      string `json:"type"`
	TaskID    string `json:"taskID"`
	StageName string `json:"stageName,omitempty"` // EventProgress only
	Progress  int    `json:"progress,omitempty"`  // 0–100, EventProgress only
	Status    string `json:"status,omitempty"`    // EventTaskStatus only
}

type Task struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Status       TaskStatus `json:"status"`
	SubmittedAt  time.Time  `json:"submittedAt"`
	DispatchedAt *time.Time `json:"dispatchedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	Stages       []Stage    `json:"stages"`
}

// Stage records the name and index of a task stage.
// Progress is tracked ephemerally via TaskEvent; it is not persisted.
type Stage struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}
