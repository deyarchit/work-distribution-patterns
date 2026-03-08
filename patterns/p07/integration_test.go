package p07_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"work-distribution-patterns/patterns/p07/internal/app"
	"work-distribution-patterns/shared/testutil"
)

// testPKI holds ephemeral cryptographic material for the integration test.
// All certs are generated in-process — no files, no external tooling.
type testPKI struct {
	serverTLS  *tls.Config     // manager bootstrap listener TLS (requires+verifies client certs)
	workerCert tls.Certificate // device client cert signed by the test CA
	workerTLS  *tls.Config     // worker HTTP client TLS (presents device cert, trusts test CA)
	caPool     *x509.CertPool  // CA pool for verifying either end
}

func TestP7Integration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Generate test PKI: CA, server cert (for bootstrap listener), worker device cert.
	pki := generateTestPKI(t, "device-test-worker")

	// 2. Start NATS container (JetStream enabled by default in the testcontainers NATS module).
	natsC, err := tcnats.Run(ctx, "nats:2-alpine")
	if err != nil {
		t.Fatalf("nats container: %v", err)
	}
	t.Cleanup(func() { _ = natsC.Terminate(context.Background()) })
	brokerURL, err := natsC.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("nats url: %v", err)
	}

	// 3. Start Postgres container.
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

	// 4. Start manager (plain task API + mTLS bootstrap server).
	// Token TTL is deliberately short so the renewal test can verify re-bootstrap
	// happens and the worker continues processing tasks without interruption.
	comps, err := app.NewManager(ctx, app.ManagerConfig{
		BrokerURL:       brokerURL,
		DatabaseURL:     dbURL,
		BootstrapAddr:   "127.0.0.1:0",
		TokenSecret:     "integration-test-secret",
		TokenTTL:        4 * time.Second,
		ServerTLSConfig: pki.serverTLS,
	})
	if err != nil {
		t.Fatalf("manager setup: %v", err)
	}
	mgrURL := startServer(t, comps.Router)
	bootstrapURL := startBootstrapServer(t, comps)
	testutil.WaitReady(t, mgrURL)

	// 5. Start worker. It only knows the bootstrap URL and its device cert — no broker config.
	go app.RunWorker(ctx, app.WorkerConfig{
		BootstrapURL:     bootstrapURL,
		TLSConfig:        pki.workerTLS,
		MaxStageDuration: 100,
	})

	// 6. Start API (same as P06 — subscribes to broker events for SSE fan-out).
	apiE, err := app.NewAPI(ctx, app.APIConfig{ManagerURL: mgrURL, BrokerURL: brokerURL})
	if err != nil {
		t.Fatalf("api setup: %v", err)
	}
	apiURL := startServer(t, apiE)
	testutil.WaitReady(t, apiURL)

	// WaitForWorker submits a probe task and waits for it to complete, ensuring the
	// worker has successfully bootstrapped and is processing tasks before RunSuite starts.
	testutil.WaitForWorker(t, apiURL)

	// Core task lifecycle tests — identical to all other patterns.
	testutil.RunSuite(t, apiURL)

	// P7-specific tests below.

	t.Run("MTLSEnforcement", func(t *testing.T) {
		// A client presenting NO client cert must be rejected at the TLS handshake.
		// This verifies the bootstrap endpoint is not open to anonymous callers.
		noCertClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: pki.caPool, // trusts the server, but presents no client cert
				},
			},
			Timeout: 5 * time.Second,
		}
		resp, err := noCertClient.Get(bootstrapURL + "/worker/bootstrap")
		if err == nil {
			_ = resp.Body.Close()
			t.Fatal("expected TLS error for unauthenticated client, got nil")
		}
	})

	t.Run("Revocation", func(t *testing.T) {
		// Revoke the worker's CN. The next bootstrap call must return 403.
		comps.Revocation.Revoke("device-test-worker")

		revokedClient := &http.Client{
			Transport: &http.Transport{TLSClientConfig: pki.workerTLS},
			Timeout:   5 * time.Second,
		}
		resp, err := revokedClient.Get(bootstrapURL + "/worker/bootstrap")
		if err != nil {
			t.Fatalf("unexpected HTTP error: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for revoked device, got %d", resp.StatusCode)
		}
	})

	t.Run("TokenRenewal", func(t *testing.T) {
		// TokenTTL is 4s. After 5s the original token has expired and the worker's
		// re-bootstrap goroutine must have renewed it. Verify the worker is still
		// processing tasks (re-bootstrap succeeded and the work loop is running).
		//
		// Note: revocation was applied in the previous subtest. Un-revoke so the
		// worker can renew successfully.
		comps.Revocation.Revoke("device-test-worker") // already revoked; no-op

		// Start a fresh worker with a separate device cert so it can renew freely.
		freshPKI := generateTestPKI(t, "device-renewal-worker")
		freshCtx, freshCancel := context.WithCancel(ctx)
		defer freshCancel()

		go app.RunWorker(freshCtx, app.WorkerConfig{
			BootstrapURL:     bootstrapURL,
			TLSConfig:        freshPKI.workerTLS,
			MaxStageDuration: 100,
		})

		// Wait long enough for the token to expire and be renewed (TTL=4s, so >5s).
		time.Sleep(5 * time.Second)

		// If renewal worked, the fresh worker is still alive and processing.
		testutil.WaitForWorker(t, apiURL)
	})
}

// startServer starts e on a random port and registers cleanup. Returns the base URL.
func startServer(t *testing.T, e *echo.Echo) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })
	go func() { _ = e.Server.Serve(ln) }()
	return "http://" + ln.Addr().String()
}

// startBootstrapServer starts the mTLS bootstrap router on the listener NewManager created.
// Returns the base URL (https://...).
func startBootstrapServer(t *testing.T, comps app.ManagerComponents) string {
	t.Helper()
	t.Cleanup(func() { _ = comps.BootstrapRouter.Shutdown(context.Background()) })
	go func() { _ = comps.BootstrapRouter.Server.Serve(comps.BootstrapListener) }()
	return "https://" + comps.BootstrapListener.Addr().String()
}

// generateTestPKI creates an ephemeral CA, a server cert for the bootstrap listener,
// and a device cert with the given CN for a worker. All material is in-memory.
func generateTestPKI(t *testing.T, deviceCN string) *testPKI {
	t.Helper()

	// CA
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca", Organization: []string{"test"}},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	// Server cert (for bootstrap listener — ExtKeyUsageServerAuth + localhost SAN).
	serverCert := signCert(t, caKey, caCert, "bootstrap-server", true)

	// Worker device cert (ExtKeyUsageClientAuth).
	workerCert := signCert(t, caKey, caCert, deviceCN, false)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}

	workerTLS := &tls.Config{
		Certificates: []tls.Certificate{workerCert},
		RootCAs:      caPool,
	}

	return &testPKI{
		serverTLS:  serverTLS,
		workerCert: workerCert,
		workerTLS:  workerTLS,
		caPool:     caPool,
	}
}

// signCert creates and signs a certificate using the given CA key and cert.
func signCert(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, cn string, isServer bool) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key for %s: %v", cn, err)
	}

	extKeyUsage := x509.ExtKeyUsageClientAuth
	if isServer {
		extKeyUsage = x509.ExtKeyUsageServerAuth
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"edge-workers"},
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{extKeyUsage},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create cert for %s: %v", cn, err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair for %s: %v", cn, err)
	}
	return cert
}
