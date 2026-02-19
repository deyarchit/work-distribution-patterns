package api

import (
	"html/template"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
)

// NewRouter creates and configures the Echo router with all routes.
func NewRouter(
	taskStore store.TaskStore,
	hub *sse.Hub,
	tpl *template.Template,
	manager dispatch.TaskManager,
) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Attach template to context for HTMX fragment rendering
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("template", tpl)
			return next(c)
		}
	})

	e.POST("/tasks", SubmitTask(manager))
	e.GET("/tasks", ListTasks(taskStore))
	e.GET("/tasks/:id", GetTask(taskStore))
	e.GET("/events", SSEStream(hub))
	e.GET("/", Index(tpl))

	return e
}
