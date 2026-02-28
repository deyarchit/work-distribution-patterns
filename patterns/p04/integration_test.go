package p04_test

import (
	"context"
	"net"
	"testing"

	"github.com/labstack/echo/v4"

	"work-distribution-patterns/patterns/p04/internal/app"
	"work-distribution-patterns/shared/testutil"
)

func TestP4Integration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Wire manager components (HTTP router + gRPC server).
	r, err := app.NewManager(ctx, app.ManagerConfig{})
	if err != nil {
		t.Fatalf("manager setup: %v", err)
	}

	// 2. Start gRPC server on a random port.
	grpcLn, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("grpc listen: %v", err)
	}
	grpcAddr := grpcLn.Addr().String()
	t.Cleanup(func() { r.GRPCServer.Stop() })
	go func() { _ = r.GRPCServer.Serve(grpcLn) }() //nolint:errcheck

	// 3. Start manager HTTP server on a random port.
	mgrURL := startServer(t, r.Router)
	testutil.WaitReady(t, mgrURL)

	// 4. Start worker connecting via gRPC.
	go app.RunWorker(ctx, app.WorkerConfig{ManagerGRPCAddr: grpcAddr, MaxStageDuration: 100})

	// 5. Start API connecting via gRPC.
	apiE, err := app.NewAPI(ctx, app.APIConfig{ManagerGRPCAddr: grpcAddr})
	if err != nil {
		t.Fatalf("api setup: %v", err)
	}
	apiURL := startServer(t, apiE)
	testutil.WaitReady(t, apiURL)

	// Wait for at least one worker to register via gRPC.
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
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) }) //nolint:errcheck
	go func() { _ = e.Server.Serve(ln) }()                     //nolint:errcheck
	return "http://" + ln.Addr().String()
}
