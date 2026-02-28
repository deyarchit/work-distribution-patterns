package testutil

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// SubmitRequest is the POST /tasks body.
type SubmitRequest struct {
	Name       string `json:"name"`
	StageCount int    `json:"stage_count"`
}

// SubmitResponse is the 202 response from POST /tasks.
type SubmitResponse struct {
	ID string `json:"id"`
}

// TaskResponse mirrors models.Task for JSON decoding.
type TaskResponse struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Status string          `json:"status"`
	Stages []StageResponse `json:"stages"`
}

// StageResponse mirrors models.Stage for JSON decoding.
type StageResponse struct {
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

// CollectedEvents contains the result of event collection.
type CollectedEvents struct {
	EventCounts   map[string]int            // eventType -> count (for single task)
	PerTask       map[string]map[string]int // taskID -> eventType -> count (for multiple tasks)
	StatusSeq     []string                  // sequence of status events
	SeenCompleted bool                      // whether task_status=completed was seen
}

// SSEClient connects to the /events endpoint and emits parsed events on a channel.
// If taskID is non-empty, connects to /events?taskID=<taskID> for scoped delivery.
// If taskID is empty, connects to /events for global delivery (all tasks).
// Returns when ctx is cancelled or the connection drops.
func SSEClient(ctx context.Context, t *testing.T, baseURL, taskID string) <-chan SSEEvent {
	t.Helper()
	ch := make(chan SSEEvent, 256)

	url := baseURL + "/events"
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

// PostTask submits a task and returns its ID.
func PostTask(t *testing.T, baseURL, name string, stageCount int) string {
	t.Helper()
	body, _ := json.Marshal(SubmitRequest{
		Name:       name,
		StageCount: stageCount,
	})
	resp, err := http.Post(baseURL+"/tasks", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("POST /tasks: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	var sr SubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	return sr.ID
}

// ListTasks fetches all tasks from GET /tasks.
func ListTasks(t *testing.T, baseURL string) []TaskResponse {
	t.Helper()
	resp, err := http.Get(baseURL + "/tasks")
	if err != nil {
		t.Fatalf("GET /tasks: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /tasks: expected 200, got %d", resp.StatusCode)
	}
	var tasks []TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode task list: %v", err)
	}
	return tasks
}

// GetTask fetches a task by ID.
func GetTask(t *testing.T, baseURL, id string) TaskResponse {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL, id))
	if err != nil {
		t.Fatalf("GET /tasks/%s: %v", id, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var tr TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	return tr
}

// DoGet performs a GET request and returns the HTTP status code.
// Returns (0, err) on connection failure.
func DoGet(url string) (int, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// WaitReady polls baseURL/health until it returns 200 or times out after 10 seconds.
func WaitReady(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		code, err := DoGet(baseURL + "/health")
		if err == nil && code == http.StatusOK {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready within 10s", baseURL)
}

// WaitForWorker submits a probe task and retries until a worker accepts it (HTTP 202).
// This is needed for patterns where workers register asynchronously (P3 WebSocket, P4 gRPC).
// It also waits for the probe task to complete so the worker is idle before the suite runs.
// The probe task runs normally; its events are filtered out by RunSuite's per-task tracking.
func WaitForWorker(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, _ := json.Marshal(SubmitRequest{Name: "probe", StageCount: 1})
		resp, err := http.Post(baseURL+"/tasks", "application/json", strings.NewReader(string(body))) //nolint:noctx
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var sr SubmitResponse
		_ = json.NewDecoder(resp.Body).Decode(&sr)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusAccepted {
			// Wait for the probe task to complete so the worker is free before the suite starts.
			waitForTaskCompletion(baseURL, sr.ID, 10*time.Second)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no workers became available within 5s")
}

// waitForTaskCompletion polls GET /tasks/<id> until status is terminal or timeout elapses.
func waitForTaskCompletion(baseURL, taskID string, timeout time.Duration) {
	if taskID == "" {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("%s/tasks/%s", baseURL, taskID)) //nolint:noctx
		if err == nil {
			var tr TaskResponse
			if json.NewDecoder(resp.Body).Decode(&tr) == nil && (tr.Status == "completed" || tr.Status == "failed") {
				_ = resp.Body.Close()
				return
			}
			_ = resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// CollectEventsUntilQuiet collects events from the SSE channel for the specified task IDs
// until no events are received for the quietDuration. Returns collected event data.
func CollectEventsUntilQuiet(ctx context.Context, t *testing.T, events <-chan SSEEvent, taskIDs []string, quietDuration time.Duration) CollectedEvents {
	t.Helper()

	idSet := make(map[string]bool)
	for _, id := range taskIDs {
		idSet[id] = true
	}

	result := CollectedEvents{
		EventCounts: make(map[string]int),
		PerTask:     make(map[string]map[string]int),
		StatusSeq:   []string{},
	}

	seenEvents := make(map[string]bool)
	quiescenceTimer := time.NewTimer(quietDuration)
	defer quiescenceTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for events")
		case <-quiescenceTimer.C:
			return result
		case ev, ok := <-events:
			if !ok {
				t.Fatal("SSE stream closed unexpectedly")
			}
			if !idSet[ev.TaskID] {
				continue
			}

			if !quiescenceTimer.Stop() {
				select {
				case <-quiescenceTimer.C:
				default:
				}
			}
			quiescenceTimer.Reset(quietDuration)

			eventSig := fmt.Sprintf("%s:%s:%s:%d:%s", ev.TaskID, ev.Type, ev.Status, ev.Progress, ev.StageName)
			if seenEvents[eventSig] {
				t.Errorf("DUPLICATE event detected: %s", eventSig)
			}
			seenEvents[eventSig] = true

			result.EventCounts[ev.Type]++

			if result.PerTask[ev.TaskID] == nil {
				result.PerTask[ev.TaskID] = make(map[string]int)
			}
			result.PerTask[ev.TaskID][ev.Type]++

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
