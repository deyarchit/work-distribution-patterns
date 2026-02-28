package api //nolint:revive

import (
	"html/template"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/sse"
)

// NewRouter creates and configures the Echo router with all routes.
func NewRouter(
	hub *sse.Hub,
	tpl *template.Template,
	manager contracts.TaskManager,
) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	// Logger skips /health so frequent health-check polls don't flood the log.
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{ //nolint:staticcheck // deprecated but still functional; sufficient for demo
		Skipper: func(c echo.Context) bool {
			return c.Request().URL.Path == "/health"
		},
	}))
	e.Use(middleware.Recover())

	e.GET("/health", Health())

	// Attach template to context for HTMX fragment rendering
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("template", tpl)
			return next(c)
		}
	})

	e.POST("/tasks", SubmitTask(manager))
	e.GET("/tasks", ListTasks(manager))
	e.GET("/tasks/:id", GetTask(manager))
	e.GET("/events", SSEStream(hub))
	e.GET("/", Index(tpl))

	return e
}
