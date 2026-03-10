package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p04/internal/app"
)

type config struct {
	Addr            string `envconfig:"addr" default:":8080"`
	ManagerGRPCAddr string `envconfig:"manager_grpc_addr" default:"localhost:9091"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		slog.Error("Config error", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	e, err := app.NewAPI(ctx, app.APIConfig{ManagerGRPCAddr: cfg.ManagerGRPCAddr})
	if err != nil {
		slog.Error("Setup error", "error", err)
		os.Exit(1)
	}

	slog.Info("Pattern 4 (gRPC Streaming) API listening", "addr", cfg.Addr, "manager", cfg.ManagerGRPCAddr)
	if err := e.Start(cfg.Addr); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
