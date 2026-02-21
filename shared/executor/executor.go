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
// event at start, 10 overall-progress ticks per stage, and a terminal
// task_status=completed or task_status=failed event on exit.
// The sink receives all events in order through a single channel, eliminating
// the ordering race between progress and status events.
func (e *Executor) Run(ctx context.Context, task models.Task, sink contracts.EventSink) {
	total := len(task.Stages) * 10 // total ticks across all stages

	_ = sink.Emit(ctx, models.TaskEvent{
		Type:   models.EventTaskStatus,
		TaskID: task.ID,
		Status: string(models.TaskRunning),
	})

	for stageIdx, stage := range task.Stages {
		var tickDuration time.Duration
		if e.MaxStageDuration > 0 {
			tickDuration = time.Duration(rand.Int63n(int64(e.MaxStageDuration)+1)) / 10
		}

		for tick := 1; tick <= 10; tick++ {
			select {
			case <-ctx.Done():
				_ = sink.Emit(ctx, models.TaskEvent{
					Type:   models.EventTaskStatus,
					TaskID: task.ID,
					Status: string(models.TaskFailed),
				})
				return
			case <-time.After(tickDuration):
			}

			overallProgress := (stageIdx*10 + tick) * 100 / total
			_ = sink.Emit(ctx, models.TaskEvent{
				Type:      models.EventProgress,
				TaskID:    task.ID,
				StageName: stage.Name,
				Progress:  overallProgress,
			})
		}
	}

	_ = sink.Emit(ctx, models.TaskEvent{
		Type:   models.EventTaskStatus,
		TaskID: task.ID,
		Status: string(models.TaskCompleted),
	})
}
