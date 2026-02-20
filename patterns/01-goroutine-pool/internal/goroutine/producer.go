package goroutine

import (
	"context"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.TaskProducer = (*ChannelProducer)(nil)

// ChannelProducer is the manager-side view of the in-process channel transport.
// It writes tasks to workers and reads results/progress back.
type ChannelProducer struct {
	tasks    chan<- models.Task            // write-only: dispatch tasks to workers
	results  <-chan models.TaskStatusEvent // read-only:  receive results from workers
	progress <-chan models.ProgressEvent   // read-only:  receive progress from workers
}

// New creates a linked ChannelProducer and ChannelConsumer sharing the same
// buffered channels. tasks is buffered to queueSize; results and progress to 256.
func New(queueSize int) (*ChannelProducer, *ChannelConsumer) {
	tasks := make(chan models.Task, queueSize)
	results := make(chan models.TaskStatusEvent, 256)
	progress := make(chan models.ProgressEvent, 256)
	return &ChannelProducer{tasks: tasks, results: results, progress: progress},
		&ChannelConsumer{tasks: tasks, results: results, progress: progress}
}

// Start is a no-op; channels are ready at construction time.
func (p *ChannelProducer) Start(_ context.Context) error { return nil }

// Dispatch sends the task to the tasks channel.
// Returns ErrDispatchFull immediately if the channel is at capacity.
func (p *ChannelProducer) Dispatch(_ context.Context, task models.Task) error {
	select {
	case p.tasks <- task:
		return nil
	default:
		return dispatch.ErrDispatchFull
	}
}

// ReceiveResult blocks until a result event is available or ctx is cancelled.
func (p *ChannelProducer) ReceiveResult(ctx context.Context) (models.TaskStatusEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskStatusEvent{}, ctx.Err()
	case ev := <-p.results:
		return ev, nil
	}
}

// ReceiveProgress blocks until a progress event is available or ctx is cancelled.
func (p *ChannelProducer) ReceiveProgress(ctx context.Context) (models.ProgressEvent, error) {
	select {
	case <-ctx.Done():
		return models.ProgressEvent{}, ctx.Err()
	case ev := <-p.progress:
		return ev, nil
	}
}
