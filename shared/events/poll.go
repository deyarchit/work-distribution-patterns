package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"work-distribution-patterns/shared/models"
)

// PollingEventBus implements events.TaskEventBus by long-polling a remote
// manager's /events/poll endpoint.
type PollingEventBus struct {
	baseURL    string
	httpClient *http.Client
}

// NewPollingEventBus creates a bus that fetches events via HTTP polling.
func NewPollingEventBus(baseURL string) *PollingEventBus {
	return &PollingEventBus{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second}, // Longer timeout for long-polling
	}
}

func (p *PollingEventBus) Publish(event models.TaskEvent) {
	// Not implemented for client-side polling bus.
}

func (p *PollingEventBus) Poll(ctx context.Context, afterID int64) ([]StoredEvent, error) {
	// Not implemented for client-side polling bus.
	// The client uses Subscribe() which internally polls the remote manager.
	return nil, fmt.Errorf("Poll not supported on client-side PollingEventBus")
}

func (p *PollingEventBus) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error) {
	out := make(chan models.TaskEvent, 64)
	go p.pollLoop(ctx, out)
	return out, nil
}

func (p *PollingEventBus) pollLoop(ctx context.Context, out chan<- models.TaskEvent) {
	defer close(out)

	var lastID int64
	for {
		if ctx.Err() != nil {
			return
		}

		url := fmt.Sprintf("%s/events/poll?afterID=%d", p.baseURL, lastID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return
		}

		resp, err := p.httpClient.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		var evs []StoredEvent
		err = json.NewDecoder(resp.Body).Decode(&evs)
		_ = resp.Body.Close()

		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		for _, e := range evs {
			select {
			case out <- e.Event:
				if e.ID > lastID {
					lastID = e.ID
				}
			case <-ctx.Done():
				return
			}
		}
	}
}
