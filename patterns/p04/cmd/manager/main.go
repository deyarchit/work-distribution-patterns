package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kelseyhightower/envconfig"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nats-io/nats.go"

	natsinternal "work-distribution-patterns/patterns/p04/internal/nats"
	pgstore "work-distribution-patterns/patterns/p04/internal/postgres"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/templates"
)

type config struct {
	Addr        string `envconfig:"addr" default:":8081"`
	NATSURL     string `envconfig:"nats_url" default:"nats://127.0.0.1:4222"`
	DatabaseURL string `envconfig:"database_url" default:"postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pool.Close()

	taskStore, err := pgstore.New(ctx, pool)
	if err != nil {
		log.Fatalf("postgres store: %v", err)
	}

	nc, err := nats.Connect(cfg.NATSURL,
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
	)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	if err := natsinternal.SetupJetStream(js); err != nil {
		log.Printf("setup warning: %v", err)
	}

	bus := events.NewNATSEventBus(nc)
	dispatcher := natsinternal.NewNATSDispatcher(nc, js)
	mgr := manager.New(taskStore, dispatcher, bus, 30*time.Second)
	mgr.Start(ctx)

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	// Build the router manually — the manager accepts a fully-formed Task
	// (pre-created by the API with models.NewTask) rather than a submit request.
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

	// Accept a fully-formed Task forwarded by the API process.
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

	log.Printf("Pattern 4 (Queue-and-Store) Manager listening on %s", cfg.Addr)
	log.Fatal(e.Start(cfg.Addr))
}
