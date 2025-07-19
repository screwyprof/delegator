package pgxdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors for pgxdb package operations
var (
	// Connection errors
	ErrInvalidConnectionString = errors.New("invalid database connection string")
	ErrConnectionPoolCreation  = errors.New("failed to create database connection pool")
	ErrDatabaseConnection      = errors.New("failed to connect to database")
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
