package rest

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// RESTDispatcher implements contracts.TaskDispatcher.
// Workers poll GET /work/next for tasks and post events via POST /work/events.
type RESTDispatcher struct {
	tasks  chan models.Task
	events chan models.TaskEvent
}

// NewRESTDispatcher creates a RESTDispatcher with a buffered task queue.
func NewRESTDispatcher(queueSize int) *RESTDispatcher {
	return &RESTDispatcher{
		tasks:  make(chan models.Task, queueSize),
		events: make(chan models.TaskEvent, 256),
	}
}

// Start implements contracts.TaskDispatcher — no-op; no background initialisation needed.
func (p *RESTDispatcher) Start(_ context.Context) error { return nil }

// Dispatch implements contracts.TaskDispatcher.
// Non-blocking: returns ErrDispatchFull if the queue is at capacity.
func (p *RESTDispatcher) Dispatch(_ context.Context, task models.Task) error {
	select {
	case p.tasks <- task:
		return nil
	default:
		return contracts.ErrDispatchFull
	}
}

// ReceiveEvent implements contracts.TaskDispatcher.
// Blocks until an event arrives or ctx is cancelled.
func (p *RESTDispatcher) ReceiveEvent(ctx context.Context) (models.TaskEvent, error) {
	select {
	case ev := <-p.events:
		return ev, nil
	case <-ctx.Done():
		return models.TaskEvent{}, ctx.Err()
	}
}

// HandleNext handles GET /work/next.
// Returns 200+JSON if a task is queued, 204 if the queue is empty.
func (p *RESTDispatcher) HandleNext(c echo.Context) error {
	select {
	case task := <-p.tasks:
		return c.JSON(http.StatusOK, task)
	default:
		return c.NoContent(http.StatusNoContent)
	}
}

// HandleEvent handles POST /work/events.
// Terminal events (completed/failed) are forwarded with a blocking send to
// guarantee delivery; non-terminal events are best-effort.
func (p *RESTDispatcher) HandleEvent(c echo.Context) error {
	var ev models.TaskEvent
	if err := c.Bind(&ev); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event body")
	}

	isTerminal := ev.Type == models.EventTaskStatus &&
		(ev.Status == string(models.TaskCompleted) || ev.Status == string(models.TaskFailed))

	if isTerminal {
		select {
		case p.events <- ev:
		case <-c.Request().Context().Done():
		}
	} else {
		select {
		case p.events <- ev:
		default: // drop non-terminal events if buffer is full
		}
	}

	return c.NoContent(http.StatusNoContent)
}
