package health

import (
	"context"
	"log"
	"net/http"
	"time"
)

// HealthResponse is the standard string returned by all health check endpoints.
const HealthResponse = "ok"

// Handler returns a standard net/http handler for health checks.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(HealthResponse)) //nolint:errcheck
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
		log.Printf("Health check server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Health check server error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Health check server shutdown error: %v", err)
		}
	}()
}
