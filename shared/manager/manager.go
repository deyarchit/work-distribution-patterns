package manager

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
)

// Compile-time check that Manager implements dispatch.TaskManager.
var _ dispatch.TaskManager = (*Manager)(nil)

// Manager is the unified task lifecycle owner.
// It persists tasks, dispatches them via WorkerBus, routes events from the bus
// to the store and SSE hub, and optionally re-dispatches stalled tasks.
type Manager struct {
	store         store.TaskStore
	bus           dispatch.WorkerBus
	hub           *sse.Hub
	deadline      time.Duration
	mu            sync.Mutex
	dispatchTimes map[string]time.Time // taskID → last dispatch time; in-memory only
}

// New creates a Manager.
// deadline controls re-dispatch: 0 disables the deadline loop entirely.
func New(s store.TaskStore, bus dispatch.WorkerBus, hub *sse.Hub, deadline time.Duration) *Manager {
	return &Manager{
		store:         s,
		bus:           bus,
		hub:           hub,
		deadline:      deadline,
		dispatchTimes: make(map[string]time.Time),
	}
}

// Submit persists the task then dispatches it via the bus.
// Maps bus sentinel errors to appropriate HTTP status codes.
// On any dispatch error, marks the task failed in the store.
func (m *Manager) Submit(ctx context.Context, task models.Task) error {
	if err := m.store.Create(task); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := m.bus.Dispatch(ctx, task); err != nil {
		_ = m.store.SetStatus(task.ID, models.TaskFailed)
		switch {
		case errors.Is(err, dispatch.ErrDispatchFull):
			return echo.NewHTTPError(http.StatusTooManyRequests, "task queue is full — retry later").
				SetInternal(err)
		case errors.Is(err, dispatch.ErrNoWorkers):
			return echo.NewHTTPError(http.StatusServiceUnavailable, "no workers available — retry later").
				SetInternal(err)
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	m.mu.Lock()
	m.dispatchTimes[task.ID] = time.Now()
	m.mu.Unlock()
	return nil
}

// Start initialises the bus then launches the result, progress, and deadline goroutines.
// It is non-blocking; call it once before serving requests.
func (m *Manager) Start(ctx context.Context) {
	if err := m.bus.Start(ctx); err != nil {
		return
	}
	go m.runResultLoop(ctx)
	go m.runProgressLoop(ctx)
	go m.runDeadlineLoop(ctx)
}

func (m *Manager) runResultLoop(ctx context.Context) {
	for {
		event, err := m.bus.ReceiveResult(ctx)
		if err != nil {
			return
		}
		_ = m.store.SetStatus(event.TaskID, event.Status)
		m.hub.PublishTaskStatus(event.TaskID, event.Status)
		if event.Status == models.TaskCompleted || event.Status == models.TaskFailed {
			m.mu.Lock()
			delete(m.dispatchTimes, event.TaskID)
			m.mu.Unlock()
		}
	}
}

func (m *Manager) runProgressLoop(ctx context.Context) {
	for {
		event, err := m.bus.ReceiveProgress(ctx)
		if err != nil {
			return
		}
		m.hub.Publish(event)
	}
}

// runDeadlineLoop ticks at deadline/2 and re-dispatches tasks whose last
// dispatch time is older than the deadline. Exits immediately if deadline == 0.
func (m *Manager) runDeadlineLoop(ctx context.Context) {
	if m.deadline == 0 {
		return
	}
	ticker := time.NewTicker(m.deadline / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			tasks := m.store.List()
			m.mu.Lock()
			for _, task := range tasks {
				if task.Status == models.TaskCompleted || task.Status == models.TaskFailed {
					continue
				}
				t, ok := m.dispatchTimes[task.ID]
				if ok && now.Sub(t) > m.deadline {
					_ = m.bus.Dispatch(ctx, task)
					m.dispatchTimes[task.ID] = now
				}
			}
			m.mu.Unlock()
		}
	}
}
