package app

import (
	"context"
	"html/template"

	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/client"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"

	"github.com/labstack/echo/v4"
)

// APIConfig holds runtime parameters for the Pattern 3 API process.
type APIConfig struct {
	ManagerURL string
}

// NewAPI wires the Pattern 3 API and returns a configured Echo router.
// The caller is responsible for starting the server.
func NewAPI(ctx context.Context, cfg APIConfig) (*echo.Echo, error) {
	taskManager := client.NewTaskManager(cfg.ManagerURL)
	hub := sse.NewHub()

	sseClient := sse.NewClient(cfg.ManagerURL + "/events")
	ch, _ := sseClient.Subscribe(ctx)
	go func() {
		for ev := range ch {
			hub.Publish(ev)
		}
	}()

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		return nil, err
	}

	return api.NewRouter(hub, tpl, taskManager), nil
}
