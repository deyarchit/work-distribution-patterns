package pubsubinternal

import (
	"context"
	"encoding/json"
	"log"

	"work-distribution-patterns/shared/models"

	"gocloud.dev/pubsub"
)

// PubSubEventBridge implements TaskEventPublisher using Go Cloud PubSub.
type PubSubEventBridge struct {
	topic *pubsub.Topic
}

// NewPubSubEventBridge creates an event bridge from a pubsub.Topic.
func NewPubSubEventBridge(topic *pubsub.Topic) *PubSubEventBridge {
	return &PubSubEventBridge{topic: topic}
}

// Publish sends an event to the topic (for manager→API event broadcasting).
func (b *PubSubEventBridge) Publish(event models.TaskEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("pubsub event bridge marshal error: %v", err)
		return
	}

	err = b.topic.Send(context.Background(), &pubsub.Message{
		Body: data,
		Metadata: map[string]string{
			"task_id": event.TaskID,
			"type":    event.Type,
		},
	})
	if err != nil {
		log.Printf("pubsub event bridge publish error: %v", err)
	}
}
