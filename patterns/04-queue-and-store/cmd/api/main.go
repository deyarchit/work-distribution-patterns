package main

import (
	"context"
	"html/template"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"

	natsinternal "work-distribution-patterns/patterns/04-queue-and-store/internal/nats"
	pgstore "work-distribution-patterns/patterns/04-queue-and-store/internal/postgres"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/sse"
	"work-distribution-patterns/shared/templates"
)

type config struct {
	Addr        string `envconfig:"addr" default:":8080"`
	NATSURL     string `envconfig:"nats_url" default:"nats://127.0.0.1:4222"`
	DatabaseURL string `envconfig:"database_url" default:"postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pool.Close()

	taskStore, err := pgstore.New(ctx, pool)
	if err != nil {
		log.Fatalf("postgres store: %v", err)
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

	hub := sse.NewHub()
	natsBus := natsinternal.NewNATSProducer(nc, js)
	mgr := manager.New(taskStore, natsBus, hub, 30*time.Second)
	mgr.Start(ctx)

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		log.Fatalf("parse template: %v", err)
	}

	e := api.NewRouter(hub, tpl, mgr)
	log.Printf("Pattern 4 (Queue-and-Store) API listening on %s", cfg.Addr)
	log.Fatal(e.Start(cfg.Addr))
}
