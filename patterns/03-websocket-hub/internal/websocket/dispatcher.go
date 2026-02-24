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
var _ dispatch.TaskDispatcher = (*WebSocketDispatcher)(nil)

// msgTypeTask is sent from the API to a worker carrying the task to execute.
const msgTypeTask = "task"

// taskMsg wraps a Task for delivery to a worker over WebSocket.
type taskMsg struct {
	Type string      `json:"type"`
	Task models.Task `json:"task"`
}

// workerConn represents a connected remote worker.
type workerConn struct {
	id   string
	conn *websocket.Conn
	send chan []byte
	hub  *WebSocketDispatcher
	busy bool
}

// WebSocketDispatcher manages connected remote workers and implements TaskDispatcher.
// Workers connect via HTTP GET /ws/register; Register is called by that handler.
type WebSocketDispatcher struct {
	mu      sync.Mutex
	workers []*workerConn
	nextIdx int
	events  chan models.TaskEvent
}

// NewWebSocketDispatcher creates a WebSocketDispatcher with a buffered event channel.
func NewWebSocketDispatcher() *WebSocketDispatcher {
	return &WebSocketDispatcher{
		events: make(chan models.TaskEvent, 256),
	}
}

// Start is a no-op; subscriptions are established per-connection via Register.
func (p *WebSocketDispatcher) Start(_ context.Context) error { return nil }

// Register adds a new worker connection and starts its read/write pumps.
func (p *WebSocketDispatcher) Register(conn *websocket.Conn) {
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
func (p *WebSocketDispatcher) Dispatch(_ context.Context, task models.Task) error {
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

// ReceiveEvent blocks until an event is available or ctx is cancelled.
func (p *WebSocketDispatcher) ReceiveEvent(ctx context.Context) (models.TaskEvent, error) {
	select {
	case <-ctx.Done():
		return models.TaskEvent{}, ctx.Err()
	case ev := <-p.events:
		return ev, nil
	}
}

func (p *WebSocketDispatcher) remove(wc *workerConn) {
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

// readPump processes TaskEvent messages from the worker.
// Terminal task_status events mark the worker as idle and use a blocking send.
// All other events are forwarded best-effort.
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

		var ev models.TaskEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			continue
		}

		switch ev.Type {
		case models.EventTaskStatus:
			isTerminal := ev.Status == string(models.TaskCompleted) || ev.Status == string(models.TaskFailed)
			if isTerminal {
				wc.hub.events <- ev // blocking: must not lose terminal status
				wc.hub.mu.Lock()
				wc.busy = false
				wc.hub.mu.Unlock()
			} else {
				select {
				case wc.hub.events <- ev:
				default:
				}
			}
		case models.EventProgress:
			select {
			case wc.hub.events <- ev:
			default:
			}
		}
		// Unknown types (e.g., "ready") are silently ignored.
	}
}
