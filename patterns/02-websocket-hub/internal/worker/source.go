package worker

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"time"

	"github.com/gorilla/websocket"

	wsapi "work-distribution-patterns/patterns/02-websocket-hub/internal/api"
	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
)

// sinkTask pairs an incoming task with the sink for the connection it arrived on.
type sinkTask struct {
	task models.Task
	sink *wsSink
}

// WSTaskSource implements dispatch.TaskSource over a WebSocket connection.
// Call Connect in a goroutine to start the reconnect loop; call Receive to
// pull tasks one at a time. Sink returns the connection-scoped sink for the
// most recently received task — safe to call immediately after Receive in the
// same goroutine.
type WSTaskSource struct {
	apiURL  string
	tasks   chan sinkTask
	current *wsSink
}

// NewWSTaskSource creates a WSTaskSource that connects to the given WebSocket URL.
func NewWSTaskSource(apiURL string) *WSTaskSource {
	return &WSTaskSource{
		apiURL: apiURL,
		tasks:  make(chan sinkTask, 1),
	}
}

// Connect runs the reconnect loop until ctx is cancelled.
// It should be called in a goroutine by the caller.
func (s *WSTaskSource) Connect(ctx context.Context) {
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

		if err := s.runConn(ctx, conn); err != nil {
			log.Printf("connection error: %v", err)
		}
	}
}

// Receive implements dispatch.TaskSource.
// Blocks until a task is available or ctx is cancelled.
func (s *WSTaskSource) Receive(ctx context.Context) (models.Task, error) {
	select {
	case <-ctx.Done():
		return models.Task{}, ctx.Err()
	case st := <-s.tasks:
		s.current = st.sink
		return st.task, nil
	}
}

// Sink returns the connection-scoped ProgressSink for the most recently
// received task. Safe to call immediately after Receive in the same goroutine.
func (s *WSTaskSource) Sink() executor.ProgressSink {
	return s.current
}

// runConn handles one connection lifecycle: write pump, read loop, task dispatch.
func (s *WSTaskSource) runConn(ctx context.Context, conn *websocket.Conn) error {
	send := make(chan []byte, 128)
	done := make(chan struct{})

	// Write pump — only goroutine allowed to write to conn.
	go func() {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case msg, ok := <-send:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if !ok {
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					log.Printf("write error: %v", err)
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	ready, _ := json.Marshal(wsapi.ReadyMsg{Type: wsapi.MsgTypeReady})
	send <- ready

	sink := &wsSink{send: send}

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			close(send)
			<-done
			conn.Close()
			return nil
		case <-done:
			conn.Close()
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
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var generic wsapi.GenericMsg
		if err := json.Unmarshal(raw, &generic); err != nil {
			continue
		}

		if generic.Type == wsapi.MsgTypeTask {
			var msg wsapi.TaskMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				log.Printf("unmarshal task: %v", err)
				continue
			}
			log.Printf("received task %s (%d stages)", msg.Task.ID, len(msg.Task.Stages))
			select {
			case s.tasks <- sinkTask{task: msg.Task, sink: sink}:
			case <-ctx.Done():
				close(send)
				<-done
				conn.Close()
				return nil
			}
		}
	}
}

// wsSink sends progress events back to the API over WebSocket.
type wsSink struct {
	send chan []byte
}

func (s *wsSink) Publish(event models.ProgressEvent) {
	msg := wsapi.ProgressMsg{
		Type:  wsapi.MsgTypeProgress,
		Event: event,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case s.send <- data:
	default:
	}
}

func (s *wsSink) PublishTaskStatus(taskID string, status models.TaskStatus) {
	if status == models.TaskCompleted || status == models.TaskFailed {
		msg := wsapi.DoneMsg{
			Type:   wsapi.MsgTypeDone,
			TaskID: taskID,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return
		}
		select {
		case s.send <- data:
		default:
		}
	}
}

// Compile-time interface checks.
var _ executor.ProgressSink = (*wsSink)(nil)
var _ dispatch.TaskSource = (*WSTaskSource)(nil)
