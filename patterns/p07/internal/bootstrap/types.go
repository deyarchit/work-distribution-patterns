package bootstrap

import "time"

// BootstrapResponse is returned by GET /worker/bootstrap to authenticated edge workers.
// It contains everything the worker needs to connect to the task broker — discovered at
// runtime via an mTLS-authenticated handshake rather than baked into the worker's config.
type BootstrapResponse struct {
	BrokerURL string    `json:"brokerURL"` // e.g., "nats://broker:4222"
	Token     string    `json:"token"`     // short-lived HMAC-signed credential
	ExpiresAt time.Time `json:"expiresAt"` // worker must re-bootstrap before this
}
