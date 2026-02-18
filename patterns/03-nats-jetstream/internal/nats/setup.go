package natsinternal

import (
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	StreamName  = "TASKS"
	StreamSubj  = "tasks.>"
	KVBucket    = "task-store"
	ConsumerDur = "workers"
)

// SetupJetStream idempotently creates the TASKS stream and task-store KV bucket.
// Safe to call from multiple services on startup.
func SetupJetStream(js nats.JetStreamContext) error {
	// Create stream (idempotent)
	_, err := js.AddStream(&nats.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{StreamSubj},
		Retention: nats.WorkQueuePolicy,
	})
	if err != nil {
		// If stream already exists with the same config, ErrStreamNameAlreadyInUse is returned
		// by older versions; newer versions return nil. Either way, continue.
		log.Printf("stream setup: %v (may already exist)", err)
	}

	// Create KV bucket (idempotent)
	_, err = js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: KVBucket,
		TTL:    24 * time.Hour,
	})
	if err != nil {
		log.Printf("KV setup: %v (may already exist)", err)
	}

	return nil
}
