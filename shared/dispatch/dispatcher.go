package dispatch

import (
	"context"

	"work-distribution-patterns/shared/models"
)

// Dispatcher is the single abstraction point between the HTTP layer and
// whatever mechanism is used to execute tasks. All patterns implement this.
type Dispatcher interface {
	// Submit enqueues a task. The implementation is responsible for
	// executing stages and routing all ProgressEvents into the SSE hub.
	// Stage duration is part of task.StageDurationSecs — no extra params needed.
	Submit(ctx context.Context, task models.Task) error
}
