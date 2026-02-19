package natsinternal

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"

	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
)

// NATSTaskManager implements dispatch.TaskManager by persisting the task then
// publishing it to JetStream for worker consumption.
// It owns the full API-side task lifecycle: persisting to the store, publishing to
// the queue, and receiving progress/status events from workers via NATS Core
// subscriptions that update the SSE hub and store.
type NATSTaskManager struct {
	js    nats.JetStreamContext
	store store.TaskStore
}

// NewNATSTaskManager creates a NATSTaskManager and wires the NATS Core subscriptions
// that route worker progress and terminal status to the SSE hub and task store.
// Every API replica calls this, so all SSE hubs receive all events regardless of
// which replica the browser is connected to — no sticky sessions needed.
func NewNATSTaskManager(nc *nats.Conn, js nats.JetStreamContext, store store.TaskStore, hub *sse.Hub) (*NATSTaskManager, error) {
	m := &NATSTaskManager{js: js, store: store}

	if _, err := nc.Subscribe("progress.*", func(msg *nats.Msg) {
		var ev models.ProgressEvent
		if err := json.Unmarshal(msg.Data, &ev); err == nil {
			hub.Publish(ev)
		}
	}); err != nil {
		return nil, err
	}

	// task_status.* events carry terminal and intermediate status from workers.
	// Both the SSE hub and the task store are updated here; workers never touch the store.
	if _, err := nc.Subscribe("task_status.*", func(msg *nats.Msg) {
		var payload struct {
			TaskID string            `json:"taskID"`
			Status models.TaskStatus `json:"status"`
		}
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			hub.PublishTaskStatus(payload.TaskID, payload.Status)
			_ = store.SetStatus(payload.TaskID, payload.Status)
		}
	}); err != nil {
		return nil, err
	}

	return m, nil
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
