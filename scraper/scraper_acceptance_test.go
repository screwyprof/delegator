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

	"github.com/screwyprof/delegator/migrator/migratortest"
	"github.com/screwyprof/delegator/pkg/pgxdb"
	"github.com/screwyprof/delegator/pkg/tzkt"
	"github.com/screwyprof/delegator/scraper"
	"github.com/screwyprof/delegator/scraper/store/pgxstore"
	"github.com/screwyprof/delegator/scraper/testcfg"
)

// TestScraperAcceptanceBehavior tests end-to-end scraper functionality with real PostgreSQL and Tezos API
//
// Configuration:
//   - Uses testcfg.New() for test-optimized parameters:
//     ChunkSize=1000 (vs 10000 in production) for faster tests
//     PollInterval=100ms (vs 10s in production) for faster tests
//   - Override via environment variables if needed (SCRAPER_TEST_CHUNK_SIZE, etc.)
func TestScraperAcceptanceBehavior(t *testing.T) {
	t.Parallel()

	t.Run("it successfully fetches and stores delegations", func(t *testing.T) {
		t.Parallel()

		// Arrange
		// Load test configuration (ALL test-optimized parameters)
		testCfg := testcfg.New()

		// Create test database with schema + checkpoint (migrator concern)
		testDB := migratortest.CreateScraperTestDatabase(t, "../migrator/migrations", uint64(testCfg.Checkpoint))
		defer testDB.Close()

		// Create separate connection for production code (connection isolation)
		productionDB, err := pgxdb.NewConnection(t.Context(), testDB.Config().ConnString())
		require.NoError(t, err)
		defer productionDB.Close()

		// Production code uses its own connection
		store, storeCloser := pgxstore.New(productionDB)
		defer storeCloser()

		httpClient := &http.Client{Timeout: testCfg.HttpClientTimeout}
		client := tzkt.NewClient(httpClient, testCfg.TzktAPIURL)

		service := createTestService(t, client, store, testCfg)

		// Act
		backfillResult := runScraperUntilPollingStarts(t, service, testCfg.ShutdownTimeout)

		// Assert
		assertBackfillSucceeded(t, backfillResult)
		// Test assertions use separate connection for isolation
		assertDataWasStoredCorrectly(t, testDB)(backfillResult, testCfg.Checkpoint)
	})
}

// runScraperUntilPollingStarts executes the scraper and returns backfill results
func runScraperUntilPollingStarts(t *testing.T, service *scraper.Service, shutdownTimeout time.Duration) scraper.BackfillDone {
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
	case <-time.After(shutdownTimeout):
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
func assertDataWasStoredCorrectly(t *testing.T, testDB *pgxpool.Pool) func(scraper.BackfillDone, int64) {
	t.Helper()

	return func(backfillResult scraper.BackfillDone, startCheckpoint int64) {
		ctx := t.Context()

		if backfillResult.TotalProcessed == 0 {
			t.Error("No delegations were processed - this may indicate checkpoint is too far in future or doesn't exist")
			return
		}

		assertDatabaseCountMatchesBackfill(t, testDB, ctx, backfillResult.TotalProcessed)
		assertCheckpointAdvanced(t, testDB, ctx, startCheckpoint)
		assertFirstDelegationAfterCheckpoint(t, testDB, ctx, startCheckpoint)
		assertLastDelegationMatchesCheckpoint(t, testDB, ctx)
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
func assertCheckpointAdvanced(t *testing.T, testDB *pgxpool.Pool, ctx context.Context, startCheckpoint int64) {
	t.Helper()

	var finalCheckpoint int64
	err := testDB.QueryRow(ctx, "SELECT COALESCE(last_id, 0) FROM scraper_checkpoint").Scan(&finalCheckpoint)
	require.NoError(t, err)
	assert.Greater(t, finalCheckpoint, startCheckpoint, "Checkpoint should have advanced beyond starting point")
}

// assertFirstDelegationAfterCheckpoint verifies first delegation ID is greater than checkpoint
func assertFirstDelegationAfterCheckpoint(t *testing.T, testDB *pgxpool.Pool, ctx context.Context, startCheckpoint int64) {
	t.Helper()

	var firstID int64
	err := testDB.QueryRow(ctx, "SELECT id FROM delegations ORDER BY id ASC LIMIT 1").Scan(&firstID)
	require.NoError(t, err)
	assert.Greater(t, firstID, startCheckpoint, "First stored delegation should be after checkpoint")
}

// assertLastDelegationMatchesCheckpoint verifies last delegation ID matches final checkpoint
func assertLastDelegationMatchesCheckpoint(t *testing.T, testDB *pgxpool.Pool, ctx context.Context) {
	t.Helper()

	var lastID int64
	err := testDB.QueryRow(ctx, "SELECT id FROM delegations ORDER BY id DESC LIMIT 1").Scan(&lastID)
	require.NoError(t, err)

	var finalCheckpoint int64
	err = testDB.QueryRow(ctx, "SELECT COALESCE(last_id, 0) FROM scraper_checkpoint").Scan(&finalCheckpoint)
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

// createTestService creates a scraper service with test-optimized configuration
func createTestService(t *testing.T, client *tzkt.Client, store *pgxstore.Store, testCfg testcfg.Config) *scraper.Service {
	t.Helper()

	return scraper.NewService(
		client,
		store,
		scraper.WithChunkSize(testCfg.ChunkSize),
		scraper.WithPollInterval(testCfg.PollInterval),
	)
}
