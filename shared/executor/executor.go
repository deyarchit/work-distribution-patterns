package executor

import (
	"context"
	"math/rand"
	"time"

	"work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// Executor runs a task's stages sequentially, emitting progress events.
// MaxStageDuration is set once at construction from server config (MAX_STAGE_DURATION).
// Each stage picks a random duration in [0, MaxStageDuration].
type Executor struct {
	MaxStageDuration time.Duration
}

// Run executes all stages of the task sequentially, emitting a task_status=running
// event at start, a progress event when each stage starts and completes, and a
// terminal task_status=completed or task_status=failed event on exit.
// Context cancellation is detected during each stage's sleep and immediately
// emits a failed status before returning.
func (e *Executor) Run(ctx context.Context, task models.Task, consumer contracts.TaskConsumer) {
	total := len(task.Stages)

	_ = consumer.Emit(ctx, models.TaskEvent{
		Type:   models.EventTaskStatus,
		TaskID: task.ID,
		Status: string(models.TaskRunning),
	})

	for stageIdx, stage := range task.Stages {
		_ = consumer.Emit(ctx, models.TaskEvent{
			Type:      models.EventProgress,
			TaskID:    task.ID,
			StageName: stage.Name,
			Progress:  stageIdx * 100 / total,
		})

		var stageDuration time.Duration
		if e.MaxStageDuration > 0 {
			stageDuration = time.Duration(rand.Int63n(int64(e.MaxStageDuration) + 1))
		}

		select {
		case <-ctx.Done():
			_ = consumer.Emit(ctx, models.TaskEvent{
				Type:   models.EventTaskStatus,
				TaskID: task.ID,
				Status: string(models.TaskFailed),
			})
			return
		case <-time.After(stageDuration):
		}

		_ = consumer.Emit(ctx, models.TaskEvent{
			Type:      models.EventProgress,
			TaskID:    task.ID,
			StageName: stage.Name,
			Progress:  (stageIdx + 1) * 100 / total,
		})
	}

	_ = consumer.Emit(ctx, models.TaskEvent{
		Type:   models.EventTaskStatus,
		TaskID: task.ID,
		Status: string(models.TaskCompleted),
	})
}
