package p02_test

import (
	"context"
	"net"
	"testing"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/patterns/p02/internal/app"
	"work-distribution-patterns/shared/testutil"
)

func TestP2Integration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start manager.
	mgrE, err := app.NewManager(ctx, app.ManagerConfig{WorkersQueueSize: 10})
	if err != nil {
		t.Fatalf("manager setup: %v", err)
	}
	mgrURL := startServer(t, mgrE)
	testutil.WaitReady(t, mgrURL)

	// 2. Start worker(s) pointing at the manager.
	go app.RunWorker(ctx, app.WorkerConfig{ManagerURL: mgrURL, MaxStageDuration: 100})

	// 3. Start API pointing at the manager.
	apiE, err := app.NewAPI(ctx, app.APIConfig{ManagerURL: mgrURL})
	if err != nil {
		t.Fatalf("api setup: %v", err)
	}
	apiURL := startServer(t, apiE)
	testutil.WaitReady(t, apiURL)

	testutil.RunSuite(t, apiURL)
}

// startServer starts e on a random port and registers cleanup. Returns the base URL.
func startServer(t *testing.T, e *echo.Echo) string {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })
	go func() { _ = e.Server.Serve(ln) }()
	return "http://" + ln.Addr().String()
}
