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

	pgstore "work-distribution-patterns/patterns/p06/internal/postgres"
	pubsubinternal "work-distribution-patterns/patterns/p06/internal/pubsub"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/templates"
)

type config struct {
	Addr        string `envconfig:"addr" default:":8081"`
	BrokerURL   string `envconfig:"broker_url" default:"nats://localhost:4222"`
	DatabaseURL string `envconfig:"database_url" default:"postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Setup Postgres
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pool.Close()

	taskStore, err := pgstore.New(ctx, pool)
	if err != nil {
		log.Fatalf("postgres store: %v", err)
	}

	// 2. Setup PubSub (Go Cloud with NATS/RabbitMQ)
	tasksTopic, workerEventsSub, apiEventsTopic, err := pubsubinternal.OpenManagerResources(ctx, cfg.BrokerURL)
	if err != nil {
		log.Fatalf("pubsub setup: %v", err)
	}

	dispatcher := pubsubinternal.NewPubSubDispatcher(tasksTopic, workerEventsSub)
	defer dispatcher.Shutdown(ctx)

	// Start dispatcher to receive worker events
	if err := dispatcher.Start(ctx); err != nil {
		log.Fatalf("dispatcher start: %v", err)
	}

	// Event bridge for publishing to APIs (manager republishes worker events)
	eventBridge := pubsubinternal.NewPubSubEventBridge(apiEventsTopic)

	// 4. Setup Manager
	mgr := manager.New(taskStore, dispatcher, eventBridge, 30*time.Second)
	mgr.Start(ctx)

	// 5. Serve Manager API
	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
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

	log.Printf("Pattern 06 (Cloud-Agnostic) Manager listening on %s [broker=%s, postgres=%s]",
		cfg.Addr, cfg.BrokerURL, cfg.DatabaseURL)
	log.Fatal(e.Start(cfg.Addr))
}
