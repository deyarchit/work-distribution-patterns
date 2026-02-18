package sse

import (
	"encoding/json"
	"sync"

	"work-distribution-patterns/shared/models"
)

type sseEvent struct {
	Type string `json:"type"`
	// For stage_progress events
	TaskID    string            `json:"taskID,omitempty"`
	StageIdx  int               `json:"stageIdx,omitempty"`
	StageName string            `json:"stageName,omitempty"`
	Progress  int               `json:"progress,omitempty"`
	Status    models.StageStatus `json:"status,omitempty"`
	// For task_status events (Status field reused as string below via separate struct)
}

type taskStatusEvent struct {
	Type   string           `json:"type"`
	TaskID string           `json:"taskID"`
	Status models.TaskStatus `json:"status"`
}

type stageProgressEvent struct {
	Type      string            `json:"type"`
	TaskID    string            `json:"taskID"`
	StageIdx  int               `json:"stageIdx"`
	StageName string            `json:"stageName"`
	Progress  int               `json:"progress"`
	Status    models.StageStatus `json:"status"`
}

// Hub manages SSE subscribers and broadcasts events to all of them.
type Hub struct {
	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[chan []byte]struct{}),
	}
}

// Subscribe returns a buffered channel that receives SSE event bytes and an
// unsubscribe function. The caller must call unsubscribe when done.
func (h *Hub) Subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		delete(h.subscribers, ch)
		h.mu.Unlock()
	}
	return ch, unsub
}

// Publish broadcasts a stage progress event to all subscribers.
func (h *Hub) Publish(event models.ProgressEvent) {
	ev := stageProgressEvent{
		Type:      "stage_progress",
		TaskID:    event.TaskID,
		StageIdx:  event.StageIdx,
		StageName: event.StageName,
		Progress:  event.Progress,
		Status:    event.Status,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.broadcast(data)
}

// PublishTaskStatus broadcasts a task status change event to all subscribers.
func (h *Hub) PublishTaskStatus(taskID string, status models.TaskStatus) {
	ev := taskStatusEvent{
		Type:   "task_status",
		TaskID: taskID,
		Status: status,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.broadcast(data)
}

func (h *Hub) broadcast(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subscribers {
		select {
		case ch <- data:
		default:
			// slow consumer — drop the event rather than blocking
		}
	}
}
