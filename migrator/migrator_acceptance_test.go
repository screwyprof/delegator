//go:build acceptance

package migrator_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/migrator"
	"github.com/screwyprof/delegator/migrator/migratortest"
)

// Applying migrations honours its context: cmd/migrator lets SIGINT/SIGTERM stop a running
// migration, which only works if cancellation reaches sql-migrate.
func TestApplyMigrationsDBHonoursContext(t *testing.T) {
	t.Parallel()

	// Arrange: an empty database, so there is real work to cancel. (sql-migrate plans without the
	// context and only consults it while executing, so an already-migrated database proves nothing.)
	pool := migratortest.CreateTestDatabase(t, pgtestdb.NoopMigrator{})
	t.Cleanup(pool.Close)
	db := stdlib.OpenDBFromPool(pool)

	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Act
	err := migrator.ApplyMigrationsDB(ctx, db, "migrations")

	// Assert
	require.ErrorIs(t, err, context.Canceled)
}

// Migration 003 recovers from an interrupted run. CREATE INDEX CONCURRENTLY, cut off by
// a signal (say, while it waits behind a long transaction), leaves an INVALID index behind; a retry must rebuild
// it rather than let IF NOT EXISTS skip it and then drop the old, still-working indexes.
func TestMigration003RecoversFromAnInterruptedIndexBuild(t *testing.T) {
	t.Parallel()

	// Arrange: the schema as it was before 003 (migrations 001 and 002 only)
	pool := migratortest.CreateTestDatabase(t, pgtestdb.NoopMigrator{})
	t.Cleanup(pool.Close)

	db := stdlib.OpenDBFromPool(pool)

	t.Cleanup(func() { _ = db.Close() })

	ctx := t.Context()
	require.NoError(t, migrator.ApplyMigrationsDB(ctx, db, migrationsUpTo(t, "002")))

	// ...then 003's first index, left INVALID the way a cancelled concurrent build leaves it
	_, err := pool.Exec(ctx, "CREATE INDEX idx_delegations_timestamp_id ON delegations (timestamp DESC, id DESC)")
	require.NoError(t, err)
	markIndexInvalid(t, pool.Config().ConnConfig.Database, "idx_delegations_timestamp_id")

	// Act: the retry
	require.NoError(t, migrator.ApplyMigrationsDB(ctx, db, "migrations"))

	// Assert
	var valid bool

	err = pool.QueryRow(ctx, `SELECT i.indisvalid FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname = 'idx_delegations_timestamp_id'`).Scan(&valid)
	require.NoError(t, err, "the index should exist after the retry")
	assert.True(t, valid, "the retry should rebuild the INVALID index, not keep it")
}

// migrationsUpTo copies the migrations whose names sort at or before prefix into a temp dir.
func migrationsUpTo(t *testing.T, prefix string) string {
	t.Helper()

	dir := t.TempDir()

	entries, err := os.ReadDir("migrations")
	require.NoError(t, err)

	for _, e := range entries {
		if e.Name() > prefix+"~" {
			continue
		}

		content, err := os.ReadFile(filepath.Join("migrations", e.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, e.Name()), content, 0o600))
	}

	return dir
}

// markIndexInvalid flags an index invalid in the catalog, as Postgres does when a concurrent
// build is interrupted. That takes a superuser, so it connects with TEST_DATABASE_URL's admin
// role to the test's own database.
func markIndexInvalid(t *testing.T, database, index string) {
	t.Helper()

	adminURL, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)

	if adminURL.Host == "" {
		adminURL, err = url.Parse("postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable")
		require.NoError(t, err)
	}

	adminURL.Path = "/" + database

	admin, err := pgx.Connect(t.Context(), adminURL.String())
	require.NoError(t, err)

	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	_, err = admin.Exec(t.Context(),
		"UPDATE pg_index SET indisvalid = false WHERE indexrelid = $1::regclass", index)
	require.NoError(t, err)
}
