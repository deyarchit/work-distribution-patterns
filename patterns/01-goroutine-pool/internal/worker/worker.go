package worker

import (
	"context"

	"work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/executor"
)

// RunWorker loops indefinitely, pulling tasks from consumer and executing them.
// The executor emits all status and progress events via consumer (TaskConsumer).
// Each call to RunWorker should run in its own goroutine.
func RunWorker(ctx context.Context, consumer contracts.TaskConsumer, exec *executor.Executor) {
	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			return
		}
		exec.Run(ctx, task, consumer)
	}
}
