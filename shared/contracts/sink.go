package contracts

import (
	"context"

	"work-distribution-patterns/shared/models"
)

// EventSink is the minimal interface the executor uses to emit task events.
// TaskConsumer automatically satisfies EventSink (same Emit signature).
type EventSink interface {
	Emit(ctx context.Context, event models.TaskEvent) error
}
