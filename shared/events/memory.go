package events

import (
	"context"
	"sync"

	"work-distribution-patterns/shared/models"
)

type MemoryEventBus struct {
	mu   sync.RWMutex
	live map[chan models.TaskEvent]struct{}
}

func NewMemoryEventBus() *MemoryEventBus {
	return &MemoryEventBus{
		live: make(map[chan models.TaskEvent]struct{}),
	}
}

func (m *MemoryEventBus) Publish(event models.TaskEvent) {
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

func (m *MemoryEventBus) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error) {
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
