// Package migrator applies the database schema with sql-migrate and manages the scraper's
// checkpoint row. It also adapts the migrations as a pgtestdb.Migrator, so tests build their
// databases from the same files production runs.
package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/sqlmigrator"
	migrate "github.com/rubenv/sql-migrate"
)

// migrationsTableName is the sql-migrate bookkeeping table name.
const migrationsTableName = "schema_migrations"

const schemaHashPrefix = "schema_only_"

// SQL queries.
const (
	initCheckpointSQL = `
		INSERT INTO scraper_checkpoint (single_row, last_id) 
		VALUES (TRUE, $1)
		ON CONFLICT (single_row) DO NOTHING`

	setCheckpointSQL = `
		INSERT INTO scraper_checkpoint (single_row, last_id) 
		VALUES (TRUE, $1)
		ON CONFLICT (single_row) DO UPDATE SET last_id = EXCLUDED.last_id`
)

// Migration-related errors.
var (
	ErrMigrationExecution  = errors.New("migration execution failed")
	ErrCheckpointOperation = errors.New("checkpoint operation failed")
)

// SchemaMigrator applies only database schema migrations
// Used for production and tests that need schema-only setup.
type SchemaMigrator struct {
	migrationsDir string
}

// NewSchemaMigrator creates a migrator that applies schema migrations only.
func NewSchemaMigrator(migrationsDir string) *SchemaMigrator {
	return &SchemaMigrator{
		migrationsDir: migrationsDir,
	}
}

// Hash identifies the schema for pgtestdb's template cache: it changes whenever a migration does.
func (m *SchemaMigrator) Hash() (string, error) {
	baseHash, err := MigrationsHash(m.migrationsDir)
	if err != nil {
		return "", err
	}

	return schemaHashPrefix + baseHash, nil
}

// MigrationsHash returns the sql-migrate hash for the migrations in migrationsDir, with no
// prefix. Exported so other pgtestdb.Migrator implementations that need their own composite
// hash (see web/internal/seedtestdb) can build on the same base instead of recomputing it.
func MigrationsHash(migrationsDir string) (string, error) {
	source := &migrate.FileMigrationSource{Dir: migrationsDir}
	migrationSet := &migrate.MigrationSet{TableName: migrationsTableName}
	sqlMigrator := sqlmigrator.New(source, migrationSet)

	baseHash, err := sqlMigrator.Hash()
	if err != nil {
		return "", fmt.Errorf("failed to calculate migration hash for %s: %w", migrationsDir, err)
	}

	return baseHash, nil
}

// Migrate applies the migrations to a pgtestdb template database.
func (m *SchemaMigrator) Migrate(ctx context.Context, db *sql.DB, _ pgtestdb.Config) error {
	return ApplyMigrationsDB(ctx, db, m.migrationsDir)
}

// ApplyMigrations applies database migrations using sql-migrate with the provided pgx pool.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	// Create sql.DB from the pgx pool for sql-migrate
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	return ApplyMigrationsDB(ctx, db, migrationsDir)
}

// InitializeCheckpoint initializes the scraper checkpoint if not already set.
func InitializeCheckpoint(ctx context.Context, pool *pgxpool.Pool, initialCheckpoint uint64) error {
	_, err := pool.Exec(ctx, initCheckpointSQL, initialCheckpoint)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCheckpointOperation, err)
	}

	return nil
}

// SetCheckpoint sets the scraper checkpoint, overwriting any existing value.
func SetCheckpoint(ctx context.Context, pool *pgxpool.Pool, checkpoint uint64) error {
	_, err := pool.Exec(ctx, setCheckpointSQL, checkpoint)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCheckpointOperation, err)
	}

	return nil
}

// ApplyMigrationsDB applies database migrations using sql-migrate against an existing [sql.DB].
// Exported so pgtestdb.Migrator implementations outside this package (see
// web/internal/seedtestdb) can apply migrations without duplicating this logic; pgtestdb
// hands them a [sql.DB] directly, not a pgxpool.Pool.
func ApplyMigrationsDB(ctx context.Context, db *sql.DB, migrationsDir string) error {
	source := &migrate.FileMigrationSource{Dir: migrationsDir}
	migrationSet := &migrate.MigrationSet{TableName: migrationsTableName}

	_, err := migrationSet.ExecContext(ctx, db, "postgres", source, migrate.Up)
	if err == nil {
		return nil
	}

	// sql-migrate's *TxError has no Unwrap, which would hide e.g. context.Canceled from
	// errors.Is; re-wrap its cause ourselves, keeping the failing migration's id.
	if txErr, ok := errors.AsType[*migrate.TxError](err); ok {
		return fmt.Errorf("%w: %s: %w", ErrMigrationExecution, txErr.Migration.Id, txErr.Err)
	}

	return fmt.Errorf("%w: %w", ErrMigrationExecution, err)
}
