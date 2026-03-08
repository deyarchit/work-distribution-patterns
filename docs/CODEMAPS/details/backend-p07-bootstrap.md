<!-- Load when: Working on P7 bootstrap, mTLS, token vending, or device revocation -->

# P07 Bootstrap & mTLS Details

## Bootstrap Handshake

Worker performs mTLS GET /worker/bootstrap:
1. Client presents device cert (CN = device identity)
2. Manager's TLS layer verifies cert against CA pool
3. Manager's HTTP handler reads verified CN from `tls.ConnectionState.VerifiedChains[0][0].Subject.CommonName`
4. Manager checks revocation list — 403 if revoked
5. Manager issues HMAC-signed token with 15-min TTL (configurable)
6. Returns `{BrokerURL, Token, ExpiresAt}`

Worker stores token; uses it when connecting to broker. Periodic renewal at 80% TTL keeps worker alive without redeployment.

## Type Signatures

```go
// BootstrapResponse sent by manager
type BootstrapResponse struct {
    BrokerURL string    // e.g., "nats://broker:4222"
    Token     string    // HMAC-signed JWT-like token
    ExpiresAt time.Time // UTC expiration
}

// Tokener vends and validates tokens
type Tokener struct {
    Issue(deviceCN string) (token, expiresAt, error)
    Verify(token string) (deviceCN, error)
}

// RevocationList thread-safe deny-list
type RevocationList struct {
    Revoke(deviceCN string)
    IsRevoked(deviceCN string) bool
}

// App wiring
func NewManager(ctx context.Context, cfg ManagerConfig) (ManagerComponents, error)
type ManagerComponents struct {
    Router          *echo.Echo                    // Plain HTTP :8081 (task API)
    BootstrapRouter *echo.Echo                    // mTLS :8083 (/worker/bootstrap)
    Revocation      *bootstrap.RevocationList     // Runtime revocation
}

func RunWorker(ctx context.Context, cfg WorkerConfig)  // Blocks; cancel ctx to shutdown
type WorkerConfig struct {
    BootstrapURL     string      // https://<manager>:8083
    TLSConfig        *tls.Config // mTLS client config (device cert + CA)
    MaxStageDuration int         // ms per stage
}
```

## TLS Certificate Setup

**Production**: Load from files using `bootstrap.LoadServerTLS(certPath, keyPath, caPath)` and `bootstrap.LoadClientTLS(certPath, keyPath, caPath)`.

**Integration test**: Generate in-process using `crypto/x509` + `crypto/ecdsa`:
```go
generateTestPKI(t, deviceCN) -> testPKI{serverTLS, workerTLS, caPool}
```
- CA cert: `IsCA=true`, KeyUsageCertSign
- Server cert: ExtKeyUsageServerAuth, DNSNames=["localhost"]
- Worker cert: ExtKeyUsageClientAuth, CN=deviceCN (used as device identity)

## Files

- `patterns/p07/internal/bootstrap/types.go` — BootstrapResponse
- `patterns/p07/internal/bootstrap/token.go` — Tokener (HMAC-SHA256)
- `patterns/p07/internal/bootstrap/revocation.go` — RevocationList (sync.RWMutex)
- `patterns/p07/internal/bootstrap/pki.go` — LoadServerTLS, LoadClientTLS
- `patterns/p07/internal/app/manager.go` — NewManager, bootstrapHandler
- `patterns/p07/internal/app/worker.go` — RunWorker, doBootstrapWithRetry
- `patterns/p07/cmd/manager/main.go` — TLS loading, dual listeners
- `patterns/p07/cmd/worker/main.go` — Client TLS config, worker entry point
