package natsinternal

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"work-distribution-patterns/shared/models"
)

// JetStreamStore implements store.TaskStore against a NATS KV bucket.
type JetStreamStore struct {
	kv nats.KeyValue
}

// NewJetStreamStore creates a JetStreamStore backed by the given KV bucket.
func NewJetStreamStore(kv nats.KeyValue) *JetStreamStore {
	return &JetStreamStore{kv: kv}
}

func (s *JetStreamStore) Create(task models.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = s.kv.Put(task.ID, data)
	return err
}

func (s *JetStreamStore) Get(id string) (models.Task, bool) {
	entry, err := s.kv.Get(id)
	if err != nil {
		return models.Task{}, false
	}
	var task models.Task
	if err := json.Unmarshal(entry.Value(), &task); err != nil {
		return models.Task{}, false
	}
	return task, true
}

func (s *JetStreamStore) List() []models.Task {
	keys, err := s.kv.Keys()
	if err != nil {
		return []models.Task{}
	}
	tasks := make([]models.Task, 0, len(keys))
	for _, k := range keys {
		if t, ok := s.Get(k); ok {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

func (s *JetStreamStore) SetStatus(id string, status models.TaskStatus) error {
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
	_, err = s.kv.Put(id, data)
	return err
}

func (s *JetStreamStore) SetDispatchedAt(id string, t time.Time) error {
	task, ok := s.Get(id)
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	task.DispatchedAt = &t
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = s.kv.Put(id, data)
	return err
}
