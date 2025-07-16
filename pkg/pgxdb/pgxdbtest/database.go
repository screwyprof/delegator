package pgxdbtest

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for pgtestdb
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/sqlmigrator"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/require"
)

// CreateTestDatabase creates a test database with migrations applied.
// Returns the connection pool and database URL for further connections.
func CreateTestDatabase(t *testing.T, migrationsDir string) (*pgxpool.Pool, string) {
	t.Helper()

	config := pgtestdb.Config{
		DriverName: "pgx",
		User:       "delegator",
		Password:   "delegator",
		Host:       "localhost",
		Port:       "5432",
		Options:    "sslmode=disable",
	}

	// Set up sql-migrate migrator
	source := &migrate.FileMigrationSource{
		Dir: migrationsDir,
	}
	migrationSet := &migrate.MigrationSet{
		TableName: "schema_migrations",
	}
	migrator := sqlmigrator.New(source, migrationSet)

	// Create test database and get its config
	dbConfig := pgtestdb.Custom(t, config, migrator)
	dbURL := dbConfig.URL()

	t.Logf("testdbconf: %s", dbURL)

	pool, err := createTestConnection(t.Context(), dbURL)
	require.NoError(t, err)

	return pool, dbURL
}

// createTestConnection creates a connection pool optimized for integration tests
// Test-specific settings based on pgx pool optimization research:
// - Minimal pool size for sequential test execution
// - Shorter lifecycles for faster test cycles
// - Quick failure detection for fast feedback
func createTestConnection(ctx context.Context, connectionString string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, err
	}

	// Test-optimized settings for fast, reliable test execution
	// Based on: https://hexacluster.ai/postgresql/postgresql-client-side-connection-pooling-in-golang-using-pgxpool/
	// Based on: https://medium.com/@neelkanthsingh.jr/understanding-database-connection-pools-and-the-pgx-library-in-go-3087f3c5a0c

	// Minimal pool size for tests - tests don't need production-level concurrency
	config.MinConns = 1 // Keep one connection warm for immediate test use
	config.MaxConns = 2 // Very small max - tests are typically sequential

	// Shorter lifecycles for faster test cycles
	config.MaxConnLifetime = 10 * time.Minute   // Shorter lifetime for test scenarios
	config.MaxConnIdleTime = 1 * time.Minute    // Close idle connections quickly in tests
	config.HealthCheckPeriod = 30 * time.Second // More frequent health checks for faster failure detection

	// Fast connection acquisition for tests
	config.ConnConfig.ConnectTimeout = 5 * time.Second // Fail fast in test scenarios

	return pgxpool.NewWithConfig(ctx, config)
}

// InitializeCheckpoint initializes the scraper checkpoint in the database.
// This is useful for scraper-related tests that need a starting checkpoint.
func InitializeCheckpoint(t *testing.T, testDB *pgxpool.Pool, checkpoint int64) {
	t.Helper()

	_, err := testDB.Exec(t.Context(), "INSERT INTO scraper_checkpoint (single_row, last_id) VALUES (TRUE, $1)", checkpoint)
	require.NoError(t, err)
}
