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
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 30 * time.Second // grace period for in-flight requests
)

func main() {
	// os.Exit only here, after run has returned: an os.Exit inside run would skip its defers
	// (closing the pool, stopping signal delivery).
	if err := run(); err != nil {
		slog.Error("Web API service failed", slog.Any("error", err))
		os.Exit(1)
	}
}

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

	store, storeCloser := pgxstore.New(db)
	defer storeCloser()

	mux := http.NewServeMux()
	handler.NewTezosGetDelegations(store, handler.WithPublicURL(&cfg.PublicURL)).AddRoutes(mux)

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
