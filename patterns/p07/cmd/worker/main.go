package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p07/internal/app"
	"work-distribution-patterns/patterns/p07/internal/bootstrap"
	"work-distribution-patterns/shared/health"
)

type config struct {
	// Bootstrap discovery — the only broker-related config the worker needs.
	ManagerURL string `envconfig:"manager_url" default:"https://localhost:8083"`

	// mTLS identity — the device's unique certificate.
	CertPath string `envconfig:"cert_path" default:"certs/device.crt"`
	KeyPath  string `envconfig:"key_path" default:"certs/device.key"`
	CAPath   string `envconfig:"ca_path" default:"certs/ca.crt"`

	MaxStageDuration int    `envconfig:"max_stage_duration" default:"500"` // ms
	HealthAddr       string `envconfig:"health_addr" default:":8082"`
}

func main() {
	if err := run(); err != nil {
		slog.Error("Fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	tlsCfg, err := bootstrap.LoadClientTLS(cfg.CertPath, cfg.KeyPath, cfg.CAPath)
	if err != nil {
		return fmt.Errorf("TLS config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("Pattern 07 Worker starting", "bootstrap_url", cfg.ManagerURL)
	health.StartServer(ctx, cfg.HealthAddr)

	app.RunWorker(ctx, app.WorkerConfig{
		BootstrapURL:     cfg.ManagerURL,
		TLSConfig:        tlsCfg,
		MaxStageDuration: cfg.MaxStageDuration,
	})
	return nil
}
