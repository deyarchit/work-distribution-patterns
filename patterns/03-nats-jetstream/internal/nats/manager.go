package natsinternal

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"

	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/store"
)

// NATSTaskManager implements dispatch.TaskManager by persisting the task then
// publishing it to JetStream for worker consumption.
// It owns the API-side task lifecycle: persisting to the store and publishing to
// the queue. Terminal status (running/completed/failed) is written by the
// task_status.* NATS subscription in the API process, which receives those events
// from workers via natsSink.
type NATSTaskManager struct {
	js    nats.JetStreamContext
	store store.TaskStore
}

// NewNATSTaskManager creates a NATSTaskManager backed by JetStream and the given task store.
func NewNATSTaskManager(js nats.JetStreamContext, store store.TaskStore) *NATSTaskManager {
	return &NATSTaskManager{js: js, store: store}
}

// Submit persists the task then publishes it to "tasks.new" on JetStream.
// On publish failure the task is marked failed in the store.
func (m *NATSTaskManager) Submit(_ context.Context, task models.Task) error {
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
