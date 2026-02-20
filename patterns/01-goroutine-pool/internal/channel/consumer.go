package channel

import (
	"context"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.TaskConsumer = (*ChannelConsumer)(nil)

// ChannelConsumer is the worker-side view of the in-process channel transport.
// It reads tasks from the manager and writes results/progress back.
type ChannelConsumer struct {
	tasks    <-chan models.Task            // read-only:  receive tasks from manager
	results  chan<- models.TaskStatusEvent // write-only: report results to manager
	progress chan<- models.ProgressEvent   // write-only: report progress to manager
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

// ReportResult sends a task status event to the results channel.
// Terminal statuses (completed/failed) use a blocking send to ensure delivery.
// Non-terminal statuses are dropped if the channel is full.
func (c *ChannelConsumer) ReportResult(_ context.Context, taskID string, status models.TaskStatus) error {
	ev := models.TaskStatusEvent{TaskID: taskID, Status: status}
	if status == models.TaskCompleted || status == models.TaskFailed {
		c.results <- ev
	} else {
		select {
		case c.results <- ev:
		default:
		}
	}
	return nil
}

// ReportProgress sends a progress event to the progress channel.
// Events are dropped if the channel is full (best-effort).
func (c *ChannelConsumer) ReportProgress(_ context.Context, event models.ProgressEvent) error {
	select {
	case c.progress <- event:
	default:
	}
	return nil
}
