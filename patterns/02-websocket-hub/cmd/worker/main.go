package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	wsapi "work-distribution-patterns/patterns/02-websocket-hub/internal/api"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
)

func main() {
	apiURL       := envOr("API_URL", "ws://localhost:8080/ws/register")
	stageDurSecs := envInt("STAGE_DURATION_SECS", 3)

	exec := &executor.Executor{StageDuration: time.Duration(stageDurSecs) * time.Second}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	for attempt := 0; ; attempt++ {
		select {
		case <-quit:
			log.Println("shutting down")
			return
		default:
		}

		if attempt > 0 {
			backoff := time.Duration(math.Min(float64(attempt)*500, 10000)) * time.Millisecond
			log.Printf("reconnecting in %s (attempt %d)...", backoff, attempt)
			time.Sleep(backoff)
		}

		log.Printf("connecting to %s", apiURL)
		conn, _, err := websocket.DefaultDialer.Dial(apiURL, nil)
		if err != nil {
			log.Printf("dial error: %v", err)
			continue
		}
		log.Println("connected to API")

		// Reset backoff on successful connection
		attempt = 0

		if err := runWorker(conn, exec, quit); err != nil {
			log.Printf("worker error: %v", err)
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

func runWorker(conn *websocket.Conn, exec *executor.Executor, quit <-chan os.Signal) error {
	send := make(chan []byte, 128)
	done := make(chan struct{})

	// Write pump — only goroutine allowed to write to conn
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

	// Announce readiness
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
		case <-quit:
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
			assign := msg.Task
			log.Printf("received task %s (%d stages)", assign.TaskID, assign.StageCount)

			stageNames := []string{"Initialization", "Validation", "Processing", "Transformation", "Aggregation", "Optimization", "Finalization", "Cleanup"}
			stages := make([]models.Stage, assign.StageCount)
			for i := range stages {
				stages[i] = models.Stage{Index: i, Name: stageNames[i%len(stageNames)], Status: models.StagePending}
			}
			task := models.Task{
				ID:     assign.TaskID,
				Name:   assign.Name,
				Stages: stages,
			}

			go exec.Run(context.Background(), task, sink)
		}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
