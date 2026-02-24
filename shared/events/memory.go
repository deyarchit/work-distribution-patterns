package events

import (
	"context"
	"sync"

	"work-distribution-patterns/shared/models"
)

type MemoryEventBus struct {
	mu     sync.RWMutex
	events []StoredEvent
	lastID int64
	cond   *sync.Cond
	live   map[chan models.TaskEvent]struct{}
}

func NewMemoryEventBus() *MemoryEventBus {
	m := &MemoryEventBus{
		live: make(map[chan models.TaskEvent]struct{}),
	}
	m.cond = sync.NewCond(&m.mu)
	return m
}

func (m *MemoryEventBus) Publish(event models.TaskEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastID++
	stored := StoredEvent{
		ID:    m.lastID,
		Event: event,
	}
	m.events = append(m.events, stored)

	// Keep only last 1000 events to prevent memory leak
	if len(m.events) > 1000 {
		m.events = m.events[len(m.events)-1000:]
	}

	// Notify pollers
	m.cond.Broadcast()

	// Notify live subscribers
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

func (m *MemoryEventBus) Poll(ctx context.Context, afterID int64) ([]StoredEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Wait for new events if none are available
	for afterID >= m.lastID {
		// Check context before waiting
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Use a goroutine to signal the condition if context is cancelled
		// since sync.Cond.Wait() is not interruptible.
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				m.mu.Lock()
				m.cond.Broadcast()
				m.mu.Unlock()
			case <-done:
			}
		}()

		m.cond.Wait()
		close(done)

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	var results []StoredEvent
	for _, e := range m.events {
		if e.ID > afterID {
			results = append(results, e)
		}
	}

	return results, nil
}
