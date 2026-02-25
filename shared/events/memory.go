package events

import (
	"context"
	"sync"

	"work-distribution-patterns/shared/models"
)

// MemoryBridge implements TaskEventBridge using in-memory channels.
type MemoryBridge struct {
	mu   sync.RWMutex
	live map[chan models.TaskEvent]struct{}
}

func NewMemoryBridge() *MemoryBridge {
	return &MemoryBridge{
		live: make(map[chan models.TaskEvent]struct{}),
	}
}

func (m *MemoryBridge) Publish(event models.TaskEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Notify all live subscribers
	for ch := range m.live {
		select {
		case ch <- event:
		default:
			// Drop slow consumers
		}
	}
}

func (m *MemoryBridge) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error) {
	ch := make(chan models.TaskEvent, 64)
	m.mu.Lock()
	m.live[ch] = struct{}{}
	m.mu.Unlock()

	go func() {
		<-ctx.Done()
		m.mu.Lock()
		delete(m.live, ch)
		m.mu.Unlock()
		close(ch)
	}()

	return ch, nil
}
