package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/shared/models"
)

// RemoteTaskManager implements contracts.TaskManager by proxying Submit/Get/List
// to the manager process over HTTP. Used by API processes in Patterns 2, 3, and 5.
//
// Events are handled separately — each pattern's API subscribes to events using
// its native transport (SSE for P2/P3, NATS for P5).
type RemoteTaskManager struct {
	baseURL    string
	httpClient *http.Client
}

// NewTaskManager creates a client connected to a Manager process.
func NewTaskManager(baseURL string) *RemoteTaskManager {
	return &RemoteTaskManager{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
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
