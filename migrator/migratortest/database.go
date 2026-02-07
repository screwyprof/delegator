package migratortest

import (
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

	config := createTestDatabaseConfig()

	// Create test database and get its config
	dbConfig := pgtestdb.Custom(t, config, migratorInstance)

	// Connect to the test database using test context for proper lifecycle management
	pool, err := pgxpool.New(t.Context(), dbConfig.URL())
	require.NoError(t, err)

	// Log the database URL for debugging
	t.Logf("testdbconf: %s", dbConfig.URL())

	return pool
}

// createTestDatabaseConfig creates the standard pgtestdb configuration for delegator tests
func createTestDatabaseConfig() pgtestdb.Config {
	return pgtestdb.Config{
		DriverName: "pgx",
		User:       "delegator",
		Password:   "delegator",
		Host:       "localhost", // TODO: (postgress when run in devcontainer)
		Port:       "5432",
		Options:    "sslmode=disable",
	}
}
