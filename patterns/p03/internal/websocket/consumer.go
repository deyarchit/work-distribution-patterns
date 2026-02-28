package wsinternal

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.TaskConsumer = (*WebSocketConsumer)(nil)

// readyMsg is sent immediately after connecting to signal the worker is idle.
type readyMsg struct {
	Type string `json:"type"`
}

// WebSocketConsumer implements dispatch.TaskConsumer over a WebSocket connection
// to the API. Connect starts the reconnect loop (non-blocking); Receive blocks
// until a task arrives.
type WebSocketConsumer struct {
	apiURL      string
	tasks       chan models.Task
	mu          sync.Mutex
	currentSend chan []byte // guarded by mu; nil when disconnected
}

// NewWebSocketConsumer creates a WebSocketConsumer that connects to the given URL.
func NewWebSocketConsumer(apiURL string) *WebSocketConsumer {
	return &WebSocketConsumer{
		apiURL: apiURL,
		tasks:  make(chan models.Task, 1),
	}
}

// Connect starts the reconnect loop in a background goroutine (non-blocking).
func (c *WebSocketConsumer) Connect(ctx context.Context) error {
	go c.reconnectLoop(ctx)
	return nil
}

func (c *WebSocketConsumer) reconnectLoop(ctx context.Context) {
	for attempt := 0; ; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if attempt > 0 {
			backoff := time.Duration(math.Min(float64(attempt)*500, 10000)) * time.Millisecond
			log.Printf("reconnecting in %s (attempt %d)...", backoff, attempt)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		log.Printf("connecting to %s", c.apiURL)
		conn, _, err := websocket.DefaultDialer.Dial(c.apiURL, nil)
		if err != nil {
			log.Printf("dial error: %v", err)
			continue
		}
		log.Println("connected to API")
		attempt = 0

		send := make(chan []byte, 128)
		c.mu.Lock()
		c.currentSend = send
		c.mu.Unlock()

		if err := c.runConn(ctx, conn, send); err != nil {
			log.Printf("connection error: %v", err)
		}

		c.mu.Lock()
		c.currentSend = nil
		c.mu.Unlock()
	}
}

// Receive blocks until a task is available or ctx is cancelled.
func (c *WebSocketConsumer) Receive(ctx context.Context) (models.Task, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, ctx.Err()
	case task := <-c.tasks:
		return task, nil
	}
}

// Emit sends a TaskEvent to the API over WebSocket.
// Terminal task_status events use a blocking send to ensure delivery.
// All other events are best-effort and may be dropped if the send buffer is full.
func (c *WebSocketConsumer) Emit(_ context.Context, event models.TaskEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	c.mu.Lock()
	send := c.currentSend
	c.mu.Unlock()
	if send == nil {
		return nil
	}
	isTerminal := event.Type == models.EventTaskStatus &&
		(event.Status == string(models.TaskCompleted) || event.Status == string(models.TaskFailed))
	if isTerminal {
		send <- data // blocking: must not lose terminal status
	} else {
		select {
		case send <- data:
		default:
		}
	}
	return nil
}

// runConn manages one connection lifecycle: write pump + read loop.
func (c *WebSocketConsumer) runConn(ctx context.Context, conn *websocket.Conn, send chan []byte) error {
	done := make(chan struct{})

	// Write pump — only goroutine that writes to conn.
	go func() {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case msg, ok := <-send:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if !ok {
					_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					log.Printf("write error: %v", err)
					return
				}
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	ready, err := json.Marshal(readyMsg{Type: "ready"})
	if err != nil {
		return err
	}
	send <- ready

	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			close(send)
			<-done
			_ = conn.Close()
			return nil
		case <-done:
			_ = conn.Close()
			return nil
		default:
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				return err
			}
			return nil
		}
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var msg taskMsg
		if err := json.Unmarshal(raw, &msg); err != nil || msg.Type != msgTypeTask {
			continue
		}
		log.Printf("received task %s (%d stages)", msg.Task.ID, len(msg.Task.Stages))
		select {
		case c.tasks <- msg.Task:
		case <-ctx.Done():
			close(send)
			<-done
			_ = conn.Close()
			return nil
		}
	}
}
