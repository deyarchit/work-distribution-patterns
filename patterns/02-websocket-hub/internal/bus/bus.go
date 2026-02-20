package bus

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	dispatch "work-distribution-patterns/shared/contracts"
	"work-distribution-patterns/shared/models"
)

// Compile-time interface check.
var _ dispatch.WorkerBus = (*WebSocketBus)(nil)

// Message type constants for the WebSocket protocol.
const (
	msgTypeReady    = "ready"
	msgTypeTask     = "task"
	msgTypeProgress = "progress"
	msgTypeStatus   = "status" // non-terminal task status
	msgTypeDone     = "done"   // terminal task status
)

// Unexported message types — only used inside this package.
type genericMsg struct {
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

// workerConn represents a connected remote worker.
type workerConn struct {
	id   string
	conn *websocket.Conn
	send chan []byte
	bus  *WebSocketBus
	busy bool
}

// WebSocketBus manages connected remote workers and implements WorkerBus.
// Workers connect via HTTP GET /ws/register; Register is called by that handler.
type WebSocketBus struct {
	mu       sync.Mutex
	workers  []*workerConn
	nextIdx  int
	results  chan models.TaskStatusEvent
	progress chan models.ProgressEvent
}

// NewWebSocketBus creates a WebSocketBus with buffered result and progress channels.
func NewWebSocketBus() *WebSocketBus {
	return &WebSocketBus{
		results:  make(chan models.TaskStatusEvent, 256),
		progress: make(chan models.ProgressEvent, 256),
	}
}

// Start is a no-op; subscriptions are established per-connection via Register.
func (b *WebSocketBus) Start(_ context.Context) error { return nil }

// Register adds a new worker connection and starts its read/write pumps.
func (b *WebSocketBus) Register(conn *websocket.Conn) {
	wc := &workerConn{
		id:   uuid.New().String()[:8],
		conn: conn,
		send: make(chan []byte, 64),
		bus:  b,
	}
	b.mu.Lock()
	b.workers = append(b.workers, wc)
	b.mu.Unlock()
	log.Printf("worker %s connected", wc.id)
	go wc.writePump()
	go wc.readPump()
}

// Dispatch sends the task to an idle worker using round-robin.
// Returns ErrNoWorkers if all workers are busy or none are connected.
func (b *WebSocketBus) Dispatch(_ context.Context, task models.Task) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(b.workers)
	if n == 0 {
		return dispatch.ErrNoWorkers
	}

	for i := 0; i < n; i++ {
		idx := (b.nextIdx + i) % n
		wc := b.workers[idx]
		if !wc.busy {
			wc.busy = true
			b.nextIdx = (idx + 1) % n

			data, err := json.Marshal(taskMsg{Type: msgTypeTask, Task: task})
			if err != nil {
				wc.busy = false
				return err
			}
			select {
			case wc.send <- data:
				return nil
			default:
				wc.busy = false
				return dispatch.ErrNoWorkers
			}
		}
	}
	return dispatch.ErrNoWorkers
}

// ReceiveResult blocks until a task status event is available or ctx is cancelled.
func (b *WebSocketBus) ReceiveResult(ctx context.Context) (models.TaskStatusEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskStatusEvent{}, ctx.Err()
	case ev := <-b.results:
		return ev, nil
	}
}

// ReceiveProgress blocks until a progress event is available or ctx is cancelled.
func (b *WebSocketBus) ReceiveProgress(ctx context.Context) (models.ProgressEvent, error) {
	select {
	case <-ctx.Done():
		return models.ProgressEvent{}, ctx.Err()
	case ev := <-b.progress:
		return ev, nil
	}
}

func (b *WebSocketBus) remove(wc *workerConn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, w := range b.workers {
		if w == wc {
			b.workers = append(b.workers[:i], b.workers[i+1:]...)
			break
		}
	}
}

// writePump is the only goroutine that writes to the WebSocket connection.
func (wc *workerConn) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		_ = wc.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-wc.send:
			_ = wc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = wc.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := wc.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = wc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := wc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump processes messages from the worker.
// Progress events are forwarded to the bus's progress channel.
// Status and done events are forwarded to the bus's results channel.
// DoneMsg also marks the worker as no longer busy.
func (wc *workerConn) readPump() {
	defer func() {
		wc.bus.remove(wc)
		close(wc.send)
		_ = wc.conn.Close()
		log.Printf("worker %s disconnected", wc.id)
	}()

	wc.conn.SetReadLimit(64 * 1024)
	_ = wc.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	wc.conn.SetPongHandler(func(string) error {
		_ = wc.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, raw, err := wc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("worker %s read error: %v", wc.id, err)
			}
			return
		}
		_ = wc.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var gm genericMsg
		if err := json.Unmarshal(raw, &gm); err != nil {
			continue
		}

		switch gm.Type {
		case msgTypeProgress:
			var msg progressMsg
			if err := json.Unmarshal(raw, &msg); err == nil {
				select {
				case wc.bus.progress <- msg.Event:
				default:
				}
			}

		case msgTypeStatus:
			var msg statusMsg
			if err := json.Unmarshal(raw, &msg); err == nil {
				wc.bus.results <- models.TaskStatusEvent{TaskID: msg.TaskID, Status: msg.Status}
			}

		case msgTypeDone:
			var msg doneMsg
			if err := json.Unmarshal(raw, &msg); err == nil {
				wc.bus.results <- models.TaskStatusEvent{TaskID: msg.TaskID, Status: msg.Status}
				wc.bus.mu.Lock()
				wc.busy = false
				wc.bus.mu.Unlock()
			}

		case msgTypeReady:
			log.Printf("worker %s ready", wc.id)
		}
	}
}
