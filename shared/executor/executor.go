package executor

import (
	"context"
	"math/rand"
	"time"

	"work-distribution-patterns/shared/models"
)

// ProgressSink receives progress updates from the executor — pure transport concern.
// Implementations route events to the appropriate mechanism (SSE hub, WebSocket, NATS).
// The task manager (not the worker) is responsible for persisting task status to the store.
type ProgressSink interface {
	Publish(event models.ProgressEvent)
	PublishTaskStatus(taskID string, status models.TaskStatus)
}

// Executor runs a task's stages sequentially, emitting progress events.
// MaxStageDuration is set once at construction from server config (MAX_STAGE_DURATION).
// Each stage picks a random duration in [0, MaxStageDuration].
type Executor struct {
	MaxStageDuration time.Duration
}

// Run executes all stages of the task sequentially, emitting 10 progress ticks
// per stage. Each stage duration is randomized in [0, MaxStageDuration].
// Returns the terminal TaskStatus (TaskCompleted or TaskFailed).
func (e *Executor) Run(ctx context.Context, task models.Task, sink ProgressSink) models.TaskStatus {
	sink.PublishTaskStatus(task.ID, models.TaskRunning)

	for _, stage := range task.Stages {
		var tickDuration time.Duration
		if e.MaxStageDuration > 0 {
			tickDuration = time.Duration(rand.Int63n(int64(e.MaxStageDuration)+1)) / 10
		}
		sink.Publish(models.ProgressEvent{
			TaskID:    task.ID,
			StageIdx:  stage.Index,
			StageName: stage.Name,
			Progress:  0,
			Status:    models.StageRunning,
		})

		for tick := 1; tick <= 10; tick++ {
			select {
			case <-ctx.Done():
				sink.PublishTaskStatus(task.ID, models.TaskFailed)
				return models.TaskFailed
			case <-time.After(tickDuration):
			}

			sink.Publish(models.ProgressEvent{
				TaskID:    task.ID,
				StageIdx:  stage.Index,
				StageName: stage.Name,
				Progress:  tick * 10,
				Status:    models.StageRunning,
			})
		}

		sink.Publish(models.ProgressEvent{
			TaskID:    task.ID,
			StageIdx:  stage.Index,
			StageName: stage.Name,
			Progress:  100,
			Status:    models.StageCompleted,
		})
	}

	sink.PublishTaskStatus(task.ID, models.TaskCompleted)
	return models.TaskCompleted
}
