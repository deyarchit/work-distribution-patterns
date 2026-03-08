package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"work-distribution-patterns/patterns/p07/internal/bootstrap"
	pgstore "work-distribution-patterns/patterns/p07/internal/postgres"
	pubsubinternal "work-distribution-patterns/patterns/p07/internal/pubsub"
	"work-distribution-patterns/shared/api"
	"work-distribution-patterns/shared/manager"
	"work-distribution-patterns/shared/models"
	"work-distribution-patterns/shared/templates"
)

// ManagerConfig holds runtime parameters for the Pattern 7 manager process.
type ManagerConfig struct {
	BrokerURL   string
	DatabaseURL string

	// Bootstrap server configuration.
	BootstrapAddr   string        // address for the mTLS listener (e.g. ":8083"; "127.0.0.1:0" in tests)
	TokenSecret     string        // HMAC signing key for issued tokens
	TokenTTL        time.Duration // lifetime of each issued token (default: 15 minutes)
	ServerTLSConfig *tls.Config   // TLS config with ClientAuth: RequireAndVerifyClientCert
	RevokedCNs      []string      // device CNs initially on the deny-list
}

// ManagerComponents holds the components returned by NewManager.
type ManagerComponents struct {
	// Router is the plain-HTTP manager router serving the task API.
	// Used by API processes (RemoteTaskManager) and health checks.
	Router *echo.Echo

	// BootstrapRouter serves GET /worker/bootstrap.
	// Start it with: BootstrapRouter.Server.Serve(BootstrapListener)
	BootstrapRouter *echo.Echo

	// BootstrapListener is the TLS-wrapped net.Listener created by NewManager.
	// ClientAuth is already enforced — pass directly to BootstrapRouter.Server.Serve.
	BootstrapListener net.Listener

	// Revocation exposes the live deny-list so callers can revoke devices at runtime.
	Revocation *bootstrap.RevocationList
}

// NewManager wires the Pattern 7 manager and returns its components.
// It creates the mTLS bootstrap listener using cfg.BootstrapAddr and cfg.ServerTLSConfig;
// the caller starts it with BootstrapRouter.Server.Serve(BootstrapListener).
func NewManager(ctx context.Context, cfg ManagerConfig) (ManagerComponents, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return ManagerComponents{}, err
	}

	taskStore, err := pgstore.New(ctx, pool)
	if err != nil {
		pool.Close()
		return ManagerComponents{}, err
	}

	res, err := pubsubinternal.OpenManagerResources(ctx, cfg.BrokerURL)
	if err != nil {
		pool.Close()
		return ManagerComponents{}, err
	}

	dispatcher := pubsubinternal.NewPubSubDispatcher(res.TasksTopic, res.WorkerEventsSub)
	if startErr := dispatcher.Start(ctx); startErr != nil {
		dispatcher.Shutdown(ctx)
		pool.Close()
		return ManagerComponents{}, startErr
	}

	eventBridge := pubsubinternal.NewPubSubEventBridge(res.APIEventsTopic)
	mgr := manager.New(taskStore, dispatcher, eventBridge, 30*time.Second)
	mgr.Start(ctx)

	tpl, err := template.ParseFS(templates.FS, "index.html")
	if err != nil {
		dispatcher.Shutdown(ctx)
		pool.Close()
		return ManagerComponents{}, err
	}

	// Plain HTTP router — task API for internal API↔Manager communication.
	router := echo.New()
	router.HideBanner = true
	router.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{ //nolint:staticcheck // deprecated but still functional; sufficient for demo
		Skipper: func(c echo.Context) bool {
			return c.Request().URL.Path == "/health"
		},
	}))
	router.Use(middleware.Recover())
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("template", tpl)
			return next(c)
		}
	})

	router.GET("/health", api.Health())
	router.POST("/tasks", func(c echo.Context) error {
		var task models.Task
		if bindErr := c.Bind(&task); bindErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid task body")
		}
		if submitErr := mgr.Submit(c.Request().Context(), task); submitErr != nil {
			return submitErr
		}
		return c.JSON(http.StatusAccepted, map[string]string{"id": task.ID})
	})
	router.GET("/tasks", api.ListTasks(mgr))
	router.GET("/tasks/:id", api.GetTask(mgr))

	// Bootstrap token configuration.
	ttl := cfg.TokenTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	tokener := bootstrap.NewTokener(cfg.TokenSecret, ttl)
	revocation := bootstrap.NewRevocationList(cfg.RevokedCNs)

	// mTLS listener — ClientAuth: RequireAndVerifyClientCert is enforced here,
	// before any HTTP handler runs.
	ln, err := tls.Listen("tcp", cfg.BootstrapAddr, cfg.ServerTLSConfig)
	if err != nil {
		dispatcher.Shutdown(ctx)
		pool.Close()
		return ManagerComponents{}, fmt.Errorf("bootstrap listen: %w", err)
	}

	// mTLS router — bootstrap endpoint for edge workers.
	bootstrapRouter := echo.New()
	bootstrapRouter.HideBanner = true
	bootstrapRouter.Use(middleware.Recover())
	bootstrapRouter.GET("/worker/bootstrap", bootstrapHandler(cfg.BrokerURL, tokener, revocation))

	return ManagerComponents{
		Router:            router,
		BootstrapRouter:   bootstrapRouter,
		BootstrapListener: ln,
		Revocation:        revocation,
	}, nil
}

// bootstrapHandler handles GET /worker/bootstrap.
// The TLS layer (tls.RequireAndVerifyClientCert) has already validated the client cert
// before this handler runs — the cert's CN is trusted and used as the device identity.
func bootstrapHandler(brokerURL string, tokener *bootstrap.Tokener, revocation *bootstrap.RevocationList) echo.HandlerFunc {
	return func(c echo.Context) error {
		tlsState := c.Request().TLS
		if tlsState == nil || len(tlsState.VerifiedChains) == 0 {
			// Should not happen given ClientAuth: RequireAndVerifyClientCert, but be defensive.
			return echo.NewHTTPError(http.StatusUnauthorized, "client certificate required")
		}

		cert := tlsState.VerifiedChains[0][0]
		deviceCN := cert.Subject.CommonName

		if revocation.IsRevoked(deviceCN) {
			log.Printf("p07 bootstrap: rejected revoked device cn=%s", deviceCN)
			return echo.NewHTTPError(http.StatusForbidden, "device revoked")
		}

		token, expiresAt, err := tokener.Issue(deviceCN)
		if err != nil {
			log.Printf("p07 bootstrap: token issue error cn=%s: %v", deviceCN, err)
			return echo.NewHTTPError(http.StatusInternalServerError, "token vending failed")
		}

		log.Printf("p07 bootstrap: issued token cn=%s broker=%s expires=%s", deviceCN, brokerURL, expiresAt.Format(time.RFC3339))

		return c.JSON(http.StatusOK, bootstrap.BootstrapResponse{
			BrokerURL: brokerURL,
			Token:     token,
			ExpiresAt: expiresAt,
		})
	}
}
