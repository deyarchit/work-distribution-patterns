package app

import (
	"context"
	"html/template"

	"github.com/labstack/echo/v4"

	grpcinternal "work-distribution-patterns/patterns/p04/internal/grpc"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
)

// APIConfig holds runtime parameters for the Pattern 4 API process.
type APIConfig struct {
	ManagerGRPCAddr string
}

// NewAPI wires the Pattern 4 API and returns a configured Echo router.
// The API connects to the manager via gRPC for task management and event streaming.
// The caller is responsible for starting the server.
func NewAPI(ctx context.Context, cfg APIConfig) (*echo.Echo, error) {
	taskManager, err := grpcinternal.NewClient(cfg.ManagerGRPCAddr)
	if err != nil {
		return nil, err
	}

	hub := sse.NewHub()

	eventChan, err := taskManager.Subscribe(ctx, "")
	if err != nil {
		return nil, err
	}

	go func() {
		for ev := range eventChan {
			hub.Publish(ev)
		}
	}()

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		return nil, err
	}

	return api.NewRouter(hub, tpl, taskManager), nil
}
