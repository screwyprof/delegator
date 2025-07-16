package pgxdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
)

// Sentinel errors for pgxdb package operations
var (
	// Connection errors
	ErrInvalidConnectionString = errors.New("invalid database connection string")
	ErrConnectionPoolCreation  = errors.New("failed to create database connection pool")
	ErrDatabaseConnection      = errors.New("failed to connect to database")

	// Migration errors
	ErrMigrationExecution = errors.New("failed to execute database migrations")

	// Checkpoint errors
	ErrCheckpointInitialization = errors.New("failed to initialize checkpoint")
)

// NewConnection creates a new pgx database connection pool with production-optimized settings
func NewConnection(ctx context.Context, connectionString string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConnectionString, err)
	}

	// Optimize connection pool settings based on production best practices
	// See: https://blog.cloudflare.com/how-hyperdrive-speeds-up-database-access/
	// See: https://koho.dev/understanding-go-and-databases-at-scale-connection-pooling-f301e56fa73

	// Pool size: Start small, scale based on actual usage
	config.MinConns = 2  // Always keep minimum connections warm
	config.MaxConns = 10 // Reasonable max for most applications

	// Connection lifecycle management
	config.MaxConnLifetime = 30 * time.Minute  // Prevent stale connections
	config.MaxConnIdleTime = 5 * time.Minute   // Close idle connections quickly
	config.HealthCheckPeriod = 1 * time.Minute // Regular health checks

	// Acquisition settings
	config.ConnConfig.ConnectTimeout = 10 * time.Second // Don't wait too long for new connections

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionPoolCreation, err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("%w: %v", ErrDatabaseConnection, err)
	}

	return pool, nil
}

// ApplyMigrations applies database migrations using sql-migrate with the provided pgx pool
func ApplyMigrations(pool *pgxpool.Pool, migrationsDir string) error {
	// Create sql.DB from the pgx pool for sql-migrate
	db := stdlib.OpenDBFromPool(pool)

	migrations := &migrate.FileMigrationSource{
		Dir: migrationsDir,
	}

	migrationSet := &migrate.MigrationSet{
		TableName: "schema_migrations",
	}

	_, err := migrationSet.Exec(db, "postgres", migrations, migrate.Up)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMigrationExecution, err)
	}

	return nil
}

// InittialiseCheckpointIfNotSet ensures checkpoint is properly initialized
func InittialiseCheckpointIfNotSet(ctx context.Context, db *pgxpool.Pool, initialCheckpoint uint64) error {
	// Insert initial value if no row exists, do nothing if row already exists
	_, err := db.Exec(ctx, `
		INSERT INTO scraper_checkpoint (single_row, last_id) VALUES (TRUE, $1) 
		ON CONFLICT (single_row) DO NOTHING
	`, initialCheckpoint)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCheckpointInitialization, err)
	}

	return nil
}
