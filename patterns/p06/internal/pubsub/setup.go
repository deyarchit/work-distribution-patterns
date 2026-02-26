package pubsubinternal

import (
	"context"

	"github.com/nats-io/nats.go"
	_ "github.com/pitabwire/natspubsub" // Register nats:// scheme with JetStream support
	"gocloud.dev/pubsub"
)

// EnsureJetStream creates the necessary JetStream streams for the application.
// This is called once on startup by the manager to initialize infrastructure.
func EnsureJetStream(natsURL string) error {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return err
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		return err
	}

	// Create TASKS stream for work distribution
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      "TASKS",
		Subjects:  []string{"tasks.>"},
		Retention: nats.WorkQueuePolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil && err.Error() != "stream name already in use" {
		return err
	}

	// Create EVENTS stream for event streaming
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      "EVENTS",
		Subjects:  []string{"events.>"},
		Retention: nats.InterestPolicy,
		Storage:   nats.MemoryStorage,
	})
	if err != nil && err.Error() != "stream name already in use" {
		return err
	}

	return nil
}

// OpenManagerResources initializes Go Cloud resources for the manager using JetStream URLs.
// Returns: tasksTopic (for dispatching), eventsSub (from workers), eventsTopic (to APIs)
func OpenManagerResources(ctx context.Context, natsURL string) (*pubsub.Topic, *pubsub.Subscription, *pubsub.Topic, error) {
	// Topic for publishing tasks to workers (JetStream-backed)
	// Stream is auto-created by the driver
	tasksTopicURL := natsURL + "/tasks.new?stream_name=TASKS"
	tasksTopic, err := pubsub.OpenTopic(ctx, tasksTopicURL)
	if err != nil {
		return nil, nil, nil, err
	}

	// Subscription for receiving worker events (durable JetStream consumer)
	// Durable consumer ensures crash recovery
	eventsSubURL := natsURL + "/events.workers?stream_name=EVENTS&consumer_durable_name=manager-events"
	eventsSub, err := pubsub.OpenSubscription(ctx, eventsSubURL)
	if err != nil {
		return nil, nil, nil, err
	}

	// Topic for publishing events to APIs (JetStream-backed for replay capability)
	apiEventsTopicURL := natsURL + "/events.api?stream_name=EVENTS"
	apiEventsTopic, err := pubsub.OpenTopic(ctx, apiEventsTopicURL)
	if err != nil {
		return nil, nil, nil, err
	}

	return tasksTopic, eventsSub, apiEventsTopic, nil
}

// OpenWorkerResources initializes Go Cloud resources for workers using JetStream URLs.
func OpenWorkerResources(ctx context.Context, natsURL string) (*pubsub.Subscription, *pubsub.Topic, error) {
	// Subscription for receiving tasks (durable JetStream consumer)
	// Durable consumer ensures crash recovery; work distribution happens automatically
	tasksSubURL := natsURL + "/tasks.new?stream_name=TASKS&consumer_durable_name=workers"
	tasksSub, err := pubsub.OpenSubscription(ctx, tasksSubURL)
	if err != nil {
		return nil, nil, err
	}

	// Topic for publishing worker events (JetStream-backed)
	eventsTopicURL := natsURL + "/events.workers?stream_name=EVENTS"
	eventsTopic, err := pubsub.OpenTopic(ctx, eventsTopicURL)
	if err != nil {
		return nil, nil, err
	}

	return tasksSub, eventsTopic, nil
}

// OpenAPIResources initializes Go Cloud resources for API servers using JetStream URLs.
func OpenAPIResources(ctx context.Context, natsURL string) (*pubsub.Subscription, error) {
	// Subscription for API events (ephemeral consumer - all API instances receive all events for SSE fan-out)
	// No durable name means each API instance gets its own consumer
	eventsSubURL := natsURL + "/events.api?stream_name=EVENTS"
	eventsSub, err := pubsub.OpenSubscription(ctx, eventsSubURL)
	if err != nil {
		return nil, err
	}

	return eventsSub, nil
}
