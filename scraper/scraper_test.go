package scraper_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/pkg/tzkt"
	"github.com/screwyprof/delegator/scraper"
)

// TestServiceBackfillBehavior tests core backfill business logic
func TestServiceBackfillBehavior(t *testing.T) {
	t.Parallel()

	t.Run("it fetches and stores delegations from API", func(t *testing.T) {
		t.Parallel()

		// Arrange
		expectedDelegations := []tzkt.Delegation{delegation(1), delegation(2)}
		server := apiWithDelegations(expectedDelegations...)
		defer server.Close()

		savedBatchesCh, store := storeCapturingBatches()
		svc := scraperWithChunkSize(1)(server, store)

		// Act
		done := runBackfillUntilComplete(t, svc)
		<-done

		// Assert
		assertDelegationsWereSaved(t, savedBatchesCh, expectedDelegations)
		assertCheckpointAdvancedTo(t, store, 2)
	})

	t.Run("it updates checkpoint after successful batch", func(t *testing.T) {
		t.Parallel()

		// Arrange
		expectedDelegation := delegation(5)
		server := apiWithDelegations(expectedDelegation)
		defer server.Close()

		_, store := storeCapturingBatches()
		svc := scraperWithChunkSize(1)(server, store)

		// Act
		done := runBackfillUntilComplete(t, svc)
		<-done

		// Assert
		assertCheckpointAdvancedTo(t, store, 5)
	})

	t.Run("it processes multiple batches sequentially", func(t *testing.T) {
		t.Parallel()

		// Arrange
		expectedDelegations := []tzkt.Delegation{delegation(1), delegation(2), delegation(3)}
		server := apiWithDelegations(expectedDelegations...)
		defer server.Close()

		savedBatchesCh, store := storeCapturingBatches()
		svc := scraperWithChunkSize(1)(server, store)

		// Act
		done := runBackfillUntilComplete(t, svc)
		<-done

		// Assert
		assertDelegationsWereSaved(t, savedBatchesCh, expectedDelegations)
		assertCheckpointAdvancedTo(t, store, 3)
	})

	t.Run("it handles API errors during backfill", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server := apiReturningError()
		defer server.Close()

		_, store := storeCapturingBatches()
		svc := scraperWithChunkSize(1)(server, store)

		// Act
		errorCh := runBackfillExpectingError(t, svc)

		// Assert
		assertBackfillFailedWithAPIError(t, errorCh)
	})
}

// TestServicePollingBehavior tests core polling business logic
func TestServicePollingBehavior(t *testing.T) {
	t.Parallel()

	t.Run("it polls at configured intervals after backfill", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server := apiWithPollingResponses(emptyPoll(), pollWithDelegation(1))
		defer server.Close()

		store := storeWithCheckpoint(0)
		clock, svc := clockControlledPolling(server, store)

		// Act
		cycles := runPollingCycles(t, svc, clock, 2)

		// Assert
		assertEmptyPollOccurred(t, cycles[0])
		assertPollFoundDelegations(t, cycles[1], 1)
	})

	t.Run("it continues from last checkpoint during polling", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server := apiWithPollingResponses(pollWithDelegation(6))
		defer server.Close()

		store := storeWithCheckpoint(5) // Start with checkpoint at 5
		clock, svc := clockControlledPolling(server, store)

		// Act
		cycles := runPollingCycles(t, svc, clock, 1)

		// Assert
		assertPollFoundDelegations(t, cycles[0], 1)
		assertCheckpointAdvancedTo(t, store, 6)
	})

	t.Run("it handles API errors during polling", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server := apiWithPollingErrors()
		defer server.Close()

		store := storeWithCheckpoint(0)
		clock, svc := clockControlledPolling(server, store)

		// Act
		errorCh := runPollingExpectingError(t, svc, clock)

		// Assert
		assertPollingFailedWithAPIError(t, errorCh)
	})
}

// TestServiceEventEmission tests observability and event emission
func TestServiceEventEmission(t *testing.T) {
	t.Parallel()

	t.Run("it emits backfill lifecycle events", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server := apiWithDelegations(delegation(1))
		defer server.Close()

		store := storeWithCheckpoint(0)
		svc := scraperWithChunkSize(1)(server, store)

		// Act
		events := runBackfillCapturingEvents(t, svc)

		// Assert
		assertBackfillStartedEvent(t, events.started)
		assertBackfillSyncCompletedEvents(t, events.syncCompleted, 1)
		assertBackfillDoneEvent(t, events.done, 1)
	})

	t.Run("it emits polling lifecycle events", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server := apiWithPollingResponses(pollWithDelegation(1))
		defer server.Close()

		store := storeWithCheckpoint(0)
		clock, svc := clockControlledPolling(server, store)

		// Act
		events := runPollingCapturingEvents(t, svc, clock)

		// Assert
		assertPollingStartedEvent(t, events.started)
		assertPollingCycleEvent(t, events.cycle, 1)
	})

	t.Run("it emits shutdown events", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server := apiWithPollingResponses()
		defer server.Close()

		store := storeWithCheckpoint(0)
		clock, svc := clockControlledPolling(server, store)

		// Act
		shutdown := runPollingCapturingShutdown(t, svc, clock)

		// Assert
		assertShutdownEventOccurred(t, shutdown)
	})
}

// Test data helpers

func createDelegationJSON(id int64, timestamp string, amount int64, address string, level int64) string {
	return fmt.Sprintf(`[{"id":%d,"timestamp":"%s","amount":%d,"sender":{"address":"%s"},"level":%d}]`,
		id, timestamp, amount, address, level)
}

func emptyResponse() string {
	return `[]`
}

func endOfBackfill() string {
	return emptyResponse()
}

func emptyPoll() string {
	return emptyResponse()
}

func pollWithDelegation(id int64) string {
	return createDelegationJSON(id, "2024-01-01T00:00:00Z", 1000000, "tz1abc", 100)
}

// Test setup helpers

func createTestClock() *fakeClock {
	return &fakeClock{tick: make(chan time.Time, 10)}
}

// Domain-specific test builders for expressing business scenarios

func apiWithDelegations(delegations ...tzkt.Delegation) *httptest.Server {
	responses := make([]string, 0, len(delegations)+1)
	for _, d := range delegations {
		responses = append(responses, fmt.Sprintf(`[{"id":%d,"timestamp":"%s","amount":%d,"sender":{"address":"%s"},"level":%d}]`,
			d.ID, d.Timestamp.Format(time.RFC3339), d.Amount, d.Sender.Address, d.Level))
	}
	responses = append(responses, endOfBackfill())
	return createTestServer(responses)
}

func apiWithPollingResponses(pollResponses ...string) *httptest.Server {
	responses := []string{endOfBackfill()}
	responses = append(responses, pollResponses...)
	return createTestServer(responses)
}

func apiWithPollingErrors() *httptest.Server {
	callCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call (backfill) succeeds with empty response
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(endOfBackfill()))
		} else {
			// Subsequent calls (polling) return errors
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "polling error"}`))
		}
	}))
}

func apiReturningError() *httptest.Server {
	return createErrorServer()
}

func delegation(id int64) tzkt.Delegation {
	timestampStr := fmt.Sprintf("2024-01-01T00:%02d:00Z", id)
	timestamp, _ := time.Parse(time.RFC3339, timestampStr)
	d := tzkt.Delegation{
		ID:        id,
		Timestamp: timestamp,
		Amount:    1000000 + id*100000,
		Level:     int64(100 + id),
	}
	d.Sender.Address = fmt.Sprintf("tz1%03d", id)
	return d
}

func storeCapturingBatches() (chan []scraper.Delegation, *mockStore) {
	savedBatchesCh := make(chan []scraper.Delegation, 10)
	store := createTestStore(0, func(ctx context.Context, batch []scraper.Delegation) error {
		savedBatchesCh <- batch
		return nil
	})
	return savedBatchesCh, store
}

func storeWithCheckpoint(checkpointID int64) *mockStore {
	return createTestStore(checkpointID, nil)
}

func scraperWithChunkSize(chunkSize uint64) func(*httptest.Server, *mockStore) *scraper.Service {
	return func(server *httptest.Server, store *mockStore) *scraper.Service {
		client := tzkt.NewClient(http.DefaultClient, server.URL)
		return scraper.NewService(client, store, scraper.WithChunkSize(chunkSize))
	}
}

func clockControlledPolling(server *httptest.Server, store *mockStore) (*fakeClock, *scraper.Service) {
	clock := createTestClock()
	client := tzkt.NewClient(http.DefaultClient, server.URL)
	svc := scraper.NewService(client, store,
		scraper.WithClock(clock),
		scraper.WithPollInterval(1*time.Millisecond),
		scraper.WithChunkSize(1),
	)
	return clock, svc
}

// Domain-specific assertions

func assertDelegationsWereSaved(t *testing.T, savedBatchesCh chan []scraper.Delegation, expected []tzkt.Delegation) {
	t.Helper()
	close(savedBatchesCh)

	var allSaved []scraper.Delegation
	for batch := range savedBatchesCh {
		allSaved = append(allSaved, batch...)
	}

	require.Len(t, allSaved, len(expected), "Expected %d delegations to be saved", len(expected))
	for i, expectedDel := range expected {
		assert.Equal(t, expectedDel.ID, allSaved[i].ID, "Delegation %d should have ID %d", i, expectedDel.ID)
		assert.Equal(t, expectedDel.Sender.Address, allSaved[i].Delegator, "Delegation %d should have delegator %s", i, expectedDel.Sender.Address)
		assert.Equal(t, expectedDel.Amount, allSaved[i].Amount, "Delegation %d should have amount %d", i, expectedDel.Amount)
		assert.Equal(t, expectedDel.Level, allSaved[i].Level, "Delegation %d should have level %d", i, expectedDel.Level)
	}
}

func assertCheckpointAdvancedTo(t *testing.T, store *mockStore, expectedID int64) {
	t.Helper()
	checkpoint, err := store.LastProcessedID(t.Context())
	require.NoError(t, err)
	assert.Equal(t, expectedID, checkpoint, "Checkpoint should advance to delegation ID %d", expectedID)
}

func assertEmptyPollOccurred(t *testing.T, cycle scraper.PollingSyncCompleted) {
	t.Helper()
	assert.Equal(t, 0, cycle.Fetched, "Expected empty poll to fetch 0 delegations")
}

func assertPollFoundDelegations(t *testing.T, cycle scraper.PollingSyncCompleted, expectedCount int) {
	t.Helper()
	assert.Equal(t, expectedCount, cycle.Fetched, "Expected poll to fetch %d delegations", expectedCount)
	assert.Greater(t, cycle.CheckpointID, int64(0), "Expected valid checkpoint ID")
}

func assertBackfillFailedWithAPIError(t *testing.T, errorCh <-chan error) {
	t.Helper()
	backfillError := <-errorCh
	require.NotNil(t, backfillError, "Expected backfill to fail with an error")
	assert.ErrorIs(t, backfillError, scraper.ErrAPIRequestFailed, "Error should be an API request failure")
}

func assertPollingFailedWithAPIError(t *testing.T, errorCh <-chan error) {
	t.Helper()
	pollingError := <-errorCh
	require.NotNil(t, pollingError, "Expected polling to fail with an error")
	assert.ErrorIs(t, pollingError, scraper.ErrAPIRequestFailed, "Error should be an API request failure")
}

func runBackfillUntilComplete(t *testing.T, svc *scraper.Service) <-chan struct{} {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())

	events, done := svc.Start(ctx)

	subCloser := scraper.NewSubscriber(events,
		scraper.OnBackfillDone(func(e scraper.BackfillDone) { cancel() }),
	)

	t.Cleanup(func() {
		subCloser()
		cancel()
	})

	return done
}

func runBackfillExpectingError(t *testing.T, svc *scraper.Service) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())

	events, done := svc.Start(ctx)
	errorCh := make(chan error, 1)

	subCloser := scraper.NewSubscriber(events,
		scraper.OnBackfillError(func(e scraper.BackfillError) {
			errorCh <- e.Err
			cancel()
		}),
	)

	t.Cleanup(func() {
		subCloser()
		cancel()
		<-done
	})

	return errorCh
}

func runPollingCycles(t *testing.T, svc *scraper.Service, clock *fakeClock, cycleCount int) []scraper.PollingSyncCompleted {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())

	events, done := svc.Start(ctx)

	pollCyclesCh := make(chan scraper.PollingSyncCompleted, 10)
	cyclesReceived := 0

	subCloser := scraper.NewSubscriber(events,
		scraper.OnPollingSyncCompleted(func(e scraper.PollingSyncCompleted) {
			pollCyclesCh <- e
			cyclesReceived++
			if cyclesReceived == cycleCount {
				close(pollCyclesCh)
				cancel()
			}
		}),
	)

	t.Cleanup(func() {
		subCloser()
		cancel()
		<-done
	})

	// Drive polling ticks
	for range cycleCount {
		clock.tick <- time.Now()
	}

	// Collect cycles from channel
	var cycles []scraper.PollingSyncCompleted
	for cycle := range pollCyclesCh {
		cycles = append(cycles, cycle)
	}

	return cycles
}

func runPollingExpectingError(t *testing.T, svc *scraper.Service, clock *fakeClock) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())

	events, done := svc.Start(ctx)
	errorCh := make(chan error, 1)

	subCloser := scraper.NewSubscriber(events,
		scraper.OnPollingError(func(e scraper.PollingError) {
			errorCh <- e.Err
			cancel()
		}),
	)

	t.Cleanup(func() {
		subCloser()
		cancel()
		<-done
	})

	// Drive polling tick to trigger error
	clock.tick <- time.Now()

	return errorCh
}

func createTestServer(responses []string) *httptest.Server {
	callCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if callCount < len(responses) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(responses[callCount]))
			callCount++
		} else {
			_, _ = w.Write([]byte(emptyResponse())) // Default to empty for extra calls
		}
	}))
}

func createErrorServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "server error"}`))
	}))
}

func createTestStore(lastID int64, onSave func(ctx context.Context, batch []scraper.Delegation) error) *mockStore {
	return &mockStore{
		lastID: lastID,
		onSave: onSave,
	}
}

// Mock implementations

// fakeClock implements Clock interface for deterministic testing
type fakeClock struct {
	tick chan time.Time
}

func (f *fakeClock) After(_ time.Duration) <-chan time.Time {
	return f.tick
}

func (f *fakeClock) Now() time.Time {
	return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
}

// mockStore implements Store interface for testing
type mockStore struct {
	lastID int64
	onSave func(ctx context.Context, batch []scraper.Delegation) error
}

func (m *mockStore) LastProcessedID(ctx context.Context) (int64, error) {
	return m.lastID, nil
}

func (m *mockStore) SaveBatch(ctx context.Context, batch []scraper.Delegation) error {
	if m.onSave != nil {
		err := m.onSave(ctx, batch)
		if err == nil && len(batch) > 0 {
			m.lastID = batch[len(batch)-1].ID
		}
		return err
	}

	if len(batch) == 0 {
		return nil
	}

	// simulate checkpoint update to highest ID in batch
	newCheckpoint := batch[len(batch)-1].ID
	m.lastID = newCheckpoint

	return nil
}

// Event capture types for testing

type capturedBackfillEvents struct {
	started       scraper.BackfillStarted
	syncCompleted []scraper.BackfillSyncCompleted
	done          scraper.BackfillDone
}

type capturedPollingEvents struct {
	started scraper.PollingStarted
	cycle   scraper.PollingSyncCompleted
}

func runBackfillCapturingEvents(t *testing.T, svc *scraper.Service) capturedBackfillEvents {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())

	events, done := svc.Start(ctx)

	backfillStartedCh := make(chan scraper.BackfillStarted, 1)
	backfillSyncCompletedCh := make(chan scraper.BackfillSyncCompleted, 10) // Buffer for multiple sync events
	backfillDoneCh := make(chan scraper.BackfillDone, 1)

	subCloser := scraper.NewSubscriber(events,
		scraper.OnBackfillStarted(func(e scraper.BackfillStarted) { backfillStartedCh <- e }),
		scraper.OnBackfillSyncCompleted(func(e scraper.BackfillSyncCompleted) { backfillSyncCompletedCh <- e }),
		scraper.OnBackfillDone(func(e scraper.BackfillDone) {
			backfillDoneCh <- e
			cancel()
		}),
	)

	t.Cleanup(func() {
		subCloser()
		cancel()
	})

	<-done

	// Collect all sync completed events
	close(backfillSyncCompletedCh)
	var syncCompleted []scraper.BackfillSyncCompleted
	for event := range backfillSyncCompletedCh {
		syncCompleted = append(syncCompleted, event)
	}

	return capturedBackfillEvents{
		started:       <-backfillStartedCh,
		syncCompleted: syncCompleted,
		done:          <-backfillDoneCh,
	}
}

func runPollingCapturingEvents(t *testing.T, svc *scraper.Service, clock *fakeClock) capturedPollingEvents {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())

	events, done := svc.Start(ctx)

	pollingStartedCh := make(chan scraper.PollingStarted, 1)
	pollingCycleCh := make(chan scraper.PollingSyncCompleted, 1)

	subCloser := scraper.NewSubscriber(events,
		scraper.OnPollingStarted(func(e scraper.PollingStarted) { pollingStartedCh <- e }),
		scraper.OnPollingSyncCompleted(func(e scraper.PollingSyncCompleted) {
			pollingCycleCh <- e
			cancel()
		}),
	)

	t.Cleanup(func() {
		subCloser()
		cancel()
		<-done
	})

	// Drive polling tick
	clock.tick <- time.Now()

	return capturedPollingEvents{
		started: <-pollingStartedCh,
		cycle:   <-pollingCycleCh,
	}
}

func runPollingCapturingShutdown(t *testing.T, svc *scraper.Service, clock *fakeClock) scraper.PollingShutdown {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())

	events, done := svc.Start(ctx)

	shutdownCh := make(chan scraper.PollingShutdown, 1)

	subCloser := scraper.NewSubscriber(events,
		scraper.OnPollingStarted(func(e scraper.PollingStarted) {
			// Once polling starts, cancel to trigger shutdown
			cancel()
		}),
		scraper.OnPollingShutdown(func(e scraper.PollingShutdown) {
			shutdownCh <- e
		}),
	)

	t.Cleanup(func() {
		subCloser()
	})

	<-done

	select {
	case shutdown := <-shutdownCh:
		return shutdown
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected shutdown event was not received")
		return scraper.PollingShutdown{} // unreachable
	}
}

func assertBackfillStartedEvent(t *testing.T, event scraper.BackfillStarted) {
	t.Helper()
	assert.False(t, event.StartedAt.IsZero(), "Backfill should have a valid start time")
	assert.GreaterOrEqual(t, event.CheckpointID, int64(0), "Backfill should have a valid starting checkpoint ID")
}

func assertBackfillSyncCompletedEvents(t *testing.T, events []scraper.BackfillSyncCompleted, expectedCount int) {
	t.Helper()
	assert.Len(t, events, expectedCount, "Should emit %d BackfillSyncCompleted events", expectedCount)

	for i, event := range events {
		assert.Greater(t, event.Fetched, 0, "Sync event %d should have fetched some records", i)
		assert.GreaterOrEqual(t, event.CheckpointID, int64(0), "Sync event %d should have a valid checkpoint ID", i)
		assert.Greater(t, event.ChunkSize, uint64(0), "Sync event %d should have a valid chunk size", i)
	}
}

func assertBackfillDoneEvent(t *testing.T, event scraper.BackfillDone, expectedTotalProcessed int64) {
	t.Helper()
	assert.Equal(t, expectedTotalProcessed, event.TotalProcessed, "Backfill should process %d delegations", expectedTotalProcessed)
	assert.True(t, event.Duration > 0, "Backfill duration should be positive")
}

func assertPollingStartedEvent(t *testing.T, event scraper.PollingStarted) {
	t.Helper()
	assert.Equal(t, 1*time.Millisecond, event.Interval, "Polling should start with configured interval")
}

func assertPollingCycleEvent(t *testing.T, event scraper.PollingSyncCompleted, expectedFetched int) {
	t.Helper()
	assert.Equal(t, expectedFetched, event.Fetched, "Polling cycle should fetch %d delegations", expectedFetched)
}

func assertShutdownEventOccurred(t *testing.T, shutdown scraper.PollingShutdown) {
	t.Helper()
	assert.ErrorIs(t, shutdown.Reason, context.Canceled, "Polling should shutdown due to context cancellation")
}
