package wsinternal

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
var _ dispatch.TaskProducer = (*WebSocketProducer)(nil)

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
	hub  *WebSocketProducer
	busy bool
}

// WebSocketProducer manages connected remote workers and implements TaskProducer.
// Workers connect via HTTP GET /ws/register; Register is called by that handler.
type WebSocketProducer struct {
	mu       sync.Mutex
	workers  []*workerConn
	nextIdx  int
	results  chan models.TaskStatusEvent
	progress chan models.ProgressEvent
}

// NewWebSocketProducer creates a WebSocketProducer with buffered result and progress channels.
func NewWebSocketProducer() *WebSocketProducer {
	return &WebSocketProducer{
		results:  make(chan models.TaskStatusEvent, 256),
		progress: make(chan models.ProgressEvent, 256),
	}
}

// Start is a no-op; subscriptions are established per-connection via Register.
func (p *WebSocketProducer) Start(_ context.Context) error { return nil }

// Register adds a new worker connection and starts its read/write pumps.
func (p *WebSocketProducer) Register(conn *websocket.Conn) {
	wc := &workerConn{
		id:   uuid.New().String()[:8],
		conn: conn,
		send: make(chan []byte, 64),
		hub:  p,
	}
	p.mu.Lock()
	p.workers = append(p.workers, wc)
	p.mu.Unlock()
	log.Printf("worker %s connected", wc.id)
	go wc.writePump()
	go wc.readPump()
}

// Dispatch sends the task to an idle worker using round-robin.
// Returns ErrNoWorkers if all workers are busy or none are connected.
func (p *WebSocketProducer) Dispatch(_ context.Context, task models.Task) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(p.workers)
	if n == 0 {
		return dispatch.ErrNoWorkers
	}

	for i := 0; i < n; i++ {
		idx := (p.nextIdx + i) % n
		wc := p.workers[idx]
		if !wc.busy {
			wc.busy = true
			p.nextIdx = (idx + 1) % n

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
func (p *WebSocketProducer) ReceiveResult(ctx context.Context) (models.TaskStatusEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskStatusEvent{}, ctx.Err()
	case ev := <-p.results:
		return ev, nil
	}
}

// ReceiveProgress blocks until a progress event is available or ctx is cancelled.
func (p *WebSocketProducer) ReceiveProgress(ctx context.Context) (models.ProgressEvent, error) {
	select {
	case <-ctx.Done():
		return models.ProgressEvent{}, ctx.Err()
	case ev := <-p.progress:
		return ev, nil
	}
}

func (p *WebSocketProducer) remove(wc *workerConn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, w := range p.workers {
		if w == wc {
			p.workers = append(p.workers[:i], p.workers[i+1:]...)
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
// Progress events are forwarded to the hub's progress channel.
// Status and done events are forwarded to the hub's results channel.
// DoneMsg also marks the worker as no longer busy.
func (wc *workerConn) readPump() {
	defer func() {
		wc.hub.remove(wc)
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
				case wc.hub.progress <- msg.Event:
				default:
				}
			}

		case msgTypeStatus:
			var msg statusMsg
			if err := json.Unmarshal(raw, &msg); err == nil {
				wc.hub.results <- models.TaskStatusEvent{TaskID: msg.TaskID, Status: msg.Status}
			}

		case msgTypeDone:
			var msg doneMsg
			if err := json.Unmarshal(raw, &msg); err == nil {
				wc.hub.results <- models.TaskStatusEvent{TaskID: msg.TaskID, Status: msg.Status}
				wc.hub.mu.Lock()
				wc.busy = false
				wc.hub.mu.Unlock()
			}

		case msgTypeReady:
			log.Printf("worker %s ready", wc.id)
		}
	}
}
