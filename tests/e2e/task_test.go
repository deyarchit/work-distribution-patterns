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

	id := postTask(t, "e2e-single", 3)
	t.Logf("submitted task %s", id)

	events := SSEClient(ctx, t, id)

	progressSeen := false
	taskDone := false

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout: progressSeen=%v, taskDone=%v", progressSeen, taskDone)
		case ev, ok := <-events:
			if !ok {
				t.Fatal("SSE stream closed unexpectedly")
			}
			if ev.Type == "progress" {
				progressSeen = true
				t.Logf("progress: %d%% (stage: %s)", ev.Progress, ev.StageName)
			}
			if ev.Type == "task_status" && ev.Status == "completed" {
				taskDone = true
				t.Log("task completed")
			}
			if taskDone && progressSeen {
				return
			}
		}
	}
}

// TestConcurrentTasks submits 3 tasks simultaneously and asserts all complete.
func TestConcurrentTasks(t *testing.T) {
	const numTasks = 3
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Submit all tasks first, then open per-task SSE connections.
	ids := make([]string, numTasks)
	for i := range ids {
		ids[i] = postTask(t, "e2e-concurrent", 3)
	}
	t.Logf("submitted %d tasks: %v", numTasks, ids)

	// Fan-in all per-task SSE streams into a single merged channel.
	merged := make(chan SSEEvent, 256)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(taskID string) {
			defer wg.Done()
			ch := SSEClient(ctx, t, taskID)
			for ev := range ch {
				select {
				case merged <- ev:
				case <-ctx.Done():
					return
				}
			}
		}(id)
	}
	go func() {
		wg.Wait()
		close(merged)
	}()

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}

	completed := make(map[string]bool)
	var mu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			t.Fatalf("timeout: only %d/%d tasks completed", len(completed), numTasks)
		case ev, ok := <-merged:
			if !ok {
				t.Fatal("SSE stream closed unexpectedly")
			}
			if !idSet[ev.TaskID] {
				continue
			}
			if ev.Type == "task_status" && ev.Status == "completed" {
				mu.Lock()
				completed[ev.TaskID] = true
				done := len(completed)
				mu.Unlock()
				t.Logf("task %s completed (%d/%d)", ev.TaskID, done, numTasks)
				if done == numTasks {
					return
				}
			}
		}
	}
}

// TestProgressBeforeCompletion simulates browser behaviour: the browser
// closes its SSE connection as soon as task_status=completed arrives.
// Any progress events that haven't been delivered yet are therefore lost.
// This test detects the ordering race where task_status=completed is published
// before the final progress=100 event, leaving the bar stuck at <100% in the UI.
//
// With the unified single-channel design the ordering is guaranteed by
// construction, so the test should pass reliably. It runs numRuns times to
// confirm there is no regression.
func TestProgressBeforeCompletion(t *testing.T) {
	const (
		numStages = 3
		numRuns   = 5
	)

	for run := range numRuns {
		t.Run(fmt.Sprintf("run%d", run), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			id := postTask(t, "e2e-progress-order", numStages)
			t.Logf("submitted task %s", id)

			events := SSEClient(ctx, t, id)

			maxProgress := 0

			for {
				select {
				case <-ctx.Done():
					t.Fatalf("timeout; max progress at cut-off: %d%%", maxProgress)
				case ev, ok := <-events:
					if !ok {
						t.Fatal("SSE stream closed unexpectedly")
					}
					switch ev.Type {
					case "progress":
						if ev.Progress > maxProgress {
							maxProgress = ev.Progress
						}
					case "task_status":
						if ev.Status != "completed" {
							continue
						}
						// Simulate browser: stop reading now.
						// The progress bar must already be at 100%.
						cancel()
						if maxProgress != 100 {
							t.Errorf("progress=%d%% at task_status=completed, want 100%%", maxProgress)
						}
						return
					}
				}
			}
		})
	}
}

// TestStatusTransitions verifies the task goes through pending → running → completed.
func TestStatusTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id := postTask(t, "e2e-transitions", 2)
	t.Logf("submitted task %s", id)

	// Immediately check pending (task may have been picked up already)
	task := getTask(t, id)
	if task.Status != "pending" && task.Status != "running" {
		t.Errorf("expected pending or running immediately after submit, got %s", task.Status)
	}

	events := SSEClient(ctx, t, id)

	for {
		select {
		case <-ctx.Done():
			t.Fatal("timeout waiting for completed status")
		case ev, ok := <-events:
			if !ok {
				t.Fatal("SSE stream closed")
			}
			// running is SSE-only (UI signal); skip store check to avoid the
			// race where the event fires before the client connects.
			if ev.Type == "task_status" && ev.Status == "completed" {
				t.Log("saw completed status")
				task := getTask(t, id)
				if task.Status != "completed" {
					t.Errorf("expected store status=completed, got %s", task.Status)
				}
				return
			}
		}
	}
}
