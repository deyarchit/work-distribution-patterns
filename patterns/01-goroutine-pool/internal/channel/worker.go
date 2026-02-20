package channel

import (
	"context"

	"work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
)

// RunWorker loops indefinitely, pulling tasks from source, executing them,
// and reporting status and progress back. It exits when ctx is cancelled.
// Each call to RunWorker should run in its own goroutine.
func RunWorker(ctx context.Context, source contracts.TaskConsumer, exec *executor.Executor) {
	for {
		task, err := source.Receive(ctx)
		if err != nil {
			return
		}
		_ = source.ReportResult(ctx, task.ID, models.TaskRunning)
		status := exec.Run(ctx, task, source)
		_ = source.ReportResult(ctx, task.ID, status)
	}
}
