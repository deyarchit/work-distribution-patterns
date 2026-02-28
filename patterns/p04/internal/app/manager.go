package app

import (
	"context"
	"html/template"
	"net/http"

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

// ManagerComponents holds the HTTP router and the configured gRPC server.
// The caller is responsible for providing listeners and starting both servers.
type ManagerComponents struct {
	// Router is the HTTP Echo server for task CRUD and SSE.
	Router *echo.Echo
	// GRPCServer is the gRPC server, already registered with all services.
	// Start it with: go GRPCServer.Serve(ln)
	GRPCServer *grpc.Server
}

// ManagerConfig holds runtime parameters for the Pattern 4 manager process.
type ManagerConfig struct{}

// NewManager wires all Pattern 4 manager components.
// Returns ManagerComponents whose Router and GRPCServer the caller must start separately.
func NewManager(ctx context.Context, _ ManagerConfig) (*ManagerComponents, error) {
	taskStore := store.NewMemoryStore()
	bus := events.NewMemoryBridge()
	hub := sse.NewHub()

	grpcServer := grpcinternal.NewServer(taskStore, bus)
	dispatcher := grpcinternal.NewDispatcher(grpcServer)

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

	s := grpc.NewServer()
	pb.RegisterWorkDistributionServer(s, grpcServer)
	pb.RegisterTaskManagerServer(s, grpcServer)

	return &ManagerComponents{
		Router:     e,
		GRPCServer: s,
	}, nil
}
