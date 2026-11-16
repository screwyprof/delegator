// Package pgxdb opens pgx connection pools with the settings the services share.
package pgxdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors NewConnection returns, each wrapping the underlying pgx error.
var (
	// ErrInvalidConnectionString means the connection string could not be parsed.
	ErrInvalidConnectionString = errors.New("invalid database connection string")
	// ErrConnectionPoolCreation means the pool could not be created from a valid config.
	ErrConnectionPoolCreation = errors.New("failed to create database connection pool")
	// ErrDatabaseConnection means the pool was created but the database did not answer a ping.
	ErrDatabaseConnection = errors.New("failed to connect to database")
)

// Pool defaults, applied only to settings the connection string leaves out.
// See: https://blog.cloudflare.com/how-hyperdrive-speeds-up-database-access/
// See: https://koho.dev/understanding-go-and-databases-at-scale-connection-pooling-f301e56fa73
const (
	defaultMinConns          = 2                // keep a couple of connections warm
	defaultMaxConns          = 10               // reasonable max for these services
	defaultMaxConnLifetime   = 30 * time.Minute // prevent stale connections
	defaultMaxConnIdleTime   = 5 * time.Minute  // close idle connections quickly
	defaultHealthCheckPeriod = 1 * time.Minute  // regular health checks
	defaultConnectTimeout    = 10 * time.Second // don't wait too long for a new connection
)

// NewConnection creates a pgx connection pool and checks the database answers. Pool settings in
// connectionString (pool_max_conns, connect_timeout, ...) are honoured; see poolConfig.
func NewConnection(ctx context.Context, connectionString string) (*pgxpool.Pool, error) {
	config, err := poolConfig(connectionString)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionPoolCreation, err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("%w: %w", ErrDatabaseConnection, err)
	}

	return pool, nil
}

// poolConfig parses connectionString, then applies the package default for each pool setting the
// string doesn't set itself. Without that check a DSN's pool_max_conns would be silently replaced.
func poolConfig(connectionString string) (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConnectionString, err)
	}

	given := connectionStringKeys(connectionString)

	setDefault := func(key string, apply func()) {
		if !given[key] {
			apply()
		}
	}

	setDefault("pool_min_conns", func() { config.MinConns = defaultMinConns })
	setDefault("pool_max_conns", func() { config.MaxConns = defaultMaxConns })
	setDefault("pool_max_conn_lifetime", func() { config.MaxConnLifetime = defaultMaxConnLifetime })
	setDefault("pool_max_conn_idle_time", func() { config.MaxConnIdleTime = defaultMaxConnIdleTime })
	setDefault("pool_health_check_period", func() { config.HealthCheckPeriod = defaultHealthCheckPeriod })
	setDefault("connect_timeout", func() { config.ConnConfig.ConnectTimeout = defaultConnectTimeout })

	return config, nil
}

// connectionStringKeys returns the setting names a connection string contains, in either form pgx
// accepts: a postgres:// URL (query parameters) or keyword=value pairs. It only needs the names,
// which pgxpool.ParseConfig consumes without recording whether they were given.
func connectionStringKeys(connectionString string) map[string]bool {
	keys := map[string]bool{}

	if u, err := url.Parse(connectionString); err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		for key := range u.Query() {
			keys[key] = true
		}

		return keys
	}

	// pgx allows spaces around '=' (key = value), so a field that starts with '=' belongs to the
	// field before it.
	fields := strings.Fields(connectionString)
	for i, field := range fields {
		key, _, ok := strings.Cut(field, "=")

		switch {
		case !ok:
			continue
		case key != "":
			keys[key] = true
		case i > 0:
			keys[fields[i-1]] = true
		}
	}

	return keys
}
