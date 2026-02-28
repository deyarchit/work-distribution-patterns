package main

import (
	"context"
	"log"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p01/internal/app"
)

type config struct {
	Addr             string `envconfig:"addr" default:":8080"`
	Workers          int    `envconfig:"workers" default:"5"`
	QueueSize        int    `envconfig:"queue_size" default:"20"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	e, err := app.New(ctx, app.Config{
		Workers:          cfg.Workers,
		QueueSize:        cfg.QueueSize,
		MaxStageDuration: cfg.MaxStageDuration,
	})
	if err != nil {
		log.Fatalf("setup: %v", err)
	}

	log.Printf("Pattern 1 (Goroutine Pool) listening on %s [workers=%d, queue=%d, maxStage=%dms]",
		cfg.Addr, cfg.Workers, cfg.QueueSize, cfg.MaxStageDuration)
	log.Fatal(e.Start(cfg.Addr))
}
