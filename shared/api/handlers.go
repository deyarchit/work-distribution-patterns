package api

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
)

type submitRequest struct {
	Name       string `json:"name"        form:"name"`
	StageCount int    `json:"stage_count" form:"stage_count"`
}

var stageNames = []string{
	"Initialization", "Validation", "Processing", "Transformation",
	"Aggregation", "Optimization", "Finalization", "Cleanup",
}

// SubmitTask handles POST /tasks.
func SubmitTask(taskStore store.TaskStore, d dispatch.Dispatcher) echo.HandlerFunc {
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
		if req.StageCount > 8 {
			req.StageCount = 8
		}

		stages := make([]models.Stage, req.StageCount)
		for i := 0; i < req.StageCount; i++ {
			name := fmt.Sprintf("Stage %d", i+1)
			if i < len(stageNames) {
				name = stageNames[i]
			}
			stages[i] = models.Stage{
				Index:    i,
				Name:     name,
				Status:   models.StagePending,
				Progress: 0,
			}
		}

		task := models.Task{
			ID:          uuid.New().String(),
			Name:        req.Name,
			Status:      models.TaskPending,
			SubmittedAt: time.Now(),
			Stages:      stages,
		}

		if err := taskStore.Create(task); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		if err := d.Submit(c.Request().Context(), task); err != nil {
			// Pattern-specific errors bubble up as HTTP status
			return err
		}

		// Return HTML fragment for HTMX requests; JSON for everything else.
		// HX-Request: true is always present on HTMX requests — more reliable
		// than checking the Accept header, which HTMX may not set exactly.
		if c.Request().Header.Get("HX-Request") == "true" {
			tpl := c.Get("template").(*template.Template)
			c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
			return tpl.ExecuteTemplate(c.Response().Writer, "task-card", task)
		}

		return c.JSON(http.StatusAccepted, map[string]string{"id": task.ID})
	}
}

// ListTasks handles GET /tasks.
func ListTasks(taskStore store.TaskStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		tasks := taskStore.List()
		if tasks == nil {
			tasks = []models.Task{}
		}
		return c.JSON(http.StatusOK, tasks)
	}
}

// GetTask handles GET /tasks/:id.
func GetTask(taskStore store.TaskStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.Param("id")
		task, ok := taskStore.Get(id)
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

		ch, unsub := hub.Subscribe()
		defer unsub()

		// Send a comment to flush headers immediately
		fmt.Fprintf(c.Response().Writer, ": connected\n\n")
		c.Response().Flush()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		ctx := c.Request().Context()
		for {
			select {
			case <-ctx.Done():
				return nil
			case data := <-ch:
				fmt.Fprintf(c.Response().Writer, "data: %s\n\n", data)
				c.Response().Flush()
			case <-ticker.C:
				// Heartbeat to keep connection alive
				fmt.Fprintf(c.Response().Writer, ": heartbeat\n\n")
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
