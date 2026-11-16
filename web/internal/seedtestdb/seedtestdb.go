// Package seedtestdb builds test databases seeded with realistic demo delegation
// data, for web's acceptance tests. It exists here (not in migrator) because
// migrator has no reason to know about the scraper: seeding fixture data by
// actually running the scraper is a test concern of whoever consumes it — web.
package seedtestdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/screwyprof/delegator/migrator"
	"github.com/screwyprof/delegator/migrator/migratortest"
	"github.com/screwyprof/delegator/pkg/pgxdb"
	"github.com/screwyprof/delegator/pkg/tzkt"
	"github.com/screwyprof/delegator/scraper"
	"github.com/screwyprof/delegator/scraper/config"
	"github.com/screwyprof/delegator/scraper/store/pgxstore"
	"github.com/screwyprof/delegator/scraper/testcfg"
)

// testConfig holds the one value here with no production meaning: how long to wait for
// seeding.
type testConfig struct {
	SeedTimeout time.Duration `env:"TEST_SEED_TIMEOUT" envDefault:"5s"`
}

func newTestConfig() testConfig {
	return env.Must(env.ParseAs[testConfig]())
}

// ErrNegativeCheckpoint rejects a TEST_CHECKPOINT below zero.
var ErrNegativeCheckpoint = errors.New("demo checkpoint must not be negative")

// CreateSeededTestDatabase creates a test database with migrations applied and seeded
// with realistic demo delegation data (fetched via the real scraper against the real
// TzKT API, at a fast demo-scale checkpoint). Returns the connection pool ready for use.
func CreateSeededTestDatabase(t *testing.T, migrationsDir string) *pgxpool.Pool {
	t.Helper()

	testCfg := newTestConfig()
	checkpoint := testcfg.New().Checkpoint // see scraper/testcfg.Config.Checkpoint for why it's owned there
	cfg := config.New()                    // parsed once, reused for chunk size and the scraper client below

	migratorInstance := newSeededMigrator(migrationsDir, checkpoint, cfg, testCfg.SeedTimeout)

	return migratortest.CreateTestDatabase(t, migratorInstance)
}

// seededMigrator applies schema migrations, then seeds the database with demo delegation
// data by running the real scraper. Implements pgtestdb.Migrator.
type seededMigrator struct {
	migrationsDir  string
	demoCheckpoint int64
	cfg            config.Config
	seedTimeout    time.Duration
}

func newSeededMigrator(
	migrationsDir string,
	demoCheckpoint int64,
	cfg config.Config,
	seedTimeout time.Duration,
) *seededMigrator {
	return &seededMigrator{
		migrationsDir:  migrationsDir,
		demoCheckpoint: demoCheckpoint,
		cfg:            cfg,
		seedTimeout:    seedTimeout,
	}
}

func (m *seededMigrator) Hash() (string, error) {
	baseHash, err := migrator.MigrationsHash(m.migrationsDir)
	if err != nil {
		return "", fmt.Errorf("hashing migrations: %w", err)
	}

	return fmt.Sprintf("seeded_demo_%s_%d_%d", baseHash, m.demoCheckpoint, m.cfg.ChunkSize), nil
}

func (m *seededMigrator) Migrate(ctx context.Context, db *sql.DB, conf pgtestdb.Config) error {
	if err := migrator.ApplyMigrationsDB(ctx, db, m.migrationsDir); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	return m.seedDemoData(ctx, conf.URL())
}

// seedDemoData seeds the template database with demo delegation data: it points the scraper's
// checkpoint at the demo checkpoint, then backfills from TzKT until caught up.
func (m *seededMigrator) seedDemoData(ctx context.Context, dbURL string) error {
	slog.InfoContext(ctx, "🌱 Seeding demo database with delegation data",
		"checkpoint", m.demoCheckpoint,
		"chunkSize", m.cfg.ChunkSize,
		"timeout", m.seedTimeout)

	seedCtx, cancel := context.WithTimeout(ctx, m.seedTimeout)
	defer cancel()

	pool, err := pgxdb.NewConnection(seedCtx, dbURL)
	if err != nil {
		return fmt.Errorf("connecting to template database: %w", err)
	}
	defer pool.Close()

	// Set the demo checkpoint for seeding (always overwrite for consistent seeding). TzKT ids are
	// positive; a negative TEST_CHECKPOINT would otherwise wrap to a huge uint64 checkpoint.
	if m.demoCheckpoint < 0 {
		return fmt.Errorf("%w: %d", ErrNegativeCheckpoint, m.demoCheckpoint)
	}

	if err := migrator.SetCheckpoint(seedCtx, pool, uint64(m.demoCheckpoint)); err != nil {
		return fmt.Errorf("setting demo checkpoint: %w", err)
	}

	return m.backfill(seedCtx, cancel, pool)
}

// backfill runs the scraper over pool until its backfill finishes, then stops it via cancel.
func (m *seededMigrator) backfill(ctx context.Context, cancel context.CancelFunc, pool *pgxpool.Pool) error {
	store := pgxstore.New(pool)
	httpClient := &http.Client{Timeout: m.cfg.HTTPClientTimeout}
	client := tzkt.NewClient(httpClient, m.cfg.TzktAPIURL)

	service := scraper.NewService(
		client,
		store,
		scraper.WithChunkSize(m.cfg.ChunkSize),
		scraper.WithPollInterval(m.cfg.PollInterval),
	)

	events, done := service.Start(ctx)

	resultChan := make(chan error, 1)

	subscriberCloser := scraper.NewSubscriber(events,
		scraper.OnBackfillDone(func(scraper.BackfillDone) {
			slog.InfoContext(ctx, "✅ Demo database seeding completed successfully")

			resultChan <- nil

			cancel()
		}),
		scraper.OnBackfillError(func(e scraper.BackfillError) {
			resultChan <- e.Err

			cancel()
		}),
	)

	<-done

	// subscriberCloser blocks until the events channel is drained, so the terminal
	// BackfillDone/BackfillError event is guaranteed to have reached resultChan by
	// the time it returns. Calling it here (not deferred) avoids a race where <-done
	// unblocks before the subscriber's dispatch goroutine has processed the buffered
	// event, which would let the non-blocking select below fall through to "success"
	// on a failed backfill.
	subscriberCloser()

	select {
	case err := <-resultChan:
		return err
	default:
		return nil
	}
}
