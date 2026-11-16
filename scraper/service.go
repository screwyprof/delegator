package scraper

import (
	"context"
	"fmt"
	"time"

	"github.com/screwyprof/delegator/pkg/clock"
	"github.com/screwyprof/delegator/pkg/tzkt"
)

// Option configures the Service
// ------------------------------------------------.
type Option func(*Service)

// WithClock injects a custom Clock (e.g., for testing).
func WithClock(c Clock) Option {
	return func(s *Service) { s.clock = c }
}

// WithPollInterval sets the polling interval.
func WithPollInterval(d time.Duration) Option {
	return func(s *Service) { s.pollInterval = d }
}

// WithChunkSize sets the number of records per batch.
func WithChunkSize(n uint64) Option {
	return func(s *Service) { s.chunkSize = n }
}

// Service implements two-phase scraping: backfill then live polling
// -----------------------------------------------------------------.
type Service struct {
	api          Client
	store        Store
	clock        Clock
	pollInterval time.Duration
	chunkSize    uint64
	events       chan Event
}

// NewService constructs a Service with required dependencies and options
// ---------------------------------------------------------------------
// By default, it uses a real clock, 10s poll interval, and 500 chunk size.
func NewService(api Client, store Store, opts ...Option) *Service {
	s := &Service{
		api:          api,
		store:        store,
		clock:        clock.SystemClock{},
		pollInterval: DefaultPollInterval,
		chunkSize:    DefaultChunkSize,
		events:       make(chan Event, 10),
	}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Start launches the scraper and returns the events channel and done channel.
//
// Shutdown pattern:
//  1. Cancel context to request shutdown: cancel()
//  2. Service stops producing events and closes events channel
//  3. Wait for complete shutdown: <-done
//
// Example:
//
//	events, done := service.Start(ctx)
//	defer func() {
//	  cancel()    // 1. Request shutdown
//	  <-done      // 2. Wait for complete shutdown
//	}()
//
// The context signals when to stop, the done channel confirms when stopped.
func (s *Service) Start(ctx context.Context) (<-chan Event, <-chan struct{}) {
	done := make(chan struct{})

	go func() {
		// Deferred calls run in reverse: events closes first, so by the time done closes, every
		// event has been sent and a subscriber can finish draining them.
		defer close(done)
		defer close(s.events)

		s.run(ctx)
	}()

	return s.events, done
}

// run backfills up to the newest delegation, then polls for new ones until ctx is cancelled.
// Neither phase gives up on an error: each is reported and retried after one poll interval,
// resuming from the checkpoint, so the service only stops when ctx is cancelled.
func (s *Service) run(ctx context.Context) {
	if !s.backfill(ctx) {
		return
	}

	s.poll(ctx)
}

// backfill syncs batch after batch until one comes back empty, reporting progress as events.
// A failed step is reported as a BackfillError and retried; it returns false only when ctx is
// cancelled, which is a shutdown rather than an error and so is not reported as one.
func (s *Service) backfill(ctx context.Context) bool {
	start := s.clock.Now()

	startingCheckpointID, ok := s.startingCheckpoint(ctx)
	if !ok {
		return false
	}

	s.events <- BackfillStarted{
		StartedAt:    start,
		CheckpointID: startingCheckpointID,
	}

	var total int64

	for {
		result, err := s.syncBatch(ctx, s.chunkSize)
		if err != nil {
			if !s.reportAndWait(ctx, BackfillError{Err: err}) {
				return false
			}

			continue
		}

		if result.Count == 0 {
			break
		}

		total += int64(result.Count)

		s.events <- BackfillSyncCompleted{
			Fetched:      result.Count,
			CheckpointID: result.CheckpointID,
			ChunkSize:    s.chunkSize,
		}
	}

	s.events <- BackfillDone{
		TotalProcessed: total,
		Duration:       s.clock.Now().Sub(start),
	}

	return true
}

// startingCheckpoint reads the checkpoint the backfill starts from, retrying failures like any
// other backfill step. It reports false only when ctx is cancelled.
func (s *Service) startingCheckpoint(ctx context.Context) (int64, bool) {
	for {
		id, err := s.store.LastProcessedID(ctx)
		if err == nil {
			return id, true
		}

		if !s.reportAndWait(ctx, BackfillError{Err: fmt.Errorf("%w: %w", ErrCheckpointRetrieval, err)}) {
			return 0, false
		}
	}
}

// reportAndWait emits event for a failed step, then waits one poll interval before the caller
// retries. It reports false, emitting nothing, if ctx is cancelled: an error caused by shutdown
// is not a failure.
func (s *Service) reportAndWait(ctx context.Context, event Event) bool {
	if ctx.Err() != nil {
		return false
	}

	s.events <- event

	select {
	case <-ctx.Done():
		return false
	case <-s.clock.After(s.pollInterval):
		return true
	}
}

// poll syncs one batch per interval until ctx is cancelled. A failed cycle is reported and
// retried on the next tick rather than ending the service.
func (s *Service) poll(ctx context.Context) {
	s.events <- PollingStarted{Interval: s.pollInterval}

	for {
		select {
		case <-ctx.Done():
			s.events <- PollingShutdown{Reason: ctx.Err()}

			return
		case <-s.clock.After(s.pollInterval):
			result, err := s.syncBatch(ctx, s.chunkSize)
			if err != nil {
				// A cycle cut off by shutdown isn't a failure; the next loop sees ctx.Done.
				if ctx.Err() == nil {
					s.events <- PollingError{Err: err}
				}

				continue
			}

			// Always emit polling sync completed event
			s.events <- PollingSyncCompleted{
				Fetched:      result.Count,
				CheckpointID: result.CheckpointID,
				ChunkSize:    s.chunkSize,
			}
		}
	}
}

// syncBatch fetches the next batch, saves it atomically, and returns sync result.
func (s *Service) syncBatch(ctx context.Context, chunkSize uint64) (SyncResult, error) {
	// respect cancellation
	select {
	case <-ctx.Done():
		return SyncResult{}, ctx.Err()
	default:
	}

	// load checkpoint
	checkpointID, err := s.store.LastProcessedID(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %w", ErrCheckpointRetrieval, err)
	}

	// fetch using checkpoint
	req := tzkt.DelegationsRequest{
		Limit:         chunkSize,
		IDGreaterThan: &checkpointID,
	}

	batch, err := s.api.GetDelegations(ctx, req)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %w", ErrAPIRequestFailed, err)
	}

	if len(batch) == 0 {
		return SyncResult{Count: 0, CheckpointID: checkpointID}, nil
	}

	// Convert API delegations to domain delegations
	domainDelegations := convertTzktDelegations(batch)

	// save batch; store updates checkpoint internally
	err = s.store.SaveBatch(ctx, domainDelegations)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %w", ErrSaveBatchFailed, err)
	}

	// Return the count and new checkpoint ID (highest ID in the batch)
	newCheckpointID := domainDelegations[len(domainDelegations)-1].ID

	return SyncResult{
		Count:        len(batch),
		CheckpointID: newCheckpointID,
	}, nil
}

// convertTzktDelegations converts API delegations to domain delegations.
func convertTzktDelegations(tzktDelegations []tzkt.Delegation) []Delegation {
	delegations := make([]Delegation, len(tzktDelegations))

	for i, tzktDel := range tzktDelegations {
		delegations[i] = Delegation{
			ID:        tzktDel.ID,
			Level:     tzktDel.Level,
			Timestamp: tzktDel.Timestamp,
			Delegator: tzktDel.Sender.Address,
			Amount:    tzktDel.Amount,
		}
	}

	return delegations
}
