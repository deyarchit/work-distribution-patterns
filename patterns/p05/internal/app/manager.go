package app

import (
	"context"
	"html/template"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nats-io/nats.go"

	natsinternal "work-distribution-patterns/patterns/p05/internal/nats"
	pgstore "work-distribution-patterns/patterns/p05/internal/postgres"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/templates"
)

// ManagerConfig holds runtime parameters for the Pattern 5 manager process.
type ManagerConfig struct {
	NATSURL     string
	DatabaseURL string
}

// NewManager wires the Pattern 5 manager and returns a configured Echo router.
// Connects to NATS and Postgres; sets up JetStream streams.
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

	nc, err := nats.Connect(cfg.NATSURL,
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
	)
	if err != nil {
		pool.Close()
		return nil, err
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		pool.Close()
		return nil, err
	}

	if err := natsinternal.SetupJetStream(js); err != nil {
		// Non-fatal: streams may already exist.
		_ = err
	}

	bus := events.NewNATSBridge(nc, "task.events")
	dispatcher := natsinternal.NewNATSDispatcher(nc, js)
	mgr := manager.New(taskStore, dispatcher, bus, 30*time.Second)
	mgr.Start(ctx)

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		nc.Close()
		pool.Close()
		return nil, err
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{ //nolint:staticcheck
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
