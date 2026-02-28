package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RunSuite runs the standard set of integration tests against baseURL.
// It is intended to be called from pattern-specific integration test files.
func RunSuite(t *testing.T, baseURL string) {
	t.Helper()

	t.Run("SingleTask", func(t *testing.T) {
		testSingleTask(t, baseURL)
	})
	t.Run("ConcurrentTasks", func(t *testing.T) {
		testConcurrentTasks(t, baseURL)
	})
	t.Run("StatusTransitions", func(t *testing.T) {
		testStatusTransitions(t, baseURL)
	})
}

// testSingleTask verifies a 3-stage task emits progress events and reaches completed.
func testSingleTask(t *testing.T, baseURL string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const stageCount = 3

	events := SSEClient(ctx, t, baseURL, "")
	t.Log("SSE connected (global stream)")

	id := PostTask(t, baseURL, "integration-single", stageCount)
	t.Logf("submitted task %s", id)

	collected := CollectEventsUntilQuiet(ctx, t, events, []string{id}, 1*time.Second)

	assert.True(t, collected.SeenCompleted, "task should reach completed status")

	expectedProgress := stageCount * 2
	expectedStatus := 2
	assert.Equal(t, expectedProgress, collected.EventCounts["progress"], "progress event count")
	assert.Equal(t, expectedStatus, collected.EventCounts["task_status"], "task_status event count")
}

// testConcurrentTasks submits 3 tasks simultaneously and asserts all complete.
func testConcurrentTasks(t *testing.T, baseURL string) {
	t.Helper()
	const numTasks = 3
	const stageCount = 3
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	events := SSEClient(ctx, t, baseURL, "")
	t.Log("SSE connected (global stream)")

	ids := make([]string, numTasks)
	for i := range ids {
		ids[i] = PostTask(t, baseURL, "integration-concurrent", stageCount)
	}
	t.Logf("submitted %d tasks: %v", numTasks, ids)

	collected := CollectEventsUntilQuiet(ctx, t, events, ids, 1*time.Second)

	expectedProgress := stageCount * 2
	expectedStatus := 2

	for _, id := range ids {
		assert.Equal(t, expectedProgress, collected.PerTask[id]["progress"],
			"task %s: progress event count", id)
		assert.Equal(t, expectedStatus, collected.PerTask[id]["task_status"],
			"task %s: task_status event count", id)
	}
}

// testStatusTransitions verifies the task emits SSE status events: running → completed.
func testStatusTransitions(t *testing.T, baseURL string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const stageCount = 2

	events := SSEClient(ctx, t, baseURL, "")
	t.Log("SSE connected (global stream)")

	id := PostTask(t, baseURL, "integration-transitions", stageCount)
	t.Logf("submitted task %s", id)

	task := GetTask(t, baseURL, id)
	require.Contains(t, []string{"pending", "running"}, task.Status,
		"expected pending or running immediately after submit")

	collected := CollectEventsUntilQuiet(ctx, t, events, []string{id}, 1*time.Second)

	expectedSeq := []string{"running", "completed"}
	assert.Equal(t, expectedSeq, collected.StatusSeq, "status transition sequence")

	expectedProgress := stageCount * 2
	expectedStatus := 2
	assert.Equal(t, expectedProgress, collected.EventCounts["progress"], "progress event count")
	assert.Equal(t, expectedStatus, collected.EventCounts["task_status"], "task_status event count")

	task = GetTask(t, baseURL, id)
	assert.Equal(t, "completed", task.Status, "GET /tasks/:id final status")

	tasks := ListTasks(t, baseURL)
	var found *TaskResponse
	for i := range tasks {
		if tasks[i].ID == id {
			found = &tasks[i]
			break
		}
	}
	require.NotNil(t, found, "task %s should be in list", id)
	assert.Equal(t, "completed", found.Status, "GET /tasks final status")
}
