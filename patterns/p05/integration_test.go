package p05_test

import (
	"context"
	"net"
	"testing"

	"github.com/labstack/echo/v4"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"work-distribution-patterns/patterns/p05/internal/app"
	"work-distribution-patterns/shared/testutil"
)

func TestP5Integration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start NATS container (JetStream is enabled by default in the testcontainers NATS module).
	natsC, err := tcnats.Run(ctx, "nats:2-alpine")
	if err != nil {
		t.Fatalf("nats container: %v", err)
	}
	t.Cleanup(func() { _ = natsC.Terminate(context.Background()) })
	natsURL, err := natsC.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("nats url: %v", err)
	}

	// 2. Start Postgres container.
	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("tasks"),
		tcpostgres.WithUsername("tasks"),
		tcpostgres.WithPassword("tasks"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(context.Background()) })
	dbURL, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres url: %v", err)
	}

	// 3. Start manager.
	mgrE, err := app.NewManager(ctx, app.ManagerConfig{
		NATSURL:     natsURL,
		DatabaseURL: dbURL,
	})
	if err != nil {
		t.Fatalf("manager setup: %v", err)
	}
	mgrURL := startServer(t, mgrE)
	testutil.WaitReady(t, mgrURL)

	// 4. Start worker (synchronous — one task at a time).
	go app.RunWorker(ctx, app.WorkerConfig{NATSURL: natsURL, MaxStageDuration: 100})

	// 5. Start API.
	apiE, err := app.NewAPI(ctx, app.APIConfig{ManagerURL: mgrURL, NATSURL: natsURL})
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
