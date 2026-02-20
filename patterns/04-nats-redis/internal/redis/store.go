package redisinternal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"work-distribution-patterns/shared/models"
)

const (
	taskTTL          = 24 * time.Hour
	taskPrefix       = "task:"
	tasksSet         = "tasks:all"
	progressPrefix   = "progress:"
	taskStatusPrefix = "task_status:"
)

// RedisTaskStore implements store.TaskStore using Redis Strings and a Set.
// Tasks are stored as JSON under "task:<id>" with a 24 h TTL.
// The set "tasks:all" holds all known task IDs for List queries.
// Using Redis for the store gives all API replicas a consistent view of task
// state without requiring a separate database.
type RedisTaskStore struct {
	rdb *redis.Client
}

// NewRedisTaskStore creates a RedisTaskStore backed by the given Redis client.
func NewRedisTaskStore(rdb *redis.Client) *RedisTaskStore {
	return &RedisTaskStore{rdb: rdb}
}

func (s *RedisTaskStore) Create(task models.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := s.rdb.Set(ctx, taskPrefix+task.ID, data, taskTTL).Err(); err != nil {
		return err
	}
	return s.rdb.SAdd(ctx, tasksSet, task.ID).Err()
}

func (s *RedisTaskStore) Get(id string) (models.Task, bool) {
	data, err := s.rdb.Get(context.Background(), taskPrefix+id).Bytes()
	if err != nil {
		return models.Task{}, false
	}
	var task models.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return models.Task{}, false
	}
	return task, true
}

func (s *RedisTaskStore) List() []models.Task {
	ids, err := s.rdb.SMembers(context.Background(), tasksSet).Result()
	if err != nil {
		return []models.Task{}
	}
	tasks := make([]models.Task, 0, len(ids))
	for _, id := range ids {
		if t, ok := s.Get(id); ok {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

func (s *RedisTaskStore) SetStatus(id string, status models.TaskStatus) error {
	task, ok := s.Get(id)
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	task.Status = status
	if status == models.TaskCompleted || status == models.TaskFailed {
		now := time.Now()
		task.CompletedAt = &now
	}
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return s.rdb.Set(context.Background(), taskPrefix+id, data, taskTTL).Err()
}
