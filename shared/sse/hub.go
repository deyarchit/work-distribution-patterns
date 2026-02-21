package sse

import (
	"encoding/json"
	"sync"

	"work-distribution-patterns/shared/models"
)

// Hub manages SSE subscribers and broadcasts events to task-scoped or global subscribers.
type Hub struct {
	mu         sync.Mutex
	taskSubs   map[string]map[chan []byte]struct{} // taskID → subscribers
	globalSubs map[chan []byte]struct{}            // taskID="" → all events
}

func NewHub() *Hub {
	return &Hub{
		taskSubs:   make(map[string]map[chan []byte]struct{}),
		globalSubs: make(map[chan []byte]struct{}),
	}
}

// Subscribe returns a buffered channel that receives SSE event bytes and an
// unsubscribe function. The caller must call unsubscribe when done.
// If taskID is empty, the subscriber receives all events (global subscription).
// Otherwise, the subscriber only receives events for the specified task.
func (h *Hub) Subscribe(taskID string) (chan []byte, func()) {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	if taskID == "" {
		h.globalSubs[ch] = struct{}{}
	} else {
		if h.taskSubs[taskID] == nil {
			h.taskSubs[taskID] = make(map[chan []byte]struct{})
		}
		h.taskSubs[taskID][ch] = struct{}{}
	}
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		if taskID == "" {
			delete(h.globalSubs, ch)
		} else {
			delete(h.taskSubs[taskID], ch)
			if len(h.taskSubs[taskID]) == 0 {
				delete(h.taskSubs, taskID) // reclaim map entry
			}
		}
		h.mu.Unlock()
	}
	return ch, unsub
}

// Publish broadcasts a TaskEvent to task-scoped and global subscribers.
func (h *Hub) Publish(event models.TaskEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.broadcast(event.TaskID, data)
}

func (h *Hub) broadcast(taskID string, data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.taskSubs[taskID] { // range over nil map is safe in Go
		select {
		case ch <- data:
		default: // drop slow consumer
		}
	}
	for ch := range h.globalSubs {
		select {
		case ch <- data:
		default:
		}
	}
}
