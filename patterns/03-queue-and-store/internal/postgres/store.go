package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"work-distribution-patterns/shared/models"
)

const schema = `
CREATE TABLE IF NOT EXISTS tasks (
	id            TEXT        PRIMARY KEY,
	name          TEXT        NOT NULL,
	status        TEXT        NOT NULL,
	stages        JSONB       NOT NULL,
	submitted_at  TIMESTAMPTZ NOT NULL,
	dispatched_at TIMESTAMPTZ,
	completed_at  TIMESTAMPTZ
)`

// Store implements store.TaskStore against PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store and runs the schema migration.
func New(ctx context.Context, pool *pgxpool.Pool) (*Store, error) {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Create(task models.Task) error {
	stages, err := json.Marshal(task.Stages)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO tasks (id, name, status, stages, submitted_at, dispatched_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		task.ID, task.Name, string(task.Status), stages,
		task.SubmittedAt, task.DispatchedAt, task.CompletedAt,
	)
	return err
}

func (s *Store) Get(id string) (models.Task, bool) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, name, status, stages, submitted_at, dispatched_at, completed_at
		 FROM tasks WHERE id = $1`, id)
	task, err := scanTask(row)
	if err != nil {
		return models.Task{}, false
	}
	return task, true
}

func (s *Store) List() []models.Task {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, name, status, stages, submitted_at, dispatched_at, completed_at
		 FROM tasks ORDER BY submitted_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func (s *Store) SetStatus(id string, status models.TaskStatus) error {
	var completedAt *time.Time
	if status == models.TaskCompleted || status == models.TaskFailed {
		now := time.Now()
		completedAt = &now
	}
	_, err := s.pool.Exec(context.Background(),
		`UPDATE tasks SET status = $1, completed_at = $2 WHERE id = $3`,
		string(status), completedAt, id,
	)
	return err
}

func (s *Store) SetDispatchedAt(id string, t time.Time) error {
	_, err := s.pool.Exec(context.Background(),
		`UPDATE tasks SET dispatched_at = $1 WHERE id = $2`, t, id)
	return err
}

// scanner abstracts pgx.Row and pgx.Rows so scanTask works for both.
type scanner interface {
	Scan(dest ...any) error
}

func scanTask(s scanner) (models.Task, error) {
	var t models.Task
	var status string
	var stagesJSON string
	if err := s.Scan(
		&t.ID, &t.Name, &status, &stagesJSON,
		&t.SubmittedAt, &t.DispatchedAt, &t.CompletedAt,
	); err != nil {
		return models.Task{}, err
	}
	t.Status = models.TaskStatus(status)
	if err := json.Unmarshal([]byte(stagesJSON), &t.Stages); err != nil {
		return models.Task{}, err
	}
	return t, nil
}
