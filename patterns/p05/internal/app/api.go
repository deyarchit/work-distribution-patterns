package app

import (
	"context"
	"html/template"

	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"

	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/client"
	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
)

// APIConfig holds runtime parameters for the Pattern 5 API process.
type APIConfig struct {
	ManagerURL string
	NATSURL    string
}

// NewAPI wires the Pattern 5 API and returns a configured Echo router.
// The caller is responsible for starting the server.
func NewAPI(ctx context.Context, cfg APIConfig) (*echo.Echo, error) {
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		return nil, err
	}

	taskManager := client.NewTaskManager(cfg.ManagerURL)
	hub := sse.NewHub()

	bus := events.NewNATSBridge(nc, "task.events")
	ch, _ := bus.Subscribe(ctx)
	go func() {
		for ev := range ch {
			hub.Publish(ev)
		}
	}()

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		nc.Close()
		return nil, err
	}

	return api.NewRouter(hub, tpl, taskManager), nil
}
