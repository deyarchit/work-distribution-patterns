package redisinternal

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"

	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/models"
)

// RedisSink implements dispatch.ProgressSink and dispatch.ResultSink by publishing
// progress events and task status to Redis Pub/Sub. All API replicas subscribe to
// these channels, so every SSE hub is updated regardless of which replica the browser
// is connected to. Workers use RedisSink instead of a NATS sink so that the API
// layer — not the transport layer — owns the cross-replica fan-out responsibility.
type RedisSink struct {
	rdb *redis.Client
}

// NewRedisSink creates a RedisSink using the given Redis client.
func NewRedisSink(rdb *redis.Client) *RedisSink {
	return &RedisSink{rdb: rdb}
}

func (s *RedisSink) Publish(event models.ProgressEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	if err := s.rdb.Publish(context.Background(), progressPrefix+event.TaskID, data).Err(); err != nil {
		log.Printf("sink: publish progress error: %v", err)
	}
}

func (s *RedisSink) Record(taskID string, status models.TaskStatus) error {
	payload := models.TaskStatusEvent{TaskID: taskID, Status: status}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.rdb.Publish(context.Background(), taskStatusPrefix+taskID, data).Err()
}

// Compile-time interface checks.
var _ dispatch.ProgressSink = (*RedisSink)(nil)
var _ dispatch.ResultSink = (*RedisSink)(nil)
