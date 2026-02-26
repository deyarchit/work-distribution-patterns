package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// baseURL returns the target API base URL (default: http://localhost:8080).
func baseURL() string {
	if u := os.Getenv("BASE_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

// submitRequest is the POST /tasks body.
type submitRequest struct {
	Name       string `json:"name"`
	StageCount int    `json:"stage_count"`
}

// submitResponse is the 202 response from POST /tasks.
type submitResponse struct {
	ID string `json:"id"`
}

// taskResponse mirrors models.Task for JSON decoding.
type taskResponse struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Status string          `json:"status"`
	Stages []stageResponse `json:"stages"`
}

type stageResponse struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}

// SSEEvent represents a parsed server-sent event.
type SSEEvent struct {
	Type      string `json:"type"`
	TaskID    string `json:"taskID"`
	StageName string `json:"stageName"`
	Progress  int    `json:"progress"`
	Status    string `json:"status"`
}

// SSEClient connects to the /events endpoint and emits parsed events on a channel.
// If taskID is non-empty, connects to /events?taskID=<taskID> for scoped delivery.
// If taskID is empty, connects to /events for global delivery (all tasks).
// Returns when ctx is cancelled or the connection drops.
func SSEClient(ctx context.Context, t *testing.T, taskID string) <-chan SSEEvent {
	t.Helper()
	ch := make(chan SSEEvent, 256)

	url := baseURL() + "/events"
	if taskID != "" {
		url += "?taskID=" + taskID
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req) //nolint:bodyclose // closed in goroutine below or explicit error path
	if err != nil {
		t.Logf("SSE connect error: %v", err)
		close(ch)
		return ch
	}
	if resp.StatusCode != http.StatusOK {
		t.Logf("SSE non-200: %d", resp.StatusCode)
		_ = resp.Body.Close()
		close(ch)
		return ch
	}

	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			raw := strings.TrimPrefix(line, "data: ")
			var ev SSEEvent
			if err := json.Unmarshal([]byte(raw), &ev); err != nil {
				continue
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}

// postTask submits a task and returns its ID.
func postTask(t *testing.T, name string, stageCount int) string {
	t.Helper()
	body, _ := json.Marshal(submitRequest{
		Name:       name,
		StageCount: stageCount,
	})
	resp, err := http.Post(baseURL()+"/tasks", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("POST /tasks: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	var sr submitResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	return sr.ID
}

// listTasks fetches all tasks from GET /tasks.
func listTasks(t *testing.T) []taskResponse {
	t.Helper()
	resp, err := http.Get(baseURL() + "/tasks")
	if err != nil {
		t.Fatalf("GET /tasks: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /tasks: expected 200, got %d", resp.StatusCode)
	}
	var tasks []taskResponse
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode task list: %v", err)
	}
	return tasks
}

// getTask fetches a task by ID.
func getTask(t *testing.T, id string) taskResponse {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL(), id))
	if err != nil {
		t.Fatalf("GET /tasks/%s: %v", id, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var tr taskResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	return tr
}

// collectedEvents contains the result of event collection.
type collectedEvents struct {
	EventCounts   map[string]int            // eventType -> count (for single task)
	PerTask       map[string]map[string]int // taskID -> eventType -> count (for multiple tasks)
	StatusSeq     []string                  // sequence of status events
	SeenCompleted bool                      // whether task_status=completed was seen
}

// collectEventsUntilQuiet collects events from the SSE channel for the specified task IDs
// until no events are received for the quietDuration. Returns collected event data.
func collectEventsUntilQuiet(ctx context.Context, t *testing.T, events <-chan SSEEvent, taskIDs []string, quietDuration time.Duration) collectedEvents {
	t.Helper()

	idSet := make(map[string]bool)
	for _, id := range taskIDs {
		idSet[id] = true
	}

	result := collectedEvents{
		EventCounts: make(map[string]int),
		PerTask:     make(map[string]map[string]int),
		StatusSeq:   []string{},
	}

	seenEvents := make(map[string]bool) // for duplicate detection
	quiescenceTimer := time.NewTimer(quietDuration)
	defer quiescenceTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for events")
		case <-quiescenceTimer.C:
			// Quiescence reached - all events collected
			return result
		case ev, ok := <-events:
			if !ok {
				t.Fatal("SSE stream closed unexpectedly")
			}
			// Filter to our task IDs only
			if !idSet[ev.TaskID] {
				continue
			}

			// Reset quiescence timer on each new event
			if !quiescenceTimer.Stop() {
				select {
				case <-quiescenceTimer.C:
				default:
				}
			}
			quiescenceTimer.Reset(quietDuration)

			// Check for duplicates
			eventSig := fmt.Sprintf("%s:%s:%s:%d:%s", ev.TaskID, ev.Type, ev.Status, ev.Progress, ev.StageName)
			if seenEvents[eventSig] {
				t.Errorf("DUPLICATE event detected: %s", eventSig)
			}
			seenEvents[eventSig] = true

			// Track overall event counts (for single task tests)
			result.EventCounts[ev.Type]++

			// Track per-task event counts (for concurrent task tests)
			if result.PerTask[ev.TaskID] == nil {
				result.PerTask[ev.TaskID] = make(map[string]int)
			}
			result.PerTask[ev.TaskID][ev.Type]++

			// Track status sequence
			if ev.Type == "task_status" {
				result.StatusSeq = append(result.StatusSeq, ev.Status)
				if ev.Status == "completed" {
					result.SeenCompleted = true
				}
			}

			t.Logf("event: task=%s type=%s status=%s progress=%d stage=%s", ev.TaskID, ev.Type, ev.Status, ev.Progress, ev.StageName)
		}
	}
}
