package e2e_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestSingleTask verifies a 3-stage task emits progress events and reaches completed.
func TestSingleTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const stageCount = 3

	// Connect SSE BEFORE submitting task to avoid missing events
	events := SSEClient(ctx, t, "")
	t.Log("SSE connected (global stream)")

	id := postTask(t, "e2e-single", stageCount)
	t.Logf("submitted task %s", id)

	eventCounts := make(map[string]int)
	seenEvents := make(map[string]bool) // for duplicate detection
	taskDone := false

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout: event counts=%v", eventCounts)
		case ev, ok := <-events:
			if !ok {
				t.Fatal("SSE stream closed unexpectedly")
			}
			// Filter to our task only
			if ev.TaskID != id {
				continue
			}

			// Create unique event signature for duplicate detection
			eventSig := fmt.Sprintf("%s:%s:%d:%s", ev.Type, ev.Status, ev.Progress, ev.StageName)
			if seenEvents[eventSig] {
				t.Errorf("DUPLICATE event detected: %s", eventSig)
			}
			seenEvents[eventSig] = true

			eventCounts[ev.Type]++
			t.Logf("event: type=%s status=%s progress=%d stage=%s", ev.Type, ev.Status, ev.Progress, ev.StageName)

			if ev.Type == "task_status" && ev.Status == "completed" {
				taskDone = true
				t.Log("task completed")
			}
			if taskDone {
				// Verify expected event counts
				// Executor emits 2 progress events per stage (start + complete)
				expectedProgress := stageCount * 2
				// Executor emits 2 status events (running, completed)
				expectedStatus := 2

				progressCount := eventCounts["progress"]
				statusCount := eventCounts["task_status"]

				if progressCount != expectedProgress {
					t.Errorf("expected %d progress events, got %d", expectedProgress, progressCount)
				}
				if statusCount != expectedStatus {
					t.Errorf("expected %d task_status events, got %d", expectedStatus, statusCount)
				}
				t.Logf("event counts verified: progress=%d, task_status=%d", progressCount, statusCount)
				return
			}
		}
	}
}

// TestConcurrentTasks submits 3 tasks simultaneously and asserts all complete.
func TestConcurrentTasks(t *testing.T) {
	const numTasks = 3
	const stageCount = 3
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Connect SSE BEFORE submitting tasks
	events := SSEClient(ctx, t, "")
	t.Log("SSE connected (global stream)")

	// Submit all tasks
	ids := make([]string, numTasks)
	for i := range ids {
		ids[i] = postTask(t, "e2e-concurrent", stageCount)
	}
	t.Logf("submitted %d tasks: %v", numTasks, ids)

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	completed := make(map[string]bool)
	eventCounts := make(map[string]map[string]int) // taskID -> eventType -> count
	seenEvents := make(map[string]bool)            // for duplicate detection across all tasks
	var mu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			t.Fatalf("timeout: only %d/%d tasks completed", len(completed), numTasks)
		case ev, ok := <-events:
			if !ok {
				t.Fatal("SSE stream closed unexpectedly")
			}
			if !idSet[ev.TaskID] {
				continue
			}

			mu.Lock()
			// Create unique event signature for duplicate detection
			eventSig := fmt.Sprintf("%s:%s:%s:%d:%s", ev.TaskID, ev.Type, ev.Status, ev.Progress, ev.StageName)
			if seenEvents[eventSig] {
				t.Errorf("DUPLICATE event detected: %s", eventSig)
			}
			seenEvents[eventSig] = true

			// Track event counts per task
			if eventCounts[ev.TaskID] == nil {
				eventCounts[ev.TaskID] = make(map[string]int)
			}
			eventCounts[ev.TaskID][ev.Type]++

			if ev.Type == "task_status" && ev.Status == "completed" {
				completed[ev.TaskID] = true
				done := len(completed)
				t.Logf("task %s completed (%d/%d)", ev.TaskID, done, numTasks)

				// Verify event counts for this completed task
				// Executor emits 2 progress events per stage (start + complete)
				expectedProgress := stageCount * 2
				// Executor emits 2 status events (running, completed)
				expectedStatus := 2

				progressCount := eventCounts[ev.TaskID]["progress"]
				statusCount := eventCounts[ev.TaskID]["task_status"]
				if progressCount != expectedProgress {
					t.Errorf("task %s: expected %d progress events, got %d", ev.TaskID, expectedProgress, progressCount)
				}
				if statusCount != expectedStatus {
					t.Errorf("task %s: expected %d task_status events, got %d", ev.TaskID, expectedStatus, statusCount)
				}

				if done == numTasks {
					mu.Unlock()
					t.Logf("all tasks completed with correct event counts")
					return
				}
			}
			mu.Unlock()
		}
	}
}

// TestStatusTransitions verifies the task emits SSE status events: running → completed.
func TestStatusTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const stageCount = 2

	// Connect SSE BEFORE submitting task
	events := SSEClient(ctx, t, "")
	t.Log("SSE connected (global stream)")

	id := postTask(t, "e2e-transitions", stageCount)
	t.Logf("submitted task %s", id)

	// Immediately check pending (task may have been picked up already)
	task := getTask(t, id)
	if task.Status != "pending" && task.Status != "running" {
		t.Errorf("expected pending or running immediately after submit, got %s", task.Status)
	}

	statusSequence := []string{}
	eventCounts := make(map[string]int)
	seenEvents := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout: status sequence=%v, event counts=%v", statusSequence, eventCounts)
		case ev, ok := <-events:
			if !ok {
				t.Fatal("SSE stream closed")
			}
			// Filter to our task only
			if ev.TaskID != id {
				continue
			}

			// Create unique event signature for duplicate detection
			eventSig := fmt.Sprintf("%s:%s:%d:%s", ev.Type, ev.Status, ev.Progress, ev.StageName)
			if seenEvents[eventSig] {
				t.Errorf("DUPLICATE event detected: %s", eventSig)
			}
			seenEvents[eventSig] = true

			eventCounts[ev.Type]++

			// Track status transitions
			if ev.Type == "task_status" {
				statusSequence = append(statusSequence, ev.Status)
				t.Logf("status transition: %s", ev.Status)
			}

			if ev.Type == "task_status" && ev.Status == "completed" {
				t.Log("saw completed status")

				// Verify expected status sequence: running → completed
				// Note: pending is never emitted as an SSE event (only stored in DB)
				expectedSeq := []string{"running", "completed"}
				if len(statusSequence) != len(expectedSeq) {
					t.Errorf("expected status sequence %v, got %v", expectedSeq, statusSequence)
				} else {
					for i, expected := range expectedSeq {
						if statusSequence[i] != expected {
							t.Errorf("status sequence mismatch at index %d: expected %s, got %s", i, expected, statusSequence[i])
						}
					}
				}

				// Verify event counts
				// Executor emits 2 progress events per stage (start + complete)
				expectedProgress := stageCount * 2
				// Executor emits 2 status events (running, completed)
				expectedStatus := 2

				progressCount := eventCounts["progress"]
				statusCount := eventCounts["task_status"]
				if progressCount != expectedProgress {
					t.Errorf("expected %d progress events, got %d", expectedProgress, progressCount)
				}
				if statusCount != expectedStatus {
					t.Errorf("expected %d task_status events, got %d", expectedStatus, statusCount)
				}

				// Verify final state in storage
				task := getTask(t, id)
				if task.Status != "completed" {
					t.Errorf("GET /tasks/:id: expected status=completed, got %s", task.Status)
				}
				tasks := listTasks(t)
				var found *taskResponse
				for i := range tasks {
					if tasks[i].ID == id {
						found = &tasks[i]
						break
					}
				}
				if found == nil {
					t.Errorf("GET /tasks: task %s not found in list", id)
				} else if found.Status != "completed" {
					t.Errorf("GET /tasks: expected status=completed, got %s", found.Status)
				}

				t.Logf("event counts verified: progress=%d, task_status=%d", progressCount, statusCount)
				return
			}
		}
	}
}
