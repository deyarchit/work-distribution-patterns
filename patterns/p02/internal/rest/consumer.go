package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"work-distribution-patterns/shared/models"
)

// RESTConsumer implements contracts.TaskConsumer.
// It polls the manager's REST endpoints for tasks and emits events back.
type RESTConsumer struct {
	managerURL string
	httpClient *http.Client
}

// NewRESTConsumer creates a RESTConsumer that targets the given manager base URL.
func NewRESTConsumer(managerURL string) *RESTConsumer {
	return &RESTConsumer{
		managerURL: managerURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Connect implements contracts.TaskConsumer — no-op; polling starts in Receive.
func (c *RESTConsumer) Connect(_ context.Context) error { return nil }

// Receive implements contracts.TaskConsumer.
// Polls GET /work/next: 204 → sleep 500 ms; 200 → decode and return; error → sleep 2 s.
// All sleeps respect ctx cancellation.
func (c *RESTConsumer) Receive(ctx context.Context) (models.Task, error) {
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.managerURL+"/work/next", nil)
		if err != nil {
			return models.Task{}, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return models.Task{}, ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}

		switch resp.StatusCode {
		case http.StatusNoContent:
			_ = resp.Body.Close() //nolint:errcheck
			select {
			case <-ctx.Done():
				return models.Task{}, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}

		case http.StatusOK:
			var task models.Task
			decErr := json.NewDecoder(resp.Body).Decode(&task)
			_ = resp.Body.Close() //nolint:errcheck
			if decErr != nil {
				select {
				case <-ctx.Done():
					return models.Task{}, ctx.Err()
				case <-time.After(2 * time.Second):
				}
				continue
			}
			return task, nil

		default:
			_ = resp.Body.Close() //nolint:errcheck
			select {
			case <-ctx.Done():
				return models.Task{}, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}
}

// Emit implements contracts.TaskConsumer.
// Posts the event as JSON to POST /work/events on the manager.
func (c *RESTConsumer) Emit(ctx context.Context, event models.TaskEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.managerURL+"/work/events", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close() //nolint:errcheck
	return nil
}
