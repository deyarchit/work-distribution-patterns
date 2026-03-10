package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// HealthResponse is the standard string returned by all health check endpoints.
const HealthResponse = "ok"

// Handler returns a standard net/http handler for health checks.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(HealthResponse))
	}
}

// StartServer starts a minimal HTTP server in the background to handle health checks.
// It listens on the provided addr and shuts down gracefully when the context is canceled.
func StartServer(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", Handler())

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		slog.Info("Health check server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Health check server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("Health check server shutdown error", "error", err)
		}
	}()
}
