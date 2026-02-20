package redisinternal

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
)

// RedisTaskManager implements dispatch.TaskManager using NATS JetStream for
// task submission and Redis Pub/Sub for SSE fan-out.
//
// Separation of concerns:
//   - NATS JetStream: reliable at-least-once delivery from API to workers
//   - Redis Pub/Sub:  lightweight broadcast from workers to all API replicas
//
// Workers publish progress directly to Redis, so every API replica receives
// every event via its own PSubscribe connection — no sticky sessions needed.
type RedisTaskManager struct {
	js    nats.JetStreamContext
	store store.TaskStore
}

// NewRedisTaskManager creates a RedisTaskManager and wires the Redis Pub/Sub
// subscriptions that route worker progress events to the local SSE hub and
// task store. Called by every API replica at startup.
func NewRedisTaskManager(ctx context.Context, js nats.JetStreamContext, rdb *redis.Client, taskStore store.TaskStore, hub *sse.Hub) *RedisTaskManager {
	m := &RedisTaskManager{js: js, store: taskStore}

	// PSubscribe to all worker-published progress channels.
	// Workers publish directly to Redis, so this subscription fires on every
	// API replica — giving each its own SSE delivery path to the browser.
	pubsub := rdb.PSubscribe(ctx, progressPrefix+"*", taskStatusPrefix+"*")

	go func() {
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Printf("manager: pubsub close: %v", err)
			}
		}()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				switch {
				case len(msg.Channel) > len(progressPrefix) && msg.Channel[:len(progressPrefix)] == progressPrefix:
					var ev models.ProgressEvent
					if err := json.Unmarshal([]byte(msg.Payload), &ev); err == nil {
						hub.Publish(ev)
					}
				case len(msg.Channel) > len(taskStatusPrefix) && msg.Channel[:len(taskStatusPrefix)] == taskStatusPrefix:
					var payload models.TaskStatusEvent
					if err := json.Unmarshal([]byte(msg.Payload), &payload); err == nil {
						hub.PublishTaskStatus(payload.TaskID, payload.Status)
						if err := taskStore.SetStatus(payload.TaskID, payload.Status); err != nil {
							log.Printf("manager: SetStatus error: %v", err)
						}
					}
				}
			}
		}
	}()

	return m
}

// Submit persists the task to Redis then publishes it to NATS JetStream for
// at-least-once delivery to a worker. On publish failure the task is marked
// failed in the store.
func (m *RedisTaskManager) Submit(_ context.Context, task models.Task) error {
	if err := m.store.Create(task); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	payload, err := json.Marshal(task)
	if err != nil {
		_ = m.store.SetStatus(task.ID, models.TaskFailed)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if _, err := m.js.Publish("tasks.new", payload); err != nil {
		_ = m.store.SetStatus(task.ID, models.TaskFailed)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue task: "+err.Error())
	}

	return nil
}
