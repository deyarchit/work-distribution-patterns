package worker

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

// Message type constants — must match the API bus protocol.
const (
	msgTypeReady    = "ready"
	msgTypeTask     = "task"
	msgTypeProgress = "progress"
	msgTypeStatus   = "status"
	msgTypeDone     = "done"
)

// Unexported message types for JSON marshaling/unmarshaling.
type genericMsg struct {
	Type string `json:"type"`
}
type readyMsg struct {
	Type string `json:"type"`
}
type taskMsg struct {
	Type string      `json:"type"`
	Task models.Task `json:"task"`
}
type progressMsg struct {
	Type  string               `json:"type"`
	Event models.ProgressEvent `json:"event"`
}
type statusMsg struct {
	Type   string            `json:"type"`
	TaskID string            `json:"taskId"`
	Status models.TaskStatus `json:"status"`
}
type doneMsg struct {
	Type   string            `json:"type"`
	TaskID string            `json:"taskId"`
	Status models.TaskStatus `json:"status"`
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
func (s *WebSocketConsumer) Connect(ctx context.Context) error {
	go s.reconnectLoop(ctx)
	return nil
}

func (s *WebSocketConsumer) reconnectLoop(ctx context.Context) {
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

		log.Printf("connecting to %s", s.apiURL)
		conn, _, err := websocket.DefaultDialer.Dial(s.apiURL, nil)
		if err != nil {
			log.Printf("dial error: %v", err)
			continue
		}
		log.Println("connected to API")
		attempt = 0

		send := make(chan []byte, 128)
		s.mu.Lock()
		s.currentSend = send
		s.mu.Unlock()

		if err := s.runConn(ctx, conn, send); err != nil {
			log.Printf("connection error: %v", err)
		}

		s.mu.Lock()
		s.currentSend = nil
		s.mu.Unlock()
	}
}

// Receive blocks until a task is available or ctx is cancelled.
func (s *WebSocketConsumer) Receive(ctx context.Context) (models.Task, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, ctx.Err()
	case task := <-s.tasks:
		return task, nil
	}
}

// ReportResult sends a task status event to the API over WebSocket.
// Terminal statuses use DoneMsg; non-terminal statuses use StatusMsg.
func (s *WebSocketConsumer) ReportResult(_ context.Context, taskID string, status models.TaskStatus) error {
	var msg any
	if status == models.TaskCompleted || status == models.TaskFailed {
		msg = doneMsg{Type: msgTypeDone, TaskID: taskID, Status: status}
	} else {
		msg = statusMsg{Type: msgTypeStatus, TaskID: taskID, Status: status}
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.mu.Lock()
	send := s.currentSend
	s.mu.Unlock()
	if send == nil {
		return nil
	}
	select {
	case send <- data:
	default:
	}
	return nil
}

// ReportProgress sends a stage progress event to the API over WebSocket.
// Events are best-effort and may be dropped if the send buffer is full.
func (s *WebSocketConsumer) ReportProgress(_ context.Context, event models.ProgressEvent) error {
	data, err := json.Marshal(progressMsg{Type: msgTypeProgress, Event: event})
	if err != nil {
		return err
	}
	s.mu.Lock()
	send := s.currentSend
	s.mu.Unlock()
	if send == nil {
		return nil
	}
	select {
	case send <- data:
	default:
	}
	return nil
}

// runConn manages one connection lifecycle: write pump + read loop.
func (s *WebSocketConsumer) runConn(ctx context.Context, conn *websocket.Conn, send chan []byte) error {
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

	ready, _ := json.Marshal(readyMsg{Type: msgTypeReady})
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

		var gm genericMsg
		if err := json.Unmarshal(raw, &gm); err != nil {
			continue
		}

		if gm.Type == msgTypeTask {
			var msg taskMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				log.Printf("unmarshal task: %v", err)
				continue
			}
			log.Printf("received task %s (%d stages)", msg.Task.ID, len(msg.Task.Stages))
			select {
			case s.tasks <- msg.Task:
			case <-ctx.Done():
				close(send)
				<-done
				_ = conn.Close()
				return nil
			}
		}
	}
}
