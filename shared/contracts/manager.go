package contracts

import (
	"context"

	"work-distribution-patterns/shared/models"
)

// TaskManager is the single abstraction point between the HTTP layer and
// whatever mechanism is used to execute tasks. All patterns implement this.
//
// TaskManager owns the full task lifecycle:
//   - Persisting the task to the store before dispatching
//   - Dispatching work to the execution substrate (pool, WebSocket, NATS, etc.)
//   - Receiving progress reports from workers and routing them to the SSE hub
//   - Persisting the final terminal status (completed/failed) to the store
//
// Workers report progress via the ProgressSink they are given — they have no
// knowledge of stores, SSE, or browsers.
type TaskManager interface {
	Submit(ctx context.Context, task models.Task) error
	Get(ctx context.Context, id string) (models.Task, bool)
	List(ctx context.Context) []models.Task
	Subscribe(ctx context.Context) (<-chan models.TaskEvent, error)
}
