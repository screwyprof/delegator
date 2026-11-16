// Command web serves the delegations HTTP API (GET /xtz/delegations) from the database the
// scraper fills.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/screwyprof/delegator/pkg/logger"
	"github.com/screwyprof/delegator/pkg/pgxdb"
	"github.com/screwyprof/delegator/web/config"
	"github.com/screwyprof/delegator/web/handler"
	"github.com/screwyprof/delegator/web/store/pgxstore"
)

// Set at build time with -ldflags "-X main.version=... -X main.date=...".
//
//nolint:gochecknoglobals // stamped by the linker, so they must be package-level variables
var (
	version = "dev"
	date    = "unknown"
)

// HTTP server timeouts. ReadHeaderTimeout is the one that matters most: without it a client can
// hold a connection open indefinitely by trickling request headers (Slowloris).
const (
	readHeaderTimeout  = 5 * time.Second
	readTimeout        = 10 * time.Second
	writeTimeout       = 30 * time.Second
	idleTimeout        = 120 * time.Second
	shutdownTimeout    = 30 * time.Second // grace period for in-flight requests
	healthcheckTimeout = 3 * time.Second  // whole `web healthcheck` probe
)

func main() {
	// `web healthcheck` probes a running instance and exits; the image is scratch, so a container
	// healthcheck has no curl to call /healthz with.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck(); err != nil {
			fmt.Fprintln(os.Stderr, "unhealthy:", err)
			os.Exit(1)
		}

		return
	}

	// os.Exit only here, after run has returned: an os.Exit inside run would skip its defers
	// (closing the pool, stopping signal delivery).
	if err := run(); err != nil {
		slog.Error("Web API service failed", slog.Any("error", err))
		os.Exit(1)
	}
}

// healthcheck probes the instance listening on this container's configured port.
func healthcheck() error {
	cfg := config.New()

	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()

	return checkHealth(ctx, "http://"+net.JoinHostPort("127.0.0.1", cfg.HTTPPort))
}

// checkHealth reports an error unless GET baseURL/healthz answers 200.
func checkHealth(ctx context.Context, baseURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("building health request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling /healthz: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: /healthz answered %d", ErrUnhealthy, resp.StatusCode)
	}

	return nil
}

// ErrUnhealthy means /healthz answered, but not with 200.
var ErrUnhealthy = errors.New("service unhealthy")

func run() error {
	cfg := config.New()

	log := logger.NewFromConfig(logger.Config{
		LogLevel:         cfg.LogLevel,
		LogHumanFriendly: cfg.LogHumanFriendly,
	})
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.InfoContext(ctx, "Delegator Web API Service starting",
		slog.String("version", version),
		slog.String("date", date),
	)

	db, err := pgxdb.NewConnection(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()

	store := pgxstore.New(db)
	mux := http.NewServeMux()
	handler.NewTezosGetDelegations(store, handler.WithPublicURL(&cfg.PublicURL)).AddRoutes(mux)
	handler.NewHealth(db).AddRoutes(mux)

	addr := net.JoinHostPort(cfg.HTTPHost, cfg.HTTPPort)
	server := newServer(addr, logger.NewMiddleware(log)(mux))

	// The listener's error comes back over a channel rather than ending the process from the
	// goroutine, so run's defers still execute when the port can't be bound.
	serveErr := make(chan error, 1)

	go func() {
		log.InfoContext(ctx, "Server started", slog.String("addr", addr))

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serving HTTP: %w", err)
	case <-ctx.Done():
	}

	log.InfoContext(ctx, "Shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down server: %w", err)
	}

	log.InfoContext(ctx, "Server exited gracefully")

	return nil
}

// newServer builds the HTTP server with every timeout bounded.
func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}
