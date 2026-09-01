package migratortest

import (
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for pgtestdb
	"github.com/peterldowns/pgtestdb"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/migrator"
	"github.com/screwyprof/delegator/scraper/testcfg"
)

// CreateScraperTestDatabase creates a test database with migrations applied + scraper checkpoint initialized.
// Returns the connection pool ready for use.
func CreateScraperTestDatabase(t *testing.T, migrationsDir string, initialCheckpoint uint64) *pgxpool.Pool {
	t.Helper()

	// Apply schema migrations first
	migratorInstance := migrator.NewSchemaMigrator(migrationsDir)
	pool := createTestDatabaseWithMigrator(t, migratorInstance)

	// Initialize checkpoint separately (like production would)
	err := migrator.InitializeCheckpoint(t.Context(), pool, initialCheckpoint)
	require.NoError(t, err)

	return pool
}

// CreateSeededTestDatabase creates a test database with migrations and demo data seeded.
// Returns the connection pool ready for use.
func CreateSeededTestDatabase(t *testing.T, migrationsDir string) *pgxpool.Pool {
	t.Helper()

	scraperCfg := testcfg.New()

	migratorInstance := migrator.NewSeededMigrator(migrationsDir, scraperCfg.Checkpoint, scraperCfg.ChunkSize, scraperCfg.SeedTimeout)
	return createTestDatabaseWithMigrator(t, migratorInstance)
}

// createTestDatabaseWithMigrator creates a test database using the provided migrator
func createTestDatabaseWithMigrator(t *testing.T, migratorInstance pgtestdb.Migrator) *pgxpool.Pool {
	t.Helper()

	config := createTestDatabaseConfig(t)

	// Create test database and get its config
	dbConfig := pgtestdb.Custom(t, config, migratorInstance)

	// Connect to the test database using test context for proper lifecycle management
	pool, err := pgxpool.New(t.Context(), dbConfig.URL())
	require.NoError(t, err)

	// Log the database URL for debugging
	t.Logf("testdbconf: %s", dbConfig.URL())

	return pool
}

// createTestDatabaseConfig builds the pgtestdb connection config from SCRAPER_TEST_DATABASE_URL,
// so the same test suite can target a local Postgres (default: localhost) or the "postgres"
// Docker Compose service (e.g. when running inside the devcontainer) without code changes.
func createTestDatabaseConfig(t *testing.T) pgtestdb.Config {
	t.Helper()

	dsn, err := url.Parse(testcfg.New().DatabaseURL)
	require.NoError(t, err)

	password, _ := dsn.User.Password()

	return pgtestdb.Config{
		DriverName: "pgx",
		User:       dsn.User.Username(),
		Password:   password,
		Host:       dsn.Hostname(),
		Port:       dsn.Port(),
		Database:   strings.TrimPrefix(dsn.Path, "/"),
		Options:    dsn.RawQuery,
	}
}
