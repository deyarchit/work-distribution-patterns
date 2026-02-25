package main

import (
	"context"
	"html/template"
	"log"
	"net"
	"net/http"

	"github.com/kelseyhightower/envconfig"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"google.golang.org/grpc"

	grpcinternal "work-distribution-patterns/patterns/p04/internal/grpc"
	pb "work-distribution-patterns/patterns/p04/proto"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
	"work-distribution-patterns/shared/templates"
)

type config struct {
	HTTPAddr string `envconfig:"http_addr" default:":8081"`
	GRPCAddr string `envconfig:"grpc_addr" default:":9091"`
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

	// Create gRPC server
	grpcServer := grpcinternal.NewServer(taskStore, bus)
	dispatcher := grpcinternal.NewDispatcher(grpcServer)

	// Start manager (handles task lifecycle and event routing)
	mgr := manager.New(taskStore, dispatcher, bus, 0)
	mgr.Start(ctx)

	// Pump manager events into SSE hub for API processes to subscribe
	ch, _ := bus.Subscribe(ctx)
	go func() {
		for ev := range ch {
			hub.Publish(ev)
		}
	}()

	// Start gRPC server in background
	go func() {
		lis, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			log.Fatalf("failed to listen on %s: %v", cfg.GRPCAddr, err)
		}

		s := grpc.NewServer()
		pb.RegisterWorkDistributionServer(s, grpcServer)
		pb.RegisterTaskManagerServer(s, grpcServer)

		log.Printf("gRPC server listening on %s", cfg.GRPCAddr)
		if err := s.Serve(lis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// HTTP API for compatibility with existing patterns (for remote API to call)
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

	// HTTP endpoint for API to submit tasks (compatibility with shared/client.RemoteTaskManager)
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

	log.Printf("Pattern 4 (gRPC Streaming) Manager HTTP API on %s, gRPC on %s", cfg.HTTPAddr, cfg.GRPCAddr)
	log.Fatal(e.Start(cfg.HTTPAddr))
}
