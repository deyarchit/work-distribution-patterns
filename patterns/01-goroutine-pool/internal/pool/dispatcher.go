package pool

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
)

// PoolDispatcher implements dispatch.Dispatcher using the bounded goroutine pool.
type PoolDispatcher struct {
	pool  *Pool
	hub   *sse.Hub
	exec  *executor.Executor
	store executor.StatusWriter
}

// NewPoolDispatcher creates a PoolDispatcher backed by the given pool, hub, executor, and store.
func NewPoolDispatcher(p *Pool, hub *sse.Hub, exec *executor.Executor, store executor.StatusWriter) *PoolDispatcher {
	return &PoolDispatcher{pool: p, hub: hub, exec: exec, store: store}
}

// Submit enqueues the task for execution. Returns HTTP 429 if the queue is full.
func (d *PoolDispatcher) Submit(ctx context.Context, task models.Task) error {
	err := d.pool.Enqueue(func() {
		// Use a background context so task completes even if HTTP request is cancelled
		sink := &executor.SinkWithStore{Inner: d.hub, Store: d.store}
		d.exec.Run(context.Background(), task, sink)
	})
	if err == ErrQueueFull {
		return echo.NewHTTPError(http.StatusTooManyRequests, "task queue is full — retry later").
			SetInternal(err)
	}
	return err
}
