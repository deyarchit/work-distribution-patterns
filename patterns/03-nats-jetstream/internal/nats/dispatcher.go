package natsinternal

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"

	"work-distribution-patterns/shared/models"
)

// NATSDispatcher implements dispatch.Dispatcher by publishing tasks to JetStream.
type NATSDispatcher struct {
	js nats.JetStreamContext
}

// NewNATSDispatcher creates a NATSDispatcher that publishes to the TASKS stream.
func NewNATSDispatcher(js nats.JetStreamContext) *NATSDispatcher {
	return &NATSDispatcher{js: js}
}

// Submit serializes the task and publishes it to "tasks.new" on JetStream.
// task.StageDurationSecs is included in the payload — no extra parameters needed.
func (d *NATSDispatcher) Submit(_ context.Context, task models.Task) error {
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = d.js.Publish("tasks.new", payload)
	return err
}
