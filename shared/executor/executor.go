package executor

import (
	"context"
	"time"

	"work-distribution-patterns/shared/models"
)

// ProgressSink receives progress updates from the executor.
// *sse.Hub satisfies this interface directly.
type ProgressSink interface {
	Publish(event models.ProgressEvent)
	PublishTaskStatus(taskID string, status models.TaskStatus)
}

// Executor runs a task's stages sequentially, emitting progress events.
// StageDuration is set once at construction from server config (STAGE_DURATION_SECS).
type Executor struct {
	StageDuration time.Duration
}

// Run executes all stages of the task sequentially, emitting 10 progress ticks
// per stage (StageDuration/10 sleep each tick).
func (e *Executor) Run(ctx context.Context, task models.Task, sink ProgressSink) {
	sink.PublishTaskStatus(task.ID, models.TaskRunning)

	tickDuration := e.StageDuration / 10

	for _, stage := range task.Stages {
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
				return
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
}
