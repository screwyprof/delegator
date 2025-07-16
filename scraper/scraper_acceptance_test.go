////go:build acceptance

package scraper_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/cmd/scraper/config"
	"github.com/screwyprof/delegator/pkg/pgxdb"
	"github.com/screwyprof/delegator/pkg/pgxdb/pgxdbtest"
	"github.com/screwyprof/delegator/pkg/tzkt"
	"github.com/screwyprof/delegator/scraper"
	"github.com/screwyprof/delegator/scraper/store/pgxstore"
)

const (
	// lastCheckpoint is a historical checkpoint for acceptance tests (recent data only)
	lastCheckpoint = int64(1939557726552064)
	// chunkSize is a smaller chunk size for acceptance tests
	chunkSize = uint64(1000)
)

// TestScraperAcceptanceBehavior tests end-to-end scraper functionality with real PostgreSQL and Tezos API
func TestScraperAcceptanceBehavior(t *testing.T) {
	t.Parallel()

	t.Run("it processes delegations from historical checkpoint and stores them in database", func(t *testing.T) {
		t.Parallel()

		// Arrange
		testDB, dbURL := pgxdbtest.CreateTestDatabase(t, "../migrations")
		defer testDB.Close()

		store, storeCloser := createConnection(t, dbURL)
		defer storeCloser()

		// Create config for test
		cfg := createTestConfig()

		// Initialize the checkpoint in database
		pgxdbtest.InitializeCheckpoint(t, testDB, int64(cfg.InitialCheckpoint))

		httpClient := &http.Client{Timeout: cfg.HttpClientTimeout}
		client := tzkt.NewClient(httpClient, cfg.TzktAPIURL)

		service := createTestService(t, client, store, cfg)

		// Act
		backfillResult := runScraperUntilPollingStarts(t, service)

		// Assert
		assertBackfillSucceeded(t, backfillResult)
		assertDataWasStoredCorrectly(t, testDB, store)(backfillResult, int64(cfg.InitialCheckpoint))
	})
}

// runScraperUntilPollingStarts executes the scraper and returns backfill results
func runScraperUntilPollingStarts(t *testing.T, service *scraper.Service) scraper.BackfillDone {
	t.Helper()

	// Create cancellable context for service
	ctx, cancel := context.WithCancel(t.Context())

	// Start service (returns immediately, runs in background goroutine)
	events, done := service.Start(ctx)

	// Capture backfill result for assertions
	var backfillDone scraper.BackfillDone

	// Subscribe to events and cancel when we reach polling phase
	closer := scraper.NewSubscriber(events,
		scraper.OnBackfillDone(func(e scraper.BackfillDone) {
			backfillDone = e
			t.Logf("Backfill completed: %d delegations in %v", e.TotalProcessed, e.Duration)
		}),
		scraper.OnPollingStarted(func(e scraper.PollingStarted) {
			t.Logf("Polling started with interval: %v - canceling service", e.Interval)
			cancel()
		}),
		scraper.OnPollingSyncCompleted(func(e scraper.PollingSyncCompleted) {
			t.Logf("Polling cycle: %d delegations fetched, checkpoint: %d", e.Fetched, e.CheckpointID)
		}),
		scraper.OnPollingShutdown(func(e scraper.PollingShutdown) {
			t.Logf("Polling shutdown: %v", e.Reason)
		}),
	)
	t.Cleanup(closer)

	// Wait for clean shutdown
	select {
	case <-done:
		t.Log("Service shut down cleanly")
	case <-time.After(5 * time.Second):
		t.Fatal("Service did not shut down within timeout")
	}

	return backfillDone
}

// assertBackfillSucceeded verifies the backfill process completed successfully
func assertBackfillSucceeded(t *testing.T, backfillResult scraper.BackfillDone) {
	t.Helper()

	assert.GreaterOrEqual(t, backfillResult.TotalProcessed, int64(0), "Backfill should process zero or more delegations")
	assert.Positive(t, backfillResult.Duration, "Backfill should take measurable time")
}

// assertDataWasStoredCorrectly returns a verification function that captures dependencies
func assertDataWasStoredCorrectly(t *testing.T, testDB *pgxpool.Pool, store *pgxstore.Store) func(backfillResult scraper.BackfillDone, startCheckpoint int64) {
	return func(backfillResult scraper.BackfillDone, startCheckpoint int64) {
		t.Helper()

		ctx := t.Context()
		backfillCount := backfillResult.TotalProcessed

		assertDatabaseCountMatchesBackfill(t, testDB, ctx, backfillCount)

		if backfillCount == 0 {
			t.Error("No delegations were processed - this may indicate checkpoint is too far in future or doesn't exist")
			return
		}

		assertCheckpointAdvanced(t, store, ctx, startCheckpoint)
		assertFirstDelegationAfterCheckpoint(t, testDB, ctx, startCheckpoint)
		assertLastDelegationMatchesCheckpoint(t, testDB, store, ctx)
		assertTimestampsAreValid(t, testDB, ctx)
	}
}

// assertDatabaseCountMatchesBackfill verifies database count matches event count
func assertDatabaseCountMatchesBackfill(t *testing.T, testDB *pgxpool.Pool, ctx context.Context, backfillCount int64) {
	t.Helper()

	var actualCount int64
	err := testDB.QueryRow(ctx, "SELECT COUNT(*) FROM delegations").Scan(&actualCount)
	require.NoError(t, err)
	assert.Equal(t, backfillCount, actualCount, "Database count should match BackfillDone event count")
}

// assertCheckpointAdvanced verifies the checkpoint was updated beyond the starting point
func assertCheckpointAdvanced(t *testing.T, store *pgxstore.Store, ctx context.Context, startCheckpoint int64) {
	t.Helper()

	finalCheckpoint, err := store.LastProcessedID(ctx)
	require.NoError(t, err)
	assert.Greater(t, finalCheckpoint, startCheckpoint, "Checkpoint should have advanced beyond starting point")
}

// assertFirstDelegationAfterCheckpoint verifies first stored delegation starts after checkpoint
func assertFirstDelegationAfterCheckpoint(t *testing.T, testDB *pgxpool.Pool, ctx context.Context, startCheckpoint int64) {
	t.Helper()

	var firstID int64
	err := testDB.QueryRow(ctx, "SELECT id FROM delegations ORDER BY id ASC LIMIT 1").Scan(&firstID)
	require.NoError(t, err)
	assert.Greater(t, firstID, startCheckpoint, "First stored delegation should be after checkpoint")
}

// assertLastDelegationMatchesCheckpoint verifies last delegation ID matches final checkpoint
func assertLastDelegationMatchesCheckpoint(t *testing.T, testDB *pgxpool.Pool, store *pgxstore.Store, ctx context.Context) {
	t.Helper()

	var lastID int64
	err := testDB.QueryRow(ctx, "SELECT id FROM delegations ORDER BY id DESC LIMIT 1").Scan(&lastID)
	require.NoError(t, err)

	finalCheckpoint, err := store.LastProcessedID(ctx)
	require.NoError(t, err)

	assert.Equal(t, lastID, finalCheckpoint, "Last delegation ID should match final checkpoint")
}

// assertTimestampsAreValid verifies stored timestamps are valid
func assertTimestampsAreValid(t *testing.T, testDB *pgxpool.Pool, ctx context.Context) {
	t.Helper()

	var firstTimestamp, lastTimestamp time.Time

	err := testDB.QueryRow(ctx, "SELECT timestamp FROM delegations ORDER BY id ASC LIMIT 1").Scan(&firstTimestamp)
	require.NoError(t, err)

	err = testDB.QueryRow(ctx, "SELECT timestamp FROM delegations ORDER BY id DESC LIMIT 1").Scan(&lastTimestamp)
	require.NoError(t, err)

	assert.False(t, firstTimestamp.IsZero(), "First timestamp should not be zero")
	assert.False(t, lastTimestamp.IsZero(), "Last timestamp should not be zero")
}

// createTestConfig creates configuration optimized for testing
func createTestConfig() config.Config {
	cfg := config.New()
	cfg.InitialCheckpoint = uint64(lastCheckpoint)
	cfg.ChunkSize = chunkSize

	return cfg
}

// createTestService creates a scraper service with test configuration
func createTestService(t *testing.T, client *tzkt.Client, store *pgxstore.Store, cfg config.Config) *scraper.Service {
	t.Helper()

	return scraper.NewService(
		client,
		store,
		scraper.WithChunkSize(cfg.ChunkSize),
		scraper.WithPollInterval(cfg.PollInterval),
	)
}

// createConnection creates a store connection to the test database
func createConnection(t *testing.T, dbURL string) (*pgxstore.Store, func()) {
	t.Helper()

	pool, err := pgxdb.NewConnection(t.Context(), dbURL)
	require.NoError(t, err)

	store, closer := pgxstore.New(pool)
	return store, closer
}
