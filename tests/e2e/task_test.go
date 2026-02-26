package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Collect events until 1 second of silence
	collected := collectEventsUntilQuiet(ctx, t, events, []string{id}, 1*time.Second)

	// Verify task reached completed status
	assert.True(t, collected.SeenCompleted, "task should reach completed status")

	// Executor emits 2 progress events per stage (start + complete)
	expectedProgress := stageCount * 2
	// Executor emits 2 status events (running, completed)
	expectedStatus := 2

	assert.Equal(t, expectedProgress, collected.EventCounts["progress"], "progress event count")
	assert.Equal(t, expectedStatus, collected.EventCounts["task_status"], "task_status event count")
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

	// Collect events until 1 second of silence
	collected := collectEventsUntilQuiet(ctx, t, events, ids, 1*time.Second)

	// Verify event counts for each task
	// Executor emits 2 progress events per stage (start + complete)
	expectedProgress := stageCount * 2
	// Executor emits 2 status events (running, completed)
	expectedStatus := 2

	for _, id := range ids {
		assert.Equal(t, expectedProgress, collected.PerTask[id]["progress"],
			"task %s: progress event count", id)
		assert.Equal(t, expectedStatus, collected.PerTask[id]["task_status"],
			"task %s: task_status event count", id)
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
	require.Contains(t, []string{"pending", "running"}, task.Status,
		"expected pending or running immediately after submit")

	// Collect events until 1 second of silence
	collected := collectEventsUntilQuiet(ctx, t, events, []string{id}, 1*time.Second)

	// Verify expected status sequence: running → completed
	// Note: pending is never emitted as an SSE event (only stored in DB)
	expectedSeq := []string{"running", "completed"}
	assert.Equal(t, expectedSeq, collected.StatusSeq, "status transition sequence")

	// Verify event counts
	// Executor emits 2 progress events per stage (start + complete)
	expectedProgress := stageCount * 2
	// Executor emits 2 status events (running, completed)
	expectedStatus := 2

	assert.Equal(t, expectedProgress, collected.EventCounts["progress"], "progress event count")
	assert.Equal(t, expectedStatus, collected.EventCounts["task_status"], "task_status event count")

	// Verify final state in storage
	task = getTask(t, id)
	assert.Equal(t, "completed", task.Status, "GET /tasks/:id final status")

	tasks := listTasks(t)
	var found *taskResponse
	for i := range tasks {
		if tasks[i].ID == id {
			found = &tasks[i]
			break
		}
	}
	require.NotNil(t, found, "task %s should be in list", id)
	assert.Equal(t, "completed", found.Status, "GET /tasks final status")
}
