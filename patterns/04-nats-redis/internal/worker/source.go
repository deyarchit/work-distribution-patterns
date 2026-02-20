package worker

import (
	"context"
	"encoding/json"
	"log"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	natsinternal "work-distribution-patterns/patterns/04-nats-redis/internal/nats"
	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.WorkerSource = (*NATSRedisSource)(nil)

const (
	progressPrefix   = "progress:"
	taskStatusPrefix = "task_status:"
)

// NATSRedisSource implements WorkerSource using NATS JetStream to receive tasks
// and Redis Pub/Sub to report results and progress.
// The worker loop is synchronous — one task at a time — preserving NATS
// at-least-once delivery semantics.
type NATSRedisSource struct {
	js    nats.JetStreamContext
	rdb   *redis.Client
	tasks chan models.Task
}

// New creates a NATSRedisSource. Call Connect to start the subscription.
func New(js nats.JetStreamContext, rdb *redis.Client) *NATSRedisSource {
	return &NATSRedisSource{
		js:    js,
		rdb:   rdb,
		tasks: make(chan models.Task, 1),
	}
}

// Connect starts the JetStream queue subscription in a background goroutine (non-blocking).
func (s *NATSRedisSource) Connect(ctx context.Context) error {
	go s.subscribe(ctx)
	return nil
}

func (s *NATSRedisSource) subscribe(ctx context.Context) {
	sub, err := s.js.QueueSubscribe(
		"tasks.new",
		natsinternal.ConsumerDur,
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
		nats.Durable(natsinternal.ConsumerDur),
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
func (s *NATSRedisSource) Receive(ctx context.Context) (models.Task, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, ctx.Err()
	case task := <-s.tasks:
		return task, nil
	}
}

// ReportResult publishes a task status event to "task_status:<taskID>" via Redis Pub/Sub.
// All API replicas subscribe to this channel.
func (s *NATSRedisSource) ReportResult(ctx context.Context, taskID string, status models.TaskStatus) error {
	payload := models.TaskStatusEvent{TaskID: taskID, Status: status}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.rdb.Publish(ctx, taskStatusPrefix+taskID, data).Err()
}

// ReportProgress publishes a progress event to "progress:<taskID>" via Redis Pub/Sub.
func (s *NATSRedisSource) ReportProgress(ctx context.Context, event models.ProgressEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.rdb.Publish(ctx, progressPrefix+event.TaskID, data).Err()
}
