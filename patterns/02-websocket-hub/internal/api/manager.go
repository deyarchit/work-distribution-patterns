package api

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/store"
)

// WSTaskManager implements dispatch.TaskManager by persisting the task then
// forwarding it to an available worker via the WorkerHub.
// It owns the task lifecycle on the API side: persisting to the store, dispatching
// to a worker, and marking the task failed if no worker is available.
// Terminal status (completed/failed from execution) is written by WorkerHub.readPump
// when it receives a DoneMsg from the worker.
type WSTaskManager struct {
	hub   *WorkerHub
	store store.TaskStore
}

// NewWSTaskManager creates a WSTaskManager backed by the given WorkerHub and task store.
func NewWSTaskManager(hub *WorkerHub, store store.TaskStore) *WSTaskManager {
	return &WSTaskManager{hub: hub, store: store}
}

// Submit persists the task then assigns it to an idle worker.
// Returns HTTP 503 if no worker is available; on unavailability the task is marked failed.
func (m *WSTaskManager) Submit(_ context.Context, task models.Task) error {
	if err := m.store.Create(task); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := m.hub.Assign(task); err == ErrNoWorkersAvailable {
		_ = m.store.SetStatus(task.ID, models.TaskFailed)
		return echo.NewHTTPError(http.StatusServiceUnavailable, "no workers available — retry later").
			SetInternal(err)
	} else if err != nil {
		_ = m.store.SetStatus(task.ID, models.TaskFailed)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return nil
}
