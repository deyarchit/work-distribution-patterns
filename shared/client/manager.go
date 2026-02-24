package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/events"
	"work-distribution-patterns/shared/models"
)

// RemoteTaskManager implements contracts.TaskManager by proxying all calls
// to the manager process over HTTP. Used by the API process in Patterns 2, 3, and 4.
type RemoteTaskManager struct {
	baseURL    string
	httpClient *http.Client
	bus        events.TaskEventBus
}

// NewRemoteTaskManager creates a client connected to a Manager API process.
// If bus is provided, Subscribe uses it directly (e.g., NATS). Otherwise, it falls back to HTTP polling.
func NewRemoteTaskManager(baseURL string, bus events.TaskEventBus) *RemoteTaskManager {
	return &RemoteTaskManager{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		bus:        bus,
	}
}

// Events returns the underlying event bus if one is configured.
func (m *RemoteTaskManager) Events() events.TaskEventBus {
	return m.bus
}

// Submit sends a fully-formed Task to POST /tasks on the manager.
// Maps 429 and 503 responses to the corresponding echo HTTP errors.
func (m *RemoteTaskManager) Submit(ctx context.Context, task models.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/tasks", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusAccepted, http.StatusOK:
		return nil
	case http.StatusTooManyRequests:
		return echo.NewHTTPError(http.StatusTooManyRequests, "task queue is full — retry later")
	case http.StatusServiceUnavailable:
		return echo.NewHTTPError(http.StatusServiceUnavailable, "no workers available — retry later")
	default:
		return fmt.Errorf("manager returned %d", resp.StatusCode)
	}
}

// Get fetches a single task from GET /tasks/:id on the manager.
// Returns (Task{}, false) on 404 or any error.
func (m *RemoteTaskManager) Get(ctx context.Context, id string) (models.Task, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/tasks/"+id, nil)
	if err != nil {
		return models.Task{}, false
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return models.Task{}, false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return models.Task{}, false
	}

	var task models.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return models.Task{}, false
	}
	return task, true
}

// List fetches all tasks from GET /tasks on the manager.
// Returns an empty slice on any error.
func (m *RemoteTaskManager) List(ctx context.Context) []models.Task {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/tasks", nil)
	if err != nil {
		return []models.Task{}
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return []models.Task{}
	}
	defer func() { _ = resp.Body.Close() }()

	var tasks []models.Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return []models.Task{}
	}
	if tasks == nil {
		return []models.Task{}
	}
	return tasks
}

// Subscribe polls GET /events/poll on the manager and forwards
// all TaskEvents to the returned channel. The goroutine reconnects on failure
// with a 2 s backoff and closes the channel when ctx is cancelled.
// If a bus was provided during creation, it calls Subscribe on the bus directly.
func (m *RemoteTaskManager) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error) {
	if m.bus != nil {
		return m.bus.Subscribe(ctx)
	}
	out := make(chan models.TaskEvent, 64)
	go m.pollLoop(ctx, out)
	return out, nil
}

func (m *RemoteTaskManager) pollLoop(ctx context.Context, out chan<- models.TaskEvent) {
	defer close(out)

	var lastID int64
	for {
		if ctx.Err() != nil {
			return
		}

		url := fmt.Sprintf("%s/events/poll?afterID=%d", m.baseURL, lastID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return
		}

		resp, err := m.httpClient.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		var evs []events.StoredEvent
		err = json.NewDecoder(resp.Body).Decode(&evs)
		_ = resp.Body.Close()

		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		for _, e := range evs {
			select {
			case out <- e.Event:
				if e.ID > lastID {
					lastID = e.ID
				}
			case <-ctx.Done():
				return
			}
		}
	}
}
