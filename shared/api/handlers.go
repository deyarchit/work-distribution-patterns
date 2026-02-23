package api

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
)

type submitRequest struct {
	Name       string `json:"name"        form:"name"`
	StageCount int    `json:"stage_count" form:"stage_count"`
}

// SubmitTask handles POST /tasks.
// The task manager is responsible for persisting the task and dispatching it.
func SubmitTask(manager contracts.TaskManager) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req submitRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		if req.Name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name is required")
		}
		if req.StageCount < 1 {
			req.StageCount = 3
		}

		task := models.NewTask(req.Name, req.StageCount)

		if err := manager.Submit(c.Request().Context(), task); err != nil {
			return err
		}

		// Return HTML fragment for HTMX requests; JSON for everything else.
		if c.Request().Header.Get("HX-Request") == "true" {
			tpl := c.Get("template").(*template.Template)
			c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
			return tpl.ExecuteTemplate(c.Response().Writer, "task-card", task)
		}

		return c.JSON(http.StatusAccepted, map[string]string{"id": task.ID})
	}
}

// ListTasks handles GET /tasks.
func ListTasks(manager contracts.TaskManager) echo.HandlerFunc {
	return func(c echo.Context) error {
		tasks := manager.List(c.Request().Context())
		return c.JSON(http.StatusOK, tasks)
	}
}

// GetTask handles GET /tasks/:id.
func GetTask(manager contracts.TaskManager) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.Param("id")
		task, ok := manager.Get(c.Request().Context(), id)
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, "task not found")
		}
		return c.JSON(http.StatusOK, task)
	}
}

// SSEStream handles GET /events — streams SSE events to the client.
func SSEStream(hub *sse.Hub) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")
		c.Response().Header().Set("X-Accel-Buffering", "no")
		c.Response().WriteHeader(http.StatusOK)

		taskID := c.QueryParam("taskID")
		ch, unsub := hub.Subscribe(taskID)
		defer unsub()

		// Send a comment to flush headers immediately
		_, _ = fmt.Fprintf(c.Response().Writer, ": connected\n\n")
		c.Response().Flush()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		ctx := c.Request().Context()
		for {
			select {
			case <-ctx.Done():
				return nil
			case data := <-ch:
				_, _ = fmt.Fprintf(c.Response().Writer, "data: %s\n\n", data)
				c.Response().Flush()
			case <-ticker.C:
				// Heartbeat to keep connection alive
				_, _ = fmt.Fprintf(c.Response().Writer, ": heartbeat\n\n")
				c.Response().Flush()
			}
		}
	}
}

// Index handles GET / — serves the HTMX frontend.
func Index(tpl *template.Template) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return tpl.ExecuteTemplate(c.Response().Writer, "index.html", nil)
	}
}

// Health handles GET /health — used by Docker health checks.
// Registered before the logger middleware so health poll traffic is not logged.
func Health() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}
}
