package app

import (
	"context"
	"html/template"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	pgstore "work-distribution-patterns/patterns/p06/internal/postgres"
	pubsubinternal "work-distribution-patterns/patterns/p06/internal/pubsub"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/templates"
)

// ManagerConfig holds runtime parameters for the Pattern 6 manager process.
type ManagerConfig struct {
	BrokerURL   string
	DatabaseURL string
}

// NewManager wires the Pattern 6 manager and returns a configured Echo router.
// The caller is responsible for starting the server.
func NewManager(ctx context.Context, cfg ManagerConfig) (*echo.Echo, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	taskStore, err := pgstore.New(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}

	res, err := pubsubinternal.OpenManagerResources(ctx, cfg.BrokerURL)
	if err != nil {
		pool.Close()
		return nil, err
	}

	dispatcher := pubsubinternal.NewPubSubDispatcher(res.TasksTopic, res.WorkerEventsSub)
	if startErr := dispatcher.Start(ctx); startErr != nil {
		dispatcher.Shutdown(ctx)
		pool.Close()
		return nil, startErr
	}

	eventBridge := pubsubinternal.NewPubSubEventBridge(res.APIEventsTopic)
	mgr := manager.New(taskStore, dispatcher, eventBridge, 30*time.Second)
	mgr.Start(ctx)

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		dispatcher.Shutdown(ctx)
		pool.Close()
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

	return e, nil
}
