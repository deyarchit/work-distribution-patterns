package store

import (
	"fmt"
	"sync"
	"time"

	"work-distribution-patterns/shared/models"
)

// MemoryStore is a thread-safe in-memory TaskStore used by Pattern 1 and 2.
type MemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]*models.Task
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tasks: make(map[string]*models.Task),
	}
}

func (s *MemoryStore) Create(task models.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tasks[task.ID]; exists {
		return fmt.Errorf("task %s already exists", task.ID)
	}
	t := task
	s.tasks[task.ID] = &t
	return nil
}

func (s *MemoryStore) Get(id string) (models.Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return models.Task{}, false
	}
	return *t, true
}

func (s *MemoryStore) List() []models.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks := make([]models.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, *t)
	}
	return tasks
}

func (s *MemoryStore) SetStatus(id string, status models.TaskStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	t.Status = status
	if status == models.TaskCompleted || status == models.TaskFailed {
		now := time.Now()
		t.CompletedAt = &now
	}
	return nil
}
