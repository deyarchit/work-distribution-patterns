package p03_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/patterns/p03/internal/app"
	"work-distribution-patterns/shared/testutil"
)

func TestP3Integration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start manager.
	mgrE, err := app.NewManager(ctx, app.ManagerConfig{WorkersQueueSize: 10})
	if err != nil {
		t.Fatalf("manager setup: %v", err)
	}
	mgrURL := startServer(t, mgrE)
	testutil.WaitReady(t, mgrURL)

	// 2. Derive the WebSocket registration URL from the manager HTTP URL.
	mgrWSURL := "ws://" + strings.TrimPrefix(mgrURL, "http://") + "/ws/register"

	// 3. Start 3 workers (matching docker-compose scale=3). P3 uses one task per worker at a time,
	// so concurrent-task tests require at least 3 idle workers.
	for range 3 {
		go app.RunWorker(ctx, app.WorkerConfig{ManagerWSURL: mgrWSURL, MaxStageDuration: 100})
	}

	// 4. Start API pointing at the manager.
	apiE, err := app.NewAPI(ctx, app.APIConfig{ManagerURL: mgrURL})
	if err != nil {
		t.Fatalf("api setup: %v", err)
	}
	apiURL := startServer(t, apiE)
	testutil.WaitReady(t, apiURL)

	// Wait for at least one worker to register via WebSocket.
	testutil.WaitForWorker(t, apiURL)

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
