package bus

import (
	"context"

	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface checks.
var _ dispatch.WorkerBus = (*ChannelBus)(nil)
var _ dispatch.WorkerSource = (*ChannelBus)(nil)

// ChannelBus implements both WorkerBus and WorkerSource using buffered channels.
// Both sides live in the same process, so a single struct serves both roles.
type ChannelBus struct {
	tasks    chan models.Task
	results  chan models.TaskStatusEvent
	progress chan models.ProgressEvent
}

// New creates a ChannelBus with tasks buffered to queueSize and
// results/progress buffered to 256 each.
func New(queueSize int) *ChannelBus {
	return &ChannelBus{
		tasks:    make(chan models.Task, queueSize),
		results:  make(chan models.TaskStatusEvent, 256),
		progress: make(chan models.ProgressEvent, 256),
	}
}

// WorkerBus side

// Start is a no-op; channels are ready at construction time.
func (b *ChannelBus) Start(_ context.Context) error { return nil }

// Dispatch sends the task to the tasks channel.
// Returns ErrDispatchFull immediately if the channel is at capacity.
func (b *ChannelBus) Dispatch(_ context.Context, task models.Task) error {
	select {
	case b.tasks <- task:
		return nil
	default:
		return dispatch.ErrDispatchFull
	}
}

// ReceiveResult blocks until a result event is available or ctx is cancelled.
func (b *ChannelBus) ReceiveResult(ctx context.Context) (models.TaskStatusEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskStatusEvent{}, ctx.Err()
	case ev := <-b.results:
		return ev, nil
	}
}

// ReceiveProgress blocks until a progress event is available or ctx is cancelled.
func (b *ChannelBus) ReceiveProgress(ctx context.Context) (models.ProgressEvent, error) {
	select {
	case <-ctx.Done():
		return models.ProgressEvent{}, ctx.Err()
	case ev := <-b.progress:
		return ev, nil
	}
}

// WorkerSource side

// Connect is a no-op; channels are ready at construction time.
func (b *ChannelBus) Connect(_ context.Context) error { return nil }

// Receive blocks until a task is available or ctx is cancelled.
func (b *ChannelBus) Receive(ctx context.Context) (models.Task, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, ctx.Err()
	case task := <-b.tasks:
		return task, nil
	}
}

// ReportResult sends a task status event to the results channel.
// Terminal statuses (completed/failed) use a blocking send to ensure delivery.
// Non-terminal statuses are dropped if the channel is full.
func (b *ChannelBus) ReportResult(_ context.Context, taskID string, status models.TaskStatus) error {
	ev := models.TaskStatusEvent{TaskID: taskID, Status: status}
	if status == models.TaskCompleted || status == models.TaskFailed {
		b.results <- ev
	} else {
		select {
		case b.results <- ev:
		default:
		}
	}
	return nil
}

// ReportProgress sends a progress event to the progress channel.
// Events are dropped if the channel is full (best-effort).
func (b *ChannelBus) ReportProgress(_ context.Context, event models.ProgressEvent) error {
	select {
	case b.progress <- event:
	default:
	}
	return nil
}
