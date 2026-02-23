package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/models"
)

// RemoteTaskManager implements contracts.TaskManager by proxying all calls
// to the manager process over HTTP. Used by the API process in Patterns 2, 3, and 4.
type RemoteTaskManager struct {
	baseURL    string
	httpClient *http.Client
}

// NewRemoteTaskManager creates a RemoteTaskManager targeting the given base URL.
func NewRemoteTaskManager(baseURL string) *RemoteTaskManager {
	return &RemoteTaskManager{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
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

// Subscribe opens an SSE connection to GET /events on the manager and forwards
// all TaskEvents to the returned channel. The goroutine reconnects on disconnect
// with a 2 s backoff and closes the channel when ctx is cancelled.
func (m *RemoteTaskManager) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error) {
	out := make(chan models.TaskEvent, 64)
	go m.sseLoop(ctx, out)
	return out, nil
}

func (m *RemoteTaskManager) sseLoop(ctx context.Context, out chan<- models.TaskEvent) {
	defer close(out)

	// Separate client with no timeout — SSE connections are long-lived.
	sseClient := &http.Client{}

	for {
		if ctx.Err() != nil {
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/events", nil)
		if err != nil {
			return
		}

		resp, err := sseClient.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			var ev models.TaskEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}

			select {
			case out <- ev:
			case <-ctx.Done():
				_ = resp.Body.Close()
				return
			}
		}
		_ = resp.Body.Close()

		// Reconnect after disconnect.
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
