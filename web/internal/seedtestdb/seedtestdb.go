// Package seedtestdb builds test databases seeded with realistic demo delegation
// data, for web's acceptance tests. It exists here (not in migrator) because
// migrator has no reason to know about the scraper: seeding fixture data by
// actually running the scraper is a test concern of whoever consumes it — web.
package seedtestdb

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"
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

func newSeededMigrator(migrationsDir string, demoCheckpoint int64, cfg config.Config, seedTimeout time.Duration) *seededMigrator {
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
		return "", err
	}

	return "seeded_demo_" + baseHash + "_" + strconv.FormatInt(m.demoCheckpoint, 10) + "_" + strconv.FormatUint(m.cfg.ChunkSize, 10), nil
}

func (m *seededMigrator) Migrate(ctx context.Context, db *sql.DB, conf pgtestdb.Config) error {
	if err := migrator.ApplyMigrationsDB(db, m.migrationsDir); err != nil {
		return err
	}
	return m.seedDemoData(ctx, conf.URL())
}

// seedDemoData seeds the template database with demo delegation data
func (m *seededMigrator) seedDemoData(ctx context.Context, dbURL string) error {
	slog.InfoContext(ctx, "🌱 Seeding demo database with delegation data",
		"checkpoint", m.demoCheckpoint,
		"chunkSize", m.cfg.ChunkSize,
		"timeout", m.seedTimeout)

	seedCtx, cancel := context.WithTimeout(ctx, m.seedTimeout)
	defer cancel()

	pool, err := pgxdb.NewConnection(seedCtx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Set the demo checkpoint for seeding (always overwrite for consistent seeding)
	if err := migrator.SetCheckpoint(seedCtx, pool, uint64(m.demoCheckpoint)); err != nil {
		return err
	}

	store, storeCloser := pgxstore.New(pool)
	defer storeCloser()

	httpClient := &http.Client{Timeout: m.cfg.HttpClientTimeout}
	client := tzkt.NewClient(httpClient, m.cfg.TzktAPIURL)

	service := scraper.NewService(
		client,
		store,
		scraper.WithChunkSize(m.cfg.ChunkSize),
		scraper.WithPollInterval(m.cfg.PollInterval),
	)

	events, done := service.Start(seedCtx)

	resultChan := make(chan error, 1)

	subscriberCloser := scraper.NewSubscriber(events,
		scraper.OnBackfillDone(func(e scraper.BackfillDone) {
			slog.InfoContext(seedCtx, "✅ Demo database seeding completed successfully")
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
