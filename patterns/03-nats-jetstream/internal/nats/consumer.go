package natsinternal

import (
	"context"
	"encoding/json"
	"log"

	"github.com/nats-io/nats.go"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.TaskConsumer = (*NATSConsumer)(nil)

// NATSConsumer implements TaskConsumer over NATS.
// It receives tasks from JetStream (queue-subscribe with manual ACK for
// at-least-once delivery) and reports results and progress via NATS Core.
// The worker loop is synchronous — one task at a time — preserving NATS
// at-least-once delivery semantics.
type NATSConsumer struct {
	nc    *nats.Conn
	js    nats.JetStreamContext
	tasks chan models.Task
}

// NewNATSConsumer creates a NATSConsumer. Call Connect to start the subscription.
func NewNATSConsumer(nc *nats.Conn, js nats.JetStreamContext) *NATSConsumer {
	return &NATSConsumer{
		nc:    nc,
		js:    js,
		tasks: make(chan models.Task, 1),
	}
}

// Connect starts the JetStream queue subscription in a background goroutine (non-blocking).
func (s *NATSConsumer) Connect(ctx context.Context) error {
	go s.subscribe(ctx)
	return nil
}

func (s *NATSConsumer) subscribe(ctx context.Context) {
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
			// Block until the main loop picks up the task, then ACK.
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

// Receive blocks until a task is available or ctx is cancelled.
func (s *NATSConsumer) Receive(ctx context.Context) (models.Task, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, ctx.Err()
	case task := <-s.tasks:
		return task, nil
	}
}

// ReportResult publishes a task status event to "task_status.<taskID>" via NATS Core.
// All API replicas subscribe to this subject.
func (s *NATSConsumer) ReportResult(_ context.Context, taskID string, status models.TaskStatus) error {
	payload := models.TaskStatusEvent{TaskID: taskID, Status: status}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.nc.Publish("task_status."+taskID, data)
}

// ReportProgress publishes a progress event to "progress.<taskID>" via NATS Core.
func (s *NATSConsumer) ReportProgress(_ context.Context, event models.ProgressEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.nc.Publish("progress."+event.TaskID, data)
}
