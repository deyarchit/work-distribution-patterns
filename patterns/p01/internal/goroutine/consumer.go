package goroutine

import (
	"context"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.TaskConsumer = (*ChannelConsumer)(nil)

// ChannelConsumer is the worker-side view of the in-process channel transport.
// It reads tasks from the manager and writes all events back over a single channel.
type ChannelConsumer struct {
	tasks  <-chan models.Task      // read-only:  receive tasks from manager
	events chan<- models.TaskEvent // write-only: emit events to manager
}

// Connect is a no-op; channels are ready at construction time.
func (c *ChannelConsumer) Connect(_ context.Context) error { return nil }

// Receive blocks until a task is available or ctx is cancelled.
func (c *ChannelConsumer) Receive(ctx context.Context) (models.Task, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, ctx.Err()
	case task := <-c.tasks:
		return task, nil
	}
}

// Emit sends a TaskEvent to the manager. Terminal task_status events use a
// blocking send to guarantee delivery. All other events are best-effort and
// may be dropped if the channel is full.
func (c *ChannelConsumer) Emit(_ context.Context, event models.TaskEvent) error {
	isTerminal := event.Type == models.EventTaskStatus &&
		(event.Status == string(models.TaskCompleted) || event.Status == string(models.TaskFailed))
	if isTerminal {
		c.events <- event // blocking: must not lose terminal status
	} else {
		select {
		case c.events <- event:
		default: // drop progress/running events if channel is full
		}
	}
	return nil
}
