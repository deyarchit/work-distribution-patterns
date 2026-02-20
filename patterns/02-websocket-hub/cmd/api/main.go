package main

import (
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	wsapi "work-distribution-patterns/patterns/02-websocket-hub/internal/api"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
	"work-distribution-patterns/shared/templates"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
	addr := envOr("ADDR", ":8080")

	sseHub := sse.NewHub()
	taskStore := store.NewMemoryStore()
	workerHub := wsapi.NewWorkerHub(sseHub, taskStore)
	manager := wsapi.NewWSTaskManager(workerHub, taskStore)

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(taskStore, sseHub, tpl, manager)

	// Register worker WebSocket endpoint
	e.GET("/ws/register", func(c echo.Context) error {
		conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		workerHub.Register(conn)
		return nil
	})

	log.Printf("Pattern 2 (WebSocket Hub) API listening on %s", addr)
	log.Fatal(e.Start(addr))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
