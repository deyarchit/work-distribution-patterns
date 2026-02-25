package events

import (
	"context"
	"encoding/json"
	"fmt"

	"work-distribution-patterns/shared/models"

	"github.com/nats-io/nats.go"
)

// NATSBridge implements TaskEventBridge using NATS Core.
type NATSBridge struct {
	nc *nats.Conn
}

func NewNATSBridge(nc *nats.Conn) *NATSBridge {
	return &NATSBridge{nc: nc}
}

func (n *NATSBridge) Publish(event models.TaskEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	subject := fmt.Sprintf("task.events.%s", event.TaskID)
	_ = n.nc.Publish(subject, data)
}

func (n *NATSBridge) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error) {
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
