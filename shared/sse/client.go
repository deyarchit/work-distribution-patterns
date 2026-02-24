package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"work-distribution-patterns/shared/models"
)

// Client connects to a remote SSE endpoint and streams TaskEvents.
// Used by P2/P3 API processes to subscribe to manager's event stream.
type Client struct {
	url        string
	httpClient *http.Client
}

// NewClient creates an SSE client that connects to the given URL.
func NewClient(url string) *Client {
	return &Client{
		url: url,
		httpClient: &http.Client{
			Timeout: 0, // No timeout for SSE streaming
		},
	}
}

// Subscribe connects to the SSE endpoint and returns a channel of events.
// The connection is maintained until ctx is cancelled.
// Automatically reconnects on disconnection with exponential backoff.
func (c *Client) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error) {
	ch := make(chan models.TaskEvent, 64)
	go c.streamWithReconnect(ctx, ch)
	return ch, nil
}

func (c *Client) streamWithReconnect(ctx context.Context, out chan<- models.TaskEvent) {
	defer close(out)

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		_ = c.streamEvents(ctx, out)
		if ctx.Err() != nil {
			return
		}

		// Connection dropped, reconnect with backoff
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (c *Client) streamEvents(ctx context.Context, out chan<- models.TaskEvent) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE endpoint returned %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var ev models.TaskEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue // Skip malformed events
			}

			select {
			case out <- ev:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return scanner.Err()
}
