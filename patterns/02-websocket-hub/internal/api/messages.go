package api

import "work-distribution-patterns/shared/models"

// Message types for the WebSocket protocol between API and workers.
const (
	MsgTypeReady    = "ready"
	MsgTypeTask     = "task"
	MsgTypeProgress = "progress"
	MsgTypeDone     = "done"
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

// DoneMsg is sent by the worker when the task is fully complete.
type DoneMsg struct {
	Type   string `json:"type"` // "done"
	TaskID string `json:"taskId"`
}

// GenericMsg is used to decode the type field before further unmarshaling.
type GenericMsg struct {
	Type string `json:"type"`
}
