package main

import (
	"context"
	"log/slog"
	"os"

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
		slog.Error("Config error", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	e, err := app.New(ctx, app.Config{
		Workers:          cfg.Workers,
		QueueSize:        cfg.QueueSize,
		MaxStageDuration: cfg.MaxStageDuration,
	})
	if err != nil {
		slog.Error("Setup error", "error", err)
		os.Exit(1)
	}

	slog.Info("Pattern 1 (Goroutine Pool) listening",
		"addr", cfg.Addr,
		"workers", cfg.Workers,
		"queue", cfg.QueueSize,
		"max_stage_ms", cfg.MaxStageDuration)
	if err := e.Start(cfg.Addr); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
