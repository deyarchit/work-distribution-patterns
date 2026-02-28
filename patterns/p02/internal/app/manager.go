package app

import (
	"context"
	"html/template"
	"net/http"

	restinternal "work-distribution-patterns/patterns/p02/internal/rest"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
	"work-distribution-patterns/shared/templates"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// ManagerConfig holds runtime parameters for the Pattern 2 manager process.
type ManagerConfig struct {
	WorkersQueueSize int
}

// NewManager wires the Pattern 2 manager and returns a configured Echo router.
// The router exposes task CRUD, SSE stream, and REST worker polling endpoints.
// The caller is responsible for starting the server.
func NewManager(ctx context.Context, cfg ManagerConfig) (*echo.Echo, error) {
	taskStore := store.NewMemoryStore()
	bus := events.NewMemoryBridge()
	hub := sse.NewHub()
	dispatcher := restinternal.NewRESTDispatcher(cfg.WorkersQueueSize)
	mgr := manager.New(taskStore, dispatcher, bus, 0)
	mgr.Start(ctx)

	ch, err := bus.Subscribe(ctx)
	if err != nil {
		return nil, err
	}
	go func() {
		for ev := range ch {
			hub.Publish(ev)
		}
	}()

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		return nil, err
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{ //nolint:staticcheck // deprecated but still functional; sufficient for demo
		Skipper: func(c echo.Context) bool {
			return c.Request().URL.Path == "/health"
		},
	}))
	e.Use(middleware.Recover())

	e.GET("/health", api.Health())

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("template", tpl)
			return next(c)
		}
	})

	e.POST("/tasks", func(c echo.Context) error {
		var task models.Task
		if err := c.Bind(&task); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid task body")
		}
		if err := mgr.Submit(c.Request().Context(), task); err != nil {
			return err
		}
		return c.JSON(http.StatusAccepted, map[string]string{"id": task.ID})
	})

	e.GET("/tasks", api.ListTasks(mgr))
	e.GET("/tasks/:id", api.GetTask(mgr))
	e.GET("/events", api.SSEStream(hub))
	e.GET("/", api.Index(tpl))

	e.GET("/work/next", dispatcher.HandleNext)
	e.POST("/work/events", dispatcher.HandleEvent)

	return e, nil
}
