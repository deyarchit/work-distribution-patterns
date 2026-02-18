package models

import "time"

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
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      TaskStatus `json:"status"`
	SubmittedAt time.Time  `json:"submittedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Stages      []Stage    `json:"stages"`
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

// TaskAssignment is the wire type used in Pattern 2 (API → worker over WebSocket).
type TaskAssignment struct {
	TaskID     string `json:"taskId"`
	Name       string `json:"name"`
	StageCount int    `json:"stageCount"`
}
