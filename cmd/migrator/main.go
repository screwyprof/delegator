// Command migrator applies the database migrations and optionally seeds the scraper's starting
// checkpoint, then exits. It runs before the scraper and web services start.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/screwyprof/delegator/migrator"
	"github.com/screwyprof/delegator/migrator/config"
	"github.com/screwyprof/delegator/pkg/logger"
	"github.com/screwyprof/delegator/pkg/pgxdb"
)

// Set at build time with -ldflags "-X main.version=... -X main.date=...".
//
//nolint:gochecknoglobals // stamped by the linker, so they must be package-level variables
var (
	version = "dev"
	date    = "unknown"
)

func main() {
	// os.Exit only here, after run has returned: an os.Exit inside run would skip its defers
	// (closing the pool, cancelling the timeout, stopping signal delivery).
	if err := run(); err != nil {
		slog.Error("Database migrator failed", slog.Any("error", err))
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

	log.Info("Starting database migrator service",
		slog.String("migrationsDir", cfg.MigrationsDir),
		slog.Uint64("initialCheckpoint", cfg.InitialCheckpoint),
		slog.String("version", version),
		slog.String("date", date),
	)

	// SIGINT/SIGTERM cancel everything. The operation timeout bounds connecting and the checkpoint
	// step, but not the migrations: how long they take depends on the size of the table, and an
	// index build cut off at a fixed limit would be retried and cut off again on every restart.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connectCtx, cancelConnect := context.WithTimeout(ctx, cfg.OperationTimeout)
	defer cancelConnect()

	db, err := pgxdb.NewConnection(connectCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()

	log.Info("Applying database migrations")

	if err := migrator.ApplyMigrations(ctx, db, cfg.MigrationsDir); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	log.Info("Database migrations applied successfully")

	if cfg.InitialCheckpoint > 0 {
		log.Info("Initializing checkpoint", slog.Uint64("checkpoint", cfg.InitialCheckpoint))

		checkpointCtx, cancelCheckpoint := context.WithTimeout(ctx, cfg.OperationTimeout)
		defer cancelCheckpoint()

		if err := migrator.InitializeCheckpoint(checkpointCtx, db, cfg.InitialCheckpoint); err != nil {
			return fmt.Errorf("initializing checkpoint: %w", err)
		}

		log.Info("Checkpoint initialized successfully")
	}

	log.Info("Database migrator completed successfully")

	return nil
}
