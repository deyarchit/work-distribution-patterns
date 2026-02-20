package natsinternal

import (
	"context"
	"encoding/json"
	"log"

	"github.com/nats-io/nats.go"

	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/models"
)

// NATSTaskSource implements dispatch.TaskSource over NATS JetStream.
// Call Connect in a goroutine to start the queue subscription; call Receive
// to pull tasks one at a time.
type NATSTaskSource struct {
	js           nats.JetStreamContext
	tasks        chan models.Task
	progressSink dispatch.ProgressSink
	resultSink   dispatch.ResultSink
}

// NewNATSTaskSource creates a NATSTaskSource using the given JetStream context and sinks.
// The same RedisSink satisfies both ProgressSink and ResultSink and is injected here
// to avoid a cross-package import on the concrete type.
func NewNATSTaskSource(js nats.JetStreamContext, progressSink dispatch.ProgressSink, resultSink dispatch.ResultSink) *NATSTaskSource {
	return &NATSTaskSource{
		js:           js,
		tasks:        make(chan models.Task, 1),
		progressSink: progressSink,
		resultSink:   resultSink,
	}
}

// Connect runs the queue subscription until ctx is cancelled.
// It should be called in a goroutine by the caller.
func (s *NATSTaskSource) Connect(ctx context.Context) {
	sub, err := s.js.QueueSubscribe(
		"tasks.new",
		ConsumerDur,
		func(msg *nats.Msg) {
			var task models.Task
			if err := json.Unmarshal(msg.Data, &task); err != nil {
				log.Printf("unmarshal task: %v", err)
				if err := msg.Nak(); err != nil {
					log.Printf("nack error: %v", err)
				}
				return
			}
			// Block until main loop picks up the task, then ACK.
			// If the process crashes before Receive returns, NATS redelivers.
			select {
			case s.tasks <- task:
				if err := msg.Ack(); err != nil {
					log.Printf("ack error: %v", err)
				}
			case <-ctx.Done():
			}
		},
		nats.Durable(ConsumerDur),
		nats.ManualAck(),
	)
	if err != nil {
		log.Printf("subscribe error: %v", err)
		return
	}
	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("unsubscribe error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down worker")
}

// Receive implements dispatch.TaskSource.
// Blocks until a task is available or ctx is cancelled.
// Returns the fixed process-level sinks as ProgressSink and ResultSink.
func (s *NATSTaskSource) Receive(ctx context.Context) (models.Task, dispatch.ProgressSink, dispatch.ResultSink, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, nil, nil, ctx.Err()
	case task := <-s.tasks:
		return task, s.progressSink, s.resultSink, nil
	}
}

// Compile-time interface check.
var _ dispatch.TaskSource = (*NATSTaskSource)(nil)
