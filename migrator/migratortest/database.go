package migratortest

import (
	"net/url"
	"strings"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for pgtestdb
	"github.com/peterldowns/pgtestdb"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/migrator"
)

// config holds the one thing about a test database that genuinely varies by environment:
// where it is. Defaults to localhost (bare host / native `go test`); overridden to the
// "postgres" Compose service name inside the devcontainer. This package stays generic —
// it knows nothing about checkpoints or any other domain-specific bootstrap data; callers
// that need those (see scraper/testcfg) pass them in explicitly.
type config struct {
	DatabaseURL string `env:"TEST_DATABASE_URL" envDefault:"postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable"`
}

func newConfig() config {
	return env.Must(env.ParseAs[config]())
}

// CreateScraperTestDatabase creates a test database with migrations applied + scraper checkpoint initialized.
// Returns the connection pool ready for use.
func CreateScraperTestDatabase(t *testing.T, migrationsDir string, initialCheckpoint uint64) *pgxpool.Pool {
	t.Helper()

	pool := CreateTestDatabase(t, migrator.NewSchemaMigrator(migrationsDir))

	err := migrator.InitializeCheckpoint(t.Context(), pool, initialCheckpoint)
	require.NoError(t, err)

	return pool
}

// CreateTestDatabase creates a test database using any pgtestdb.Migrator implementation
// (e.g. migrator.SchemaMigrator, or a caller-provided one that seeds fixture data).
// Returns the connection pool ready for use.
func CreateTestDatabase(t *testing.T, migratorInstance pgtestdb.Migrator) *pgxpool.Pool {
	t.Helper()

	dbConfig := pgtestdb.Custom(t, createTestDatabaseConfig(t), migratorInstance)

	pool, err := pgxpool.New(t.Context(), dbConfig.URL())
	require.NoError(t, err)

	t.Logf("testdbconf: %s", dbConfig.URL())

	return pool
}

// createTestDatabaseConfig builds the pgtestdb connection config from TEST_DATABASE_URL, so the
// same test suite can target a local Postgres (default: localhost) or the "postgres" Docker
// Compose service (e.g. when running inside the devcontainer) without code changes.
func createTestDatabaseConfig(t *testing.T) pgtestdb.Config {
	t.Helper()

	dsn, err := url.Parse(newConfig().DatabaseURL)
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
