package events

import (
	"context"
	"encoding/json"
	"fmt"

	"work-distribution-patterns/shared/models"

	"github.com/nats-io/nats.go"
)

type NATSEventBus struct {
	nc *nats.Conn
}

func NewNATSEventBus(nc *nats.Conn) *NATSEventBus {
	return &NATSEventBus{nc: nc}
}

func (n *NATSEventBus) Publish(event models.TaskEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	subject := fmt.Sprintf("task.events.%s", event.TaskID)
	_ = n.nc.Publish(subject, data)
}

func (n *NATSEventBus) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error) {
	ch := make(chan models.TaskEvent, 64)
	sub, err := n.nc.Subscribe("task.events.*", func(msg *nats.Msg) {
		var ev models.TaskEvent
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			return
		}
		select {
		case ch <- ev:
		default:
		}
	})
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
		close(ch)
	}()

	return ch, nil
}

func (n *NATSEventBus) Poll(ctx context.Context, afterID int64) ([]StoredEvent, error) {
	// NATS implementation of Poll is not needed if the API subscribes directly.
	// But to satisfy the interface, we can return an error or empty.
	return nil, fmt.Errorf("polling not supported for NATS event bus")
}
