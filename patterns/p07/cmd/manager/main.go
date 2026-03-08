package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	"work-distribution-patterns/patterns/p07/internal/app"
	"work-distribution-patterns/patterns/p07/internal/bootstrap"
)

type config struct {
	// Task API (plain HTTP, used by API processes).
	Addr        string `envconfig:"addr" default:":8081"`
	BrokerURL   string `envconfig:"broker_url" default:"nats://localhost:4222"`
	DatabaseURL string `envconfig:"database_url" default:"postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable"`

	// Bootstrap server (mTLS, used by edge workers).
	BootstrapAddr  string `envconfig:"bootstrap_addr" default:":8083"`
	TokenSecret    string `envconfig:"token_secret" default:"change-me-in-production"`
	TokenTTLMins   int    `envconfig:"token_ttl_mins" default:"15"`
	ServerCertPath string `envconfig:"server_cert_path" default:"certs/server.crt"`
	ServerKeyPath  string `envconfig:"server_key_path" default:"certs/server.key"`
	CACertPath     string `envconfig:"ca_cert_path" default:"certs/ca.crt"`
	RevokedCNs     string `envconfig:"revoked_cns" default:""` // comma-separated
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverTLS, err := bootstrap.LoadServerTLS(cfg.ServerCertPath, cfg.ServerKeyPath, cfg.CACertPath)
	if err != nil {
		return fmt.Errorf("TLS config: %w", err)
	}

	var revokedCNs []string
	if cfg.RevokedCNs != "" {
		revokedCNs = strings.Split(cfg.RevokedCNs, ",")
	}

	comps, err := app.NewManager(ctx, app.ManagerConfig{
		BrokerURL:       cfg.BrokerURL,
		DatabaseURL:     cfg.DatabaseURL,
		BootstrapAddr:   cfg.BootstrapAddr,
		TokenSecret:     cfg.TokenSecret,
		TokenTTL:        time.Duration(cfg.TokenTTLMins) * time.Minute,
		ServerTLSConfig: serverTLS,
		RevokedCNs:      revokedCNs,
	})
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	errCh := make(chan error, 2)

	// Plain-HTTP task API for internal API↔Manager communication.
	go func() {
		log.Printf("Pattern 07 Manager (task API) listening on %s [broker=%s]", cfg.Addr, cfg.BrokerURL)
		errCh <- comps.Router.Start(cfg.Addr)
	}()

	// mTLS bootstrap server for edge worker credential vending.
	// The listener was created (and ClientAuth enforced) inside NewManager.
	go func() {
		log.Printf("Pattern 07 Manager (bootstrap) listening on %s [mTLS]", comps.BootstrapListener.Addr())
		errCh <- comps.BootstrapRouter.Server.Serve(comps.BootstrapListener)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		_ = comps.Router.Shutdown(context.Background())
		_ = comps.BootstrapRouter.Shutdown(context.Background())
		return nil
	}
}
