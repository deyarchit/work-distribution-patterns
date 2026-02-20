package main

import (
	"context"
	"html/template"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/kelseyhightower/envconfig"
	"github.com/labstack/echo/v4"

	wsinternal "work-distribution-patterns/patterns/02-websocket-hub/internal/websocket"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/store"
	"work-distribution-patterns/shared/templates"
)

type config struct {
	Addr string `envconfig:"addr" default:":8080"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	sseHub := sse.NewHub()
	taskStore := store.NewMemoryStore()
	workerBus := wsinternal.NewWebSocketProducer()
	mgr := manager.New(taskStore, workerBus, sseHub, 0) // deadline=0; workers always connected
	mgr.Start(context.Background())

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(taskStore, sseHub, tpl, mgr)

	// Worker WebSocket registration endpoint.
	e.GET("/ws/register", func(c echo.Context) error {
		conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		workerBus.Register(conn)
		return nil
	})

	log.Printf("Pattern 2 (WebSocket Hub) API listening on %s", cfg.Addr)
	log.Fatal(e.Start(cfg.Addr))
}
