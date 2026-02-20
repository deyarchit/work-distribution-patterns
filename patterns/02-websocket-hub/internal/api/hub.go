package api

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
)

// ErrNoWorkersAvailable is returned when no idle worker is available.
var ErrNoWorkersAvailable = errors.New("no workers available")

// WorkerConn represents a connected worker.
type WorkerConn struct {
	id   string
	conn *websocket.Conn
	send chan []byte
	hub  *WorkerHub
	busy bool
}

// WorkerHub manages connected workers and dispatches tasks to them.
// It is the task manager's receive side: progress events and terminal status
// reported by workers over WebSocket are routed to the SSE hub and task store here.
type WorkerHub struct {
	mu        sync.Mutex
	workers   []*WorkerConn
	sseHub    *sse.Hub
	taskStore store.TaskStore
	nextIdx   int
}

// NewWorkerHub creates a WorkerHub that routes progress events to the SSE hub
// and persists final task status to taskStore.
func NewWorkerHub(sseHub *sse.Hub, taskStore store.TaskStore) *WorkerHub {
	return &WorkerHub{sseHub: sseHub, taskStore: taskStore}
}

// Register adds a new worker connection and starts its read/write pumps.
// A unique worker ID is generated internally.
func (h *WorkerHub) Register(conn *websocket.Conn) *WorkerConn {
	wc := &WorkerConn{
		id:   uuid.New().String()[:8],
		conn: conn,
		send: make(chan []byte, 64),
		hub:  h,
	}
	h.mu.Lock()
	h.workers = append(h.workers, wc)
	h.mu.Unlock()

	log.Printf("worker %s connected", wc.id)
	go wc.writePump()
	go wc.readPump()
	return wc
}

// Assign sends the full task to an available (non-busy) worker using round-robin.
// Returns ErrNoWorkersAvailable if all workers are busy or none are connected.
func (h *WorkerHub) Assign(task models.Task) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	n := len(h.workers)
	if n == 0 {
		return ErrNoWorkersAvailable
	}

	// Round-robin over workers, skipping busy ones
	for i := 0; i < n; i++ {
		idx := (h.nextIdx + i) % n
		wc := h.workers[idx]
		if !wc.busy {
			wc.busy = true
			h.nextIdx = (idx + 1) % n

			msg := TaskMsg{
				Type: MsgTypeTask,
				Task: task,
			}
			data, err := json.Marshal(msg)
			if err != nil {
				wc.busy = false
				return err
			}
			select {
			case wc.send <- data:
				return nil
			default:
				wc.busy = false
				return ErrNoWorkersAvailable
			}
		}
	}
	return ErrNoWorkersAvailable
}

func (h *WorkerHub) remove(wc *WorkerConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, w := range h.workers {
		if w == wc {
			h.workers = append(h.workers[:i], h.workers[i+1:]...)
			break
		}
	}
}

// writePump is the ONLY goroutine that writes to the WebSocket connection.
func (wc *WorkerConn) writePump() {
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

// readPump processes messages from the worker. This is the task manager's receive side:
// progress events are forwarded to the SSE hub; terminal status is persisted to the store.
func (wc *WorkerConn) readPump() {
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

		var generic GenericMsg
		if err := json.Unmarshal(raw, &generic); err != nil {
			continue
		}

		switch generic.Type {
		case MsgTypeProgress:
			var msg ProgressMsg
			if err := json.Unmarshal(raw, &msg); err == nil {
				wc.hub.sseHub.Publish(msg.Event)
			}

		case MsgTypeDone:
			var msg DoneMsg
			if err := json.Unmarshal(raw, &msg); err == nil {
				wc.hub.sseHub.PublishTaskStatus(msg.TaskID, models.TaskCompleted)
				_ = wc.hub.taskStore.SetStatus(msg.TaskID, models.TaskCompleted)
				wc.hub.mu.Lock()
				wc.busy = false
				wc.hub.mu.Unlock()
			}

		case MsgTypeReady:
			log.Printf("worker %s ready", wc.id)
		}
	}
}
