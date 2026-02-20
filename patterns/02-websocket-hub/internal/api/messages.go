package api

import "work-distribution-patterns/shared/models"

// Message types for the WebSocket protocol between API and workers.
const (
	MsgTypeReady    = "ready"
	MsgTypeTask     = "task"
	MsgTypeProgress = "progress"
	MsgTypeDone     = "done"
	MsgTypeStatus   = "status" // non-terminal task status (e.g. "running")
)

// ReadyMsg is sent by the worker to signal it is available.
type ReadyMsg struct {
	Type string `json:"type"` // "ready"
}

// TaskMsg is sent by the API to assign work to a worker.
// The full Task is sent so the worker does not need to reconstruct any fields.
type TaskMsg struct {
	Type string      `json:"type"` // "task"
	Task models.Task `json:"task"`
}

// ProgressMsg is sent by the worker with a stage progress event.
type ProgressMsg struct {
	Type  string               `json:"type"` // "progress"
	Event models.ProgressEvent `json:"event"`
}

// StatusMsg carries a non-terminal task status from worker to API.
type StatusMsg struct {
	Type   string            `json:"type"` // "status"
	TaskID string            `json:"taskId"`
	Status models.TaskStatus `json:"status"`
}

// DoneMsg carries the terminal task status from worker to API.
type DoneMsg struct {
	Type   string            `json:"type"` // "done"
	TaskID string            `json:"taskId"`
	Status models.TaskStatus `json:"status"`
}

// GenericMsg is used to decode the type field before further unmarshaling.
type GenericMsg struct {
	Type string `json:"type"`
}
