package main

import (
	"context"
	"html/template"
	"log"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"

	natsinternal "work-distribution-patterns/patterns/03-nats-jetstream/internal/nats"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
)

type config struct {
	Addr    string `envconfig:"addr" default:":8080"`
	NATSURL string `envconfig:"nats_url" default:"nats://127.0.0.1:4222"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	nc, err := nats.Connect(cfg.NATSURL,
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
	)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	if err := natsinternal.SetupJetStream(js); err != nil {
		log.Printf("setup warning: %v", err)
	}

	kv, err := js.KeyValue(natsinternal.KVBucket)
	if err != nil {
		log.Fatalf("open KV: %v", err)
	}

	hub := sse.NewHub()
	taskStore := natsinternal.NewJetStreamStore(kv)
	natsBus := natsinternal.NewNATSProducer(nc, js)
	mgr := manager.New(taskStore, natsBus, hub, 30*time.Second)
	mgr.Start(context.Background())

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(taskStore, hub, tpl, mgr)
	log.Printf("Pattern 3 (NATS JetStream) API listening on %s", cfg.Addr)
	log.Fatal(e.Start(cfg.Addr))
}
