package natsinternal

import (
	"context"
	"encoding/json"
	"log"

	"github.com/nats-io/nats.go"

	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
)

// NATSTaskSource implements dispatch.TaskSource over NATS JetStream.
// Call Connect in a goroutine to start the queue subscription; call Receive
// to pull tasks one at a time.
type NATSTaskSource struct {
	js    nats.JetStreamContext
	tasks chan models.Task
}

// NewNATSTaskSource creates a NATSTaskSource using the given JetStream context.
func NewNATSTaskSource(js nats.JetStreamContext) *NATSTaskSource {
	return &NATSTaskSource{
		js:    js,
		tasks: make(chan models.Task, 1),
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
func (s *NATSTaskSource) Receive(ctx context.Context) (models.Task, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, ctx.Err()
	case task := <-s.tasks:
		return task, nil
	}
}

// NATSSink publishes progress events back to the API via NATS Core subjects.
// All API replicas subscribe to these subjects, so all SSE hubs are updated.
// It is independent of any specific task source and can be constructed once.
type NATSSink struct {
	nc *nats.Conn
}

// NewNATSSink creates a NATSSink using the given NATS connection.
func NewNATSSink(nc *nats.Conn) *NATSSink {
	return &NATSSink{nc: nc}
}

func (s *NATSSink) Publish(event models.ProgressEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	if err := s.nc.Publish("progress."+event.TaskID, data); err != nil {
		log.Printf("publish progress error: %v", err)
	}
}

func (s *NATSSink) PublishTaskStatus(taskID string, status models.TaskStatus) {
	payload := struct {
		TaskID string            `json:"taskID"`
		Status models.TaskStatus `json:"status"`
	}{TaskID: taskID, Status: status}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := s.nc.Publish("task_status."+taskID, data); err != nil {
		log.Printf("publish task status error: %v", err)
	}
}

// Compile-time interface checks.
var _ executor.ProgressSink = (*NATSSink)(nil)
var _ dispatch.TaskSource = (*NATSTaskSource)(nil)
