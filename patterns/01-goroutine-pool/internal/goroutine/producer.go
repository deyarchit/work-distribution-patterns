package goroutine

import (
	"context"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.TaskProducer = (*ChannelProducer)(nil)

// ChannelProducer is the manager-side view of the in-process channel transport.
// It writes tasks to workers and reads events back over a single ordered channel.
type ChannelProducer struct {
	tasks  chan<- models.Task      // write-only: dispatch tasks to workers
	events <-chan models.TaskEvent // read-only:  receive events from workers
}

// New creates a linked ChannelProducer and ChannelConsumer sharing the same
// buffered channels. tasks is buffered to queueSize; events to 256.
func New(queueSize int) (*ChannelProducer, *ChannelConsumer) {
	tasks := make(chan models.Task, queueSize)
	events := make(chan models.TaskEvent, 256)
	return &ChannelProducer{tasks: tasks, events: events},
		&ChannelConsumer{tasks: tasks, events: events}
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

// ReceiveEvent blocks until an event is available or ctx is cancelled.
func (p *ChannelProducer) ReceiveEvent(ctx context.Context) (models.TaskEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskEvent{}, ctx.Err()
	case ev := <-p.events:
		return ev, nil
	}
}
