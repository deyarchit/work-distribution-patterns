package main

import (
	"context"
	"html/template"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/kelseyhightower/envconfig"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	wsinternal "work-distribution-patterns/patterns/p03/internal/websocket"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/events"
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

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	taskStore := store.NewMemoryStore()
	bus := events.NewMemoryEventBus()
	hub := sse.NewHub()
	dispatcher := wsinternal.NewWebSocketDispatcher()
	mgr := manager.New(taskStore, dispatcher, bus, 0)
	mgr.Start(ctx)

	// Pump manager events into SSE hub for API processes to subscribe
	ch, _ := bus.Subscribe(ctx)
	go func() {
		for ev := range ch {
			hub.Publish(ev)
		}
	}()

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
	e.GET("/events", api.SSEStream(hub))

	// Worker WebSocket registration — workers connect here to receive tasks.
	e.GET("/ws/register", func(c echo.Context) error {
		conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		dispatcher.Register(conn)
		return nil
	})

	log.Printf("Pattern 3 (WebSocket Hub) Manager listening on %s", cfg.Addr)
	log.Fatal(e.Start(cfg.Addr))
}
