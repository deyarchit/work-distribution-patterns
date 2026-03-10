package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p04/internal/app"
)

type config struct {
	HTTPAddr string `envconfig:"http_addr" default:":8081"`
	GRPCAddr string `envconfig:"grpc_addr" default:":9091"`
}

func main() {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		slog.Error("Config error", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	r, err := app.NewManager(ctx, app.ManagerConfig{})
	if err != nil {
		slog.Error("Setup error", "error", err)
		os.Exit(1)
	}

	grpcLn, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		slog.Error("gRPC listen error", "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("gRPC server listening", "addr", cfg.GRPCAddr)
		if err := r.GRPCServer.Serve(grpcLn); err != nil {
			slog.Error("gRPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("Pattern 4 (gRPC Streaming) Manager listening", "http_addr", cfg.HTTPAddr, "grpc_addr", cfg.GRPCAddr)
	if err := r.Router.Start(cfg.HTTPAddr); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
