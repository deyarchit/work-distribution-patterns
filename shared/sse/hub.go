package sse

import (
	"encoding/json"
	"sync"

	"work-distribution-patterns/shared/models"
)

// Hub manages SSE subscribers and broadcasts events to user-scoped subscribers.
// Each subscriber is keyed by userID; events are routed by event.UserID.
type Hub struct {
	mu       sync.Mutex
	userSubs map[string]map[chan []byte]struct{} // userID → subscribers
}

func NewHub() *Hub {
	return &Hub{
		userSubs: make(map[string]map[chan []byte]struct{}),
	}
}

// Subscribe returns a buffered channel that receives SSE event bytes and an
// unsubscribe function. Events are delivered only for the given userID.
// The caller must call unsubscribe when done.
func (h *Hub) Subscribe(userID string) (chan []byte, func()) {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	if h.userSubs[userID] == nil {
		h.userSubs[userID] = make(map[chan []byte]struct{})
	}
	h.userSubs[userID][ch] = struct{}{}
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		delete(h.userSubs[userID], ch)
		if len(h.userSubs[userID]) == 0 {
			delete(h.userSubs, userID)
		}
		h.mu.Unlock()
	}
	return ch, unsub
}

// Publish routes a TaskEvent to all subscribers for event.UserID.
func (h *Hub) Publish(event models.TaskEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.userSubs[event.UserID] {
		select {
		case ch <- data:
		default: // drop slow consumer
		}
	}
}
