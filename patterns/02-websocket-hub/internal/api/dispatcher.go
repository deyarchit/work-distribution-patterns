package api

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/models"
)

// WSDispatcher implements dispatch.Dispatcher by forwarding tasks to workers
// via the WorkerHub.
type WSDispatcher struct {
	hub *WorkerHub
}

// NewWSDispatcher creates a WSDispatcher backed by the given WorkerHub.
func NewWSDispatcher(hub *WorkerHub) *WSDispatcher {
	return &WSDispatcher{hub: hub}
}

// Submit assigns the task to an idle worker. Returns HTTP 503 if no worker is available.
func (d *WSDispatcher) Submit(_ context.Context, task models.Task) error {
	if err := d.hub.Assign(task); err == ErrNoWorkersAvailable {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "no workers available — retry later").
			SetInternal(err)
	} else if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return nil
}
