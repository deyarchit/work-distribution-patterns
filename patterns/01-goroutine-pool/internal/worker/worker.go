package worker

import (
	"context"

	"work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/executor"
)

// RunWorker loops indefinitely, pulling tasks from source and executing them.
// The executor emits all status and progress events via source (EventSink).
// Each call to RunWorker should run in its own goroutine.
func RunWorker(ctx context.Context, source contracts.TaskConsumer, exec *executor.Executor) {
	for {
		task, err := source.Receive(ctx)
		if err != nil {
			return
		}
		exec.Run(ctx, task, source)
	}
}
