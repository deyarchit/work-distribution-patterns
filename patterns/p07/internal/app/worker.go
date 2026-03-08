package app

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"work-distribution-patterns/patterns/p07/internal/bootstrap"
	pubsubinternal "work-distribution-patterns/patterns/p07/internal/pubsub"
	"work-distribution-patterns/shared/executor"
)

// WorkerConfig holds runtime parameters for the Pattern 7 worker process.
// The worker requires only BootstrapURL and TLSConfig — no broker configuration.
// All broker details are discovered at runtime via the bootstrap handshake.
type WorkerConfig struct {
	BootstrapURL     string        // https://<manager-bootstrap-addr>
	TLSConfig        *tls.Config   // mTLS client config (device cert + CA)
	MaxStageDuration int           // milliseconds per stage
	TokenTTL         time.Duration // informational; actual TTL comes from server response
}

// RunWorker performs the bootstrap handshake and enters the task processing loop.
// It re-bootstraps automatically before the token expires, requiring no manual
// intervention or restarts when the manager rotates credentials.
// Blocks until ctx is cancelled.
func RunWorker(ctx context.Context, cfg WorkerConfig) {
	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: cfg.TLSConfig},
		Timeout:   10 * time.Second,
	}

	// Initial bootstrap: discover broker URL and receive a short-lived token.
	resp, err := doBootstrapWithRetry(ctx, httpClient, cfg.BootstrapURL)
	if err != nil {
		log.Printf("p07 worker: initial bootstrap failed: %v", err)
		return
	}
	log.Printf("p07 worker: bootstrapped [broker=%s expires=%s]", resp.BrokerURL, resp.ExpiresAt.Format(time.RFC3339))

	// Open broker resources using the discovered URL.
	tasksSub, eventsTopic, err := pubsubinternal.OpenWorkerResources(ctx, resp.BrokerURL)
	if err != nil {
		log.Printf("p07 worker: pubsub setup: %v", err)
		return
	}

	consumer := pubsubinternal.NewPubSubConsumer(tasksSub, eventsTopic)
	defer consumer.Shutdown(ctx)

	if err := consumer.Connect(ctx); err != nil {
		log.Printf("p07 worker: connect: %v", err)
		return
	}

	// Re-bootstrap goroutine: refresh the token at 80% of its remaining lifetime.
	// If the BrokerURL changes on renewal, the worker logs the change — in a full
	// production setup, the consumer would reconnect to the new broker.
	go func() {
		current := resp
		for {
			renewIn := max(time.Duration(float64(time.Until(current.ExpiresAt))*0.8), time.Second)

			select {
			case <-ctx.Done():
				return
			case <-time.After(renewIn):
			}

			newResp, err := doBootstrapWithRetry(ctx, httpClient, cfg.BootstrapURL)
			if err != nil {
				log.Printf("p07 worker: re-bootstrap failed (will retry next cycle): %v", err)
				continue
			}

			if newResp.BrokerURL != current.BrokerURL {
				log.Printf("p07 worker: broker URL changed [old=%s new=%s] — reconnect required",
					current.BrokerURL, newResp.BrokerURL)
			} else {
				log.Printf("p07 worker: token renewed [expires=%s]", newResp.ExpiresAt.Format(time.RFC3339))
			}

			current = newResp
		}
	}()

	exec := &executor.Executor{MaxStageDuration: time.Duration(cfg.MaxStageDuration) * time.Millisecond}
	for {
		task, err := consumer.Receive(ctx)
		if err != nil {
			return
		}
		exec.Run(ctx, task, consumer)
	}
}

// doBootstrapWithRetry calls GET /worker/bootstrap, retrying up to 5 times with
// exponential backoff. Retries handle the window between the test starting the
// worker goroutine and the bootstrap server becoming ready.
func doBootstrapWithRetry(ctx context.Context, client *http.Client, bootstrapURL string) (bootstrap.BootstrapResponse, error) {
	var lastErr error
	for attempt := range 5 {
		select {
		case <-ctx.Done():
			return bootstrap.BootstrapResponse{}, ctx.Err()
		default:
		}

		if attempt > 0 {
			backoff := time.Duration(attempt) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return bootstrap.BootstrapResponse{}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := doBootstrapOnce(ctx, client, bootstrapURL)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		log.Printf("p07 worker: bootstrap attempt %d/5 failed: %v", attempt+1, err)
	}
	return bootstrap.BootstrapResponse{}, fmt.Errorf("bootstrap failed after 5 attempts: %w", lastErr)
}

func doBootstrapOnce(ctx context.Context, client *http.Client, bootstrapURL string) (bootstrap.BootstrapResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bootstrapURL+"/worker/bootstrap", nil)
	if err != nil {
		return bootstrap.BootstrapResponse{}, err
	}

	res, err := client.Do(req)
	if err != nil {
		return bootstrap.BootstrapResponse{}, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return bootstrap.BootstrapResponse{}, fmt.Errorf("bootstrap endpoint returned %d: %s", res.StatusCode, body)
	}

	var bResp bootstrap.BootstrapResponse
	if err := json.NewDecoder(res.Body).Decode(&bResp); err != nil {
		return bootstrap.BootstrapResponse{}, fmt.Errorf("decode bootstrap response: %w", err)
	}
	return bResp, nil
}
