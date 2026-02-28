package p01_test

import (
	"context"
	"net"
	"testing"

	"work-distribution-patterns/patterns/p01/internal/app"
	"work-distribution-patterns/shared/testutil"
)

func TestP1Integration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e, err := app.New(ctx, app.Config{
		Workers:          3,
		QueueSize:        10,
		MaxStageDuration: 100,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	baseURL := "http://" + ln.Addr().String()
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) }) //nolint:errcheck
	go func() { _ = e.Server.Serve(ln) }()                     //nolint:errcheck

	testutil.WaitReady(t, baseURL)
	testutil.RunSuite(t, baseURL)
}
