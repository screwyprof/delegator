package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/sqlmigrator"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/screwyprof/delegator/pkg/pgxdb"
	"github.com/screwyprof/delegator/pkg/tzkt"
	"github.com/screwyprof/delegator/scraper"
	"github.com/screwyprof/delegator/scraper/config"
	"github.com/screwyprof/delegator/scraper/store/pgxstore"
)

// Migration constants
const (
	migrationsTableName = "schema_migrations"
	schemaHashPrefix    = "schema_only_"
	seededHashPrefix    = "seeded_demo_"
)

// SQL queries
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

// Migration-related errors
var (
	ErrMigrationExecution  = errors.New("migration execution failed")
	ErrCheckpointOperation = errors.New("checkpoint operation failed")
)

// SchemaMigrator applies only database schema migrations
// Used for production and tests that need schema-only setup
type SchemaMigrator struct {
	migrationsDir string
}

// NewSchemaMigrator creates a migrator that applies schema migrations only
func NewSchemaMigrator(migrationsDir string) *SchemaMigrator {
	return &SchemaMigrator{
		migrationsDir: migrationsDir,
	}
}

func (m *SchemaMigrator) Hash() (string, error) {
	source := &migrate.FileMigrationSource{Dir: m.migrationsDir}
	migrationSet := &migrate.MigrationSet{TableName: migrationsTableName}
	sqlMigrator := sqlmigrator.New(source, migrationSet)

	baseHash, err := sqlMigrator.Hash()
	if err != nil {
		return "", fmt.Errorf("failed to calculate migration hash for %s: %w", m.migrationsDir, err)
	}

	return schemaHashPrefix + baseHash, nil
}

func (m *SchemaMigrator) Migrate(ctx context.Context, db *sql.DB, conf pgtestdb.Config) error {
	return applyMigrations(db, m.migrationsDir)
}

// SeededMigrator applies schema migrations + seeds with demo delegation data
// Used for web API tests that need realistic data to test against
type SeededMigrator struct {
	migrationsDir  string
	demoCheckpoint int64
	chunkSize      uint64
	seedTimeout    time.Duration
}

// NewSeededMigrator creates a migrator that applies schema + seeds demo data
func NewSeededMigrator(migrationsDir string, demoCheckpoint int64, chunkSize uint64, seedTimeout time.Duration) *SeededMigrator {
	return &SeededMigrator{
		migrationsDir:  migrationsDir,
		demoCheckpoint: demoCheckpoint,
		chunkSize:      chunkSize,
		seedTimeout:    seedTimeout,
	}
}

func (m *SeededMigrator) Hash() (string, error) {
	source := &migrate.FileMigrationSource{Dir: m.migrationsDir}
	migrationSet := &migrate.MigrationSet{TableName: migrationsTableName}
	sqlMigrator := sqlmigrator.New(source, migrationSet)

	baseHash, err := sqlMigrator.Hash()
	if err != nil {
		return "", fmt.Errorf("failed to calculate migration hash for %s: %w", m.migrationsDir, err)
	}

	return seededHashPrefix + baseHash + "_" + strconv.FormatInt(m.demoCheckpoint, 10) + "_" + strconv.FormatUint(m.chunkSize, 10), nil
}

func (m *SeededMigrator) Migrate(ctx context.Context, db *sql.DB, conf pgtestdb.Config) error {
	// Apply schema migrations using common function
	if err := applyMigrations(db, m.migrationsDir); err != nil {
		return err
	}

	// Then seed with demo data using the scraper
	return m.seedDemoData(ctx, conf.URL())
}

// seedDemoData seeds the template database with demo delegation data
func (m *SeededMigrator) seedDemoData(ctx context.Context, dbURL string) error {
	slog.InfoContext(ctx, "🌱 Seeding demo database with delegation data",
		"checkpoint", m.demoCheckpoint,
		"chunkSize", m.chunkSize,
		"timeout", m.seedTimeout)

	// Create context with timeout for seeding
	seedCtx, cancel := context.WithTimeout(ctx, m.seedTimeout)
	defer cancel()

	// Create database connection for seeding
	pool, err := pgxdb.NewConnection(seedCtx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Set the demo checkpoint for seeding (always overwrite for consistent seeding)
	if err := SetCheckpoint(seedCtx, pool, uint64(m.demoCheckpoint)); err != nil {
		return err
	}

	// Create scraper components for seeding
	store, storeCloser := pgxstore.New(pool)
	defer storeCloser()

	cfg := config.New()
	cfg.ChunkSize = m.chunkSize // Use the configured chunk size, not default

	httpClient := &http.Client{Timeout: cfg.HttpClientTimeout}
	client := tzkt.NewClient(httpClient, cfg.TzktAPIURL)

	service := scraper.NewService(
		client,
		store,
		scraper.WithChunkSize(cfg.ChunkSize),
		scraper.WithPollInterval(cfg.PollInterval),
	)

	// Run scraper to seed data
	events, done := service.Start(seedCtx)

	// Use channel for safe communication between goroutines
	resultChan := make(chan error, 1)

	// Use subscriber pattern for cleaner event handling
	subscriberCloser := scraper.NewSubscriber(events,
		scraper.OnBackfillDone(func(e scraper.BackfillDone) {
			slog.InfoContext(seedCtx, "✅ Demo database seeding completed successfully")
			resultChan <- nil // Signal success
			cancel()          // Stop seeding
		}),
		scraper.OnBackfillError(func(e scraper.BackfillError) {
			resultChan <- e.Err // Signal error
			cancel()            // Stop seeding on error
		}),
	)
	defer subscriberCloser()

	// Wait for completion or timeout (handled by context)
	<-done

	// Get result from channel (non-blocking since we know service finished)
	select {
	case err := <-resultChan:
		return err
	default:
		return nil // No result received, assume success
	}
}

// ApplyMigrations applies database migrations using sql-migrate with the provided pgx pool
func ApplyMigrations(pool *pgxpool.Pool, migrationsDir string) error {
	// Create sql.DB from the pgx pool for sql-migrate
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	return applyMigrations(db, migrationsDir)
}

// InitializeCheckpoint initializes the scraper checkpoint if not already set
func InitializeCheckpoint(ctx context.Context, pool *pgxpool.Pool, initialCheckpoint uint64) error {
	_, err := pool.Exec(ctx, initCheckpointSQL, initialCheckpoint)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCheckpointOperation, err)
	}
	return nil
}

// SetCheckpoint sets the scraper checkpoint, overwriting any existing value
func SetCheckpoint(ctx context.Context, pool *pgxpool.Pool, checkpoint uint64) error {
	_, err := pool.Exec(ctx, setCheckpointSQL, checkpoint)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCheckpointOperation, err)
	}
	return nil
}

// applyMigrations applies database migrations using sql-migrate
func applyMigrations(db *sql.DB, migrationsDir string) error {
	source := &migrate.FileMigrationSource{Dir: migrationsDir}
	migrationSet := &migrate.MigrationSet{TableName: migrationsTableName}

	_, err := migrationSet.Exec(db, "postgres", source, migrate.Up)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMigrationExecution, err)
	}
	return nil
}
