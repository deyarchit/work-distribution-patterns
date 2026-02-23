package main

import (
	"context"
	"html/template"
	"log"
	"net/http"

	"github.com/kelseyhightower/envconfig"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	restinternal "work-distribution-patterns/patterns/02-rest-polling/internal/rest"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
	"work-distribution-patterns/shared/templates"
)

type config struct {
	Addr             string `envconfig:"addr" default:":8081"`
	WorkersQueueSize int    `envconfig:"workers_queue_size" default:"20"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	taskStore := store.NewMemoryStore()
	hub := sse.NewHub()
	producer := restinternal.NewRESTProducer(cfg.WorkersQueueSize)
	mgr := manager.New(taskStore, producer, hub, 0)
	mgr.Start(ctx)

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	// Build the router manually to avoid the route collision on POST /tasks
	// that api.NewRouter would cause — the manager needs a custom handler that
	// accepts a fully-formed Task (not the {name, stage_count} submit request).
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

	// Accept a fully-formed Task forwarded by the API process.
	// The API already called models.NewTask(...); we must not create a new UUID.
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

	// Worker polling endpoints.
	e.GET("/work/next", producer.HandleNext)
	e.POST("/work/events", producer.HandleEvent)

	log.Printf("Pattern 2 (REST Polling) Manager listening on %s [queue=%d]",
		cfg.Addr, cfg.WorkersQueueSize)
	log.Fatal(e.Start(cfg.Addr))
}
