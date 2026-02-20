package pool

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
)

// poolResultSink routes task-level status to the SSE hub (for browser updates)
// and persists it to the store (for the /tasks endpoint).
type poolResultSink struct {
	hub   *sse.Hub
	store store.TaskStore
}

func (s *poolResultSink) Record(taskID string, status models.TaskStatus) error {
	s.hub.PublishTaskStatus(taskID, status)
	return s.store.SetStatus(taskID, status)
}

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
		rs := &poolResultSink{hub: m.hub, store: m.store}
		_ = rs.Record(task.ID, models.TaskRunning)
		status := m.exec.Run(context.Background(), task, m.hub)
		_ = rs.Record(task.ID, status)
	})
	if errors.Is(err, ErrQueueFull) {
		_ = m.store.SetStatus(task.ID, models.TaskFailed)
		return echo.NewHTTPError(http.StatusTooManyRequests, "task queue is full — retry later").
			SetInternal(err)
	}
	return err
}
