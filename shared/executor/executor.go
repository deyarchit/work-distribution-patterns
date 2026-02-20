package executor

import (
	"context"
	"math/rand"
	"time"

	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/models"
)

// Executor runs a task's stages sequentially, emitting progress events.
// MaxStageDuration is set once at construction from server config (MAX_STAGE_DURATION).
// Each stage picks a random duration in [0, MaxStageDuration].
type Executor struct {
	MaxStageDuration time.Duration
}

// Run executes all stages of the task sequentially, emitting 10 progress ticks
// per stage. Each stage duration is randomized in [0, MaxStageDuration].
// Returns the terminal TaskStatus (TaskCompleted or TaskFailed).
// The caller is responsible for publishing TaskRunning before Run and the
// returned status after Run via ResultSink.
func (e *Executor) Run(ctx context.Context, task models.Task, sink dispatch.ProgressSink) models.TaskStatus {
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

	return models.TaskCompleted
}
