package manager

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/store"
)

// Compile-time check that Manager implements dispatch.TaskManager.
var _ dispatch.TaskManager = (*Manager)(nil)

// Manager is the unified task lifecycle owner.
// It persists tasks, dispatches them via TaskDispatcher, routes events from the dispatcher
// to the store and event bus, and optionally re-dispatches stalled tasks.
type Manager struct {
	store      store.TaskStore
	dispatcher dispatch.TaskDispatcher
	events     events.TaskEventBus
	deadline   time.Duration
}

// New creates a Manager.
// deadline controls re-dispatch: 0 disables the deadline loop entirely.
func New(s store.TaskStore, d dispatch.TaskDispatcher, evs events.TaskEventBus, deadline time.Duration) *Manager {
	return &Manager{
		store:      s,
		dispatcher: d,
		events:     evs,
		deadline:   deadline,
	}
}

// Events returns the event bus.
func (m *Manager) Events() events.TaskEventBus {
	return m.events
}

// Submit persists the task then dispatches it via the dispatcher.
// Maps dispatcher sentinel errors to appropriate HTTP status codes.
// On any dispatch error, marks the task failed in the store.
func (m *Manager) Submit(ctx context.Context, task models.Task) error {
	if err := m.store.Create(task); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := m.dispatcher.Dispatch(ctx, task); err != nil {
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

	_ = m.store.SetDispatchedAt(task.ID, time.Now())
	return nil
}

// Get returns the task with the given id from the store.
func (m *Manager) Get(_ context.Context, id string) (models.Task, bool) {
	return m.store.Get(id)
}

// List returns all tasks from the store.
func (m *Manager) List(_ context.Context) []models.Task {
	tasks := m.store.List()
	if tasks == nil {
		return []models.Task{}
	}
	return tasks
}

// Subscribe returns a channel that receives all TaskEvents.
func (m *Manager) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error) {
	return m.events.Subscribe(ctx)
}

// Start initialises the dispatcher then launches the event and deadline goroutines.
// It is non-blocking; call it once before serving requests.
func (m *Manager) Start(ctx context.Context) {
	if err := m.dispatcher.Start(ctx); err != nil {
		return
	}
	go m.runEventLoop(ctx)
	go m.runDeadlineLoop(ctx)
}

// runEventLoop consumes all TaskEvents from the dispatcher in a single goroutine,
// guaranteeing that progress and status events are processed in the order
// they were emitted. Terminal statuses are persisted; all events are forwarded
// to the SSE hub.
func (m *Manager) runEventLoop(ctx context.Context) {
	for {
		event, err := m.dispatcher.ReceiveEvent(ctx)
		if err != nil {
			return
		}
		if event.Type == models.EventTaskStatus {
			status := models.TaskStatus(event.Status)
			if status == models.TaskCompleted || status == models.TaskFailed {
				_ = m.store.SetStatus(event.TaskID, status)
			}
		}
		m.events.Publish(event)
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
			for _, task := range m.store.List() {
				if task.Status == models.TaskCompleted || task.Status == models.TaskFailed {
					continue
				}
				if task.DispatchedAt != nil && now.Sub(*task.DispatchedAt) > m.deadline {
					_ = m.dispatcher.Dispatch(ctx, task)
					_ = m.store.SetDispatchedAt(task.ID, now)
				}
			}
		}
	}
}
