package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/screwyprof/delegator/migrator"
	"github.com/screwyprof/delegator/migrator/config"
	"github.com/screwyprof/delegator/pkg/logger"
	"github.com/screwyprof/delegator/pkg/pgxdb"
)

// These values are overridden at build time using -ldflags
var (
	version = "dev"
	date    = "unknown"
)

func main() {
	// Load configuration from environment
	cfg := config.New()

	// Initialize logger and set as default
	log := logger.NewFromConfig(logger.Config{
		LogLevel:         cfg.LogLevel,
		LogHumanFriendly: cfg.LogHumanFriendly,
	})
	slog.SetDefault(log)

	log.Info("Starting database migrator service",
		slog.String("migrationsDir", cfg.MigrationsDir),
		slog.Uint64("initialCheckpoint", cfg.InitialCheckpoint),
		slog.String("version", version),
		slog.String("date", date),
	)

	// Create a context that cancels on SIGINT/SIGTERM _or_ when the timeout elapses
	baseCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(baseCtx, cfg.OperationTimeout)
	defer cancel()

	// Connect to database
	db, err := pgxdb.NewConnection(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("Failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer db.Close()

	// Apply migrations
	log.Info("Applying database migrations")
	if err := migrator.ApplyMigrations(db, cfg.MigrationsDir); err != nil {
		log.Error("Failed to apply migrations", slog.Any("error", err))
		os.Exit(1)
	}
	log.Info("Database migrations applied successfully")

	// Set initial checkpoint if specified
	if cfg.InitialCheckpoint > 0 {
		log.Info("Initializing checkpoint", slog.Uint64("checkpoint", cfg.InitialCheckpoint))
		if err := migrator.InitializeCheckpoint(ctx, db, cfg.InitialCheckpoint); err != nil {
			log.Error("Failed to initialize checkpoint", slog.Any("error", err))
			os.Exit(1)
		}
		log.Info("Checkpoint initialized successfully")
	}

	log.Info("Database migrator completed successfully")
}
