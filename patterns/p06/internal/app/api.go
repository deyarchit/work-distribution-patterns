package app

import (
	"context"
	"encoding/json"
	"html/template"
	"log"

	"github.com/labstack/echo/v4"

	pubsubinternal "work-distribution-patterns/patterns/p06/internal/pubsub"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/client"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
)

// APIConfig holds runtime parameters for the Pattern 6 API process.
type APIConfig struct {
	ManagerURL string
	BrokerURL  string
}

// NewAPI wires the Pattern 6 API and returns a configured Echo router.
// The caller is responsible for starting the server.
func NewAPI(ctx context.Context, cfg APIConfig) (*echo.Echo, error) {
	eventsSub, err := pubsubinternal.OpenAPIResources(ctx, cfg.BrokerURL)
	if err != nil {
		return nil, err
	}

	taskManager := client.NewTaskManager(cfg.ManagerURL)
	hub := sse.NewHub()

	go func() {
		for {
			msg, err := eventsSub.Receive(ctx)
			if err != nil {
				log.Printf("p06 api: events subscription error: %v", err)
				return
			}

			var ev models.TaskEvent
			if err := json.Unmarshal(msg.Body, &ev); err != nil {
				log.Printf("p06 api: unmarshal event error: %v", err)
				msg.Ack()
				continue
			}

			msg.Ack()
			hub.Publish(ev)
		}
	}()

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		_ = eventsSub.Shutdown(ctx)
		return nil, err
	}

	return api.NewRouter(hub, tpl, taskManager), nil
}
