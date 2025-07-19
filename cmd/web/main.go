package main

import (
	"context"
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

var (
	version = "dev"
	date    = "unknown"
)

func main() {
	// Load configuration
	cfg := config.New()

	// Initialize logger and set as default
	log := logger.NewFromConfig(logger.Config{
		LogLevel:         cfg.LogLevel,
		LogHumanFriendly: cfg.LogHumanFriendly,
	})
	slog.SetDefault(log)

	// Prepare context with signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.InfoContext(ctx, "Delegator Web API Service starting",
		slog.String("version", version),
		slog.String("date", date),
	)

	// Initialize database connection
	db, err := pgxdb.NewConnection(ctx, cfg.DatabaseURL)
	if err != nil {
		log.ErrorContext(ctx, "Failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer db.Close()

	// Initialize store
	store, storeCloser := pgxstore.New(db)
	defer storeCloser()

	// Create HTTP server
	mux := http.NewServeMux()

	// Register handlers with real store
	tezosHandler := handler.NewTezosGetDelegations(store)
	tezosHandler.AddRoutes(mux)

	// Wrap with logging middleware
	loggedMux := logger.NewMiddleware(log)(mux)

	// Create server address
	addr := net.JoinHostPort(cfg.HTTPHost, cfg.HTTPPort)

	server := &http.Server{
		Addr:    addr,
		Handler: loggedMux,
	}

	// Start server in a goroutine
	go func() {
		log.InfoContext(ctx, "Server started", slog.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.ErrorContext(ctx, "Server failed to start", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	<-ctx.Done()

	log.InfoContext(ctx, "Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.ErrorContext(ctx, "Server forced to shutdown", slog.Any("error", err))
		os.Exit(1)
	}

	log.InfoContext(ctx, "Server exited gracefully")
}
