package main

import (
	"context"
	"log"
	"net"

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
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	r, err := app.NewManager(ctx, app.ManagerConfig{})
	if err != nil {
		log.Fatalf("setup: %v", err)
	}

	grpcLn, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}

	go func() {
		log.Printf("gRPC server listening on %s", cfg.GRPCAddr)
		if err := r.GRPCServer.Serve(grpcLn); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	log.Printf("Pattern 4 (gRPC Streaming) Manager HTTP on %s, gRPC on %s", cfg.HTTPAddr, cfg.GRPCAddr)
	log.Fatal(r.Router.Start(cfg.HTTPAddr))
}
