package pool

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
)

// PoolTaskManager implements dispatch.TaskManager using the bounded goroutine pool.
// It owns the full task lifecycle: persisting to the store, enqueuing execution,
// routing progress to the SSE hub, and persisting the terminal status.
type PoolTaskManager struct {
	pool  *Pool
	hub   *sse.Hub
	exec  *executor.Executor
	store store.TaskStore
}

// NewPoolTaskManager creates a PoolTaskManager backed by the given pool, hub, executor, and store.
func NewPoolTaskManager(p *Pool, hub *sse.Hub, exec *executor.Executor, store store.TaskStore) *PoolTaskManager {
	return &PoolTaskManager{pool: p, hub: hub, exec: exec, store: store}
}

// Submit persists the task, then enqueues it for execution.
// Returns HTTP 429 if the queue is full; on queue full the task is marked failed in the store.
func (m *PoolTaskManager) Submit(ctx context.Context, task models.Task) error {
	if err := m.store.Create(task); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	err := m.pool.Enqueue(func() {
		// Use background context so the task completes even if the HTTP request is cancelled.
		// The hub is passed directly as the ProgressSink — it handles SSE broadcast.
		// The returned status is used to persist the terminal state; no sink wrapping needed.
		status := m.exec.Run(context.Background(), task, m.hub)
		_ = m.store.SetStatus(task.ID, status)
	})
	if err == ErrQueueFull {
		_ = m.store.SetStatus(task.ID, models.TaskFailed)
		return echo.NewHTTPError(http.StatusTooManyRequests, "task queue is full — retry later").
			SetInternal(err)
	}
	return err
}
