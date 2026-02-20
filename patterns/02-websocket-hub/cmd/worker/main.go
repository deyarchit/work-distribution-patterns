package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/02-websocket-hub/internal/worker"
	"work-distribution-patterns/shared/dispatch"
	"work-distribution-patterns/shared/executor"
	"work-distribution-patterns/shared/models"
)

type config struct {
	APIURL           string `envconfig:"api_url" default:"ws://localhost:8080/ws/register"`
	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"`
}

// progressSink adapts WebSocketSource into dispatch.ProgressSink for the executor.
type progressSink struct {
	ctx    context.Context
	source dispatch.WorkerSource
}

func (s *progressSink) Publish(event models.ProgressEvent) {
	_ = s.source.ReportProgress(s.ctx, event)
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	source := worker.NewWebSocketSource(cfg.APIURL)
	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}

	_ = source.Connect(ctx)

	for {
		task, err := source.Receive(ctx)
		if err != nil {
			log.Printf("worker stopped: %v", err)
			return
		}
		go func() {
			_ = source.ReportResult(ctx, task.ID, models.TaskRunning)
			status := exec.Run(ctx, task, &progressSink{ctx: ctx, source: source})
			_ = source.ReportResult(ctx, task.ID, status)
		}()
	}
}
