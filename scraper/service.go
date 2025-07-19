package scraper

import (
	"context"
	"fmt"
	"time"

	"github.com/screwyprof/delegator/pkg/clock"
	"github.com/screwyprof/delegator/pkg/tzkt"
)

// Option configures the Service
// ------------------------------------------------
type Option func(*Service)

// WithClock injects a custom Clock (e.g., for testing)
func WithClock(c Clock) Option {
	return func(s *Service) { s.clock = c }
}

// WithPollInterval sets the polling interval
func WithPollInterval(d time.Duration) Option {
	return func(s *Service) { s.pollInterval = d }
}

// WithChunkSize sets the number of records per batch
func WithChunkSize(n uint64) Option {
	return func(s *Service) { s.chunkSize = n }
}

// Service implements two-phase scraping: backfill then live polling
// -----------------------------------------------------------------
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
		defer close(s.events)
		defer close(done)
		s.run(ctx)
	}()
	return s.events, done
}

// run orchestrates the backfill and polling, respecting context cancellation
// -------------------------------------------------------------------------
func (s *Service) run(ctx context.Context) {
	// Backfill
	start := s.clock.Now()

	// Get starting checkpoint ID for observability
	startingCheckpointID, err := s.store.LastProcessedID(ctx)
	if err != nil {
		s.events <- BackfillError{Err: fmt.Errorf("%w: %w", ErrCheckpointRetrieval, err)}
		return
	}

	s.events <- BackfillStarted{
		StartedAt:    start,
		CheckpointID: startingCheckpointID,
	}

	var total int64
	for {
		result, err := s.syncBatch(ctx, s.chunkSize)
		if err != nil {
			s.events <- BackfillError{Err: err}
			return
		}
		if result.Count == 0 {
			break
		}
		total += int64(result.Count)

		// Emit sync completed event for each batch
		s.events <- BackfillSyncCompleted{
			Fetched:      result.Count,
			CheckpointID: result.CheckpointID,
			ChunkSize:    s.chunkSize,
		}
	}

	stop := s.clock.Now().Sub(start)
	s.events <- BackfillDone{
		TotalProcessed: total,
		Duration:       stop,
	}

	// Polling
	s.events <- PollingStarted{Interval: s.pollInterval}
	for {
		select {
		case <-ctx.Done():
			s.events <- PollingShutdown{Reason: ctx.Err()}
			return
		case <-s.clock.After(s.pollInterval):
			result, err := s.syncBatch(ctx, s.chunkSize)
			if err != nil {
				s.events <- PollingError{Err: err}
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

// syncBatch fetches the next batch, saves it atomically, and returns sync result
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

// convertTzktDelegations converts API delegations to domain delegations
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
