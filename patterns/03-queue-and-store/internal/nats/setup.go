package natsinternal

import (
	"log"

	"github.com/nats-io/nats.go"
)

const (
	StreamName  = "TASKS"
	StreamSubj  = "tasks.>"
	ConsumerDur = "workers"
)

// SetupJetStream idempotently creates the TASKS stream.
// Safe to call from multiple services on startup.
func SetupJetStream(js nats.JetStreamContext) error {
	_, err := js.AddStream(&nats.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{StreamSubj},
		Retention: nats.WorkQueuePolicy,
	})
	if err != nil {
		log.Printf("stream setup: %v (may already exist)", err)
	}
	return nil
}
