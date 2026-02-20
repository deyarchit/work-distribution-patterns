package bus

import (
	"context"

	"work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
)

// progressSink adapts dispatch.WorkerSource into dispatch.ProgressSink
// so the executor can report stage progress back through the source.
type progressSink struct {
	ctx    context.Context
	source contracts.WorkerSource
}

func (s progressSink) Publish(event models.ProgressEvent) {
	_ = s.source.ReportProgress(s.ctx, event)
}

// RunWorker loops indefinitely, pulling tasks from source, executing them,
// and reporting status and progress back. It exits when ctx is cancelled.
// Each call to RunWorker should run in its own goroutine.
func RunWorker(ctx context.Context, source contracts.WorkerSource, exec *executor.Executor) {
	for {
		task, err := source.Receive(ctx)
		if err != nil {
			return
		}
		sink := progressSink{ctx: ctx, source: source}
		_ = source.ReportResult(ctx, task.ID, models.TaskRunning)
		status := exec.Run(ctx, task, sink)
		_ = source.ReportResult(ctx, task.ID, status)
	}
}
