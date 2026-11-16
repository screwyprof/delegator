// Package scraper backfills Tezos delegations from TzKT into a store and then polls for new
// ones, reporting progress as a stream of events.
package scraper

import (
	"context"
	"errors"
	"time"

	"github.com/screwyprof/delegator/pkg/tzkt"
)

// Sentinel errors for failure cases.
var (
	ErrCheckpointRetrieval = errors.New("checkpoint retrieval failed")
	ErrAPIRequestFailed    = errors.New("API request failed")
	ErrSaveBatchFailed     = errors.New("save batch failed")
)

// Default configuration values.
const (
	DefaultChunkSize    = uint64(10000)
	DefaultPollInterval = 10 * time.Second
)

// Client fetches delegations from the API
// ---------------------------------------.
type Client interface {
	GetDelegations(ctx context.Context, req tzkt.DelegationsRequest) ([]tzkt.Delegation, error)
}

// Store provides persistence operations for delegation data.
type Store interface {
	// LastProcessedID returns the ID of the last processed delegation
	LastProcessedID(ctx context.Context) (int64, error)
	// SaveBatch saves a batch of delegations. It update the last checkpoint.
	SaveBatch(ctx context.Context, delegations []Delegation) error
}

// SyncResult contains the results of a sync batch operation.
type SyncResult struct {
	Count        int
	CheckpointID int64
}

// Clock abstracts time for production and testing
// ------------------------------------------------.
type Clock interface {
	After(d time.Duration) <-chan time.Time
	Now() time.Time
}

// Event is one of the events below, describing the service's progress. Subscribe with NewSubscriber.
type Event any

// BackfillDone reports the backfill has caught up with TzKT; polling starts next.
type BackfillDone struct {
	TotalProcessed int64
	Duration       time.Duration
}

// BackfillStarted reports the backfill beginning from CheckpointID, the last stored delegation.
type BackfillStarted struct {
	StartedAt    time.Time
	CheckpointID int64
}

// BackfillSyncCompleted reports one backfill batch stored, and the checkpoint it advanced to.
type BackfillSyncCompleted struct {
	Fetched      int
	CheckpointID int64
	ChunkSize    uint64
}

// BackfillError reports a failed backfill step; it is retried from the checkpoint after one poll
// interval, and polling starts only once the backfill has caught up.
type BackfillError struct {
	Err error
}

// PollingSyncCompleted reports one polling cycle, including cycles that found nothing new.
type PollingSyncCompleted struct {
	Fetched      int
	CheckpointID int64
	ChunkSize    uint64
}

// PollingStarted reports polling beginning, one cycle per Interval.
type PollingStarted struct {
	Interval time.Duration
}

// PollingShutdown reports polling stopped because the service's context was cancelled.
type PollingShutdown struct {
	Reason error // Why shutdown occurred (ctx.Err())
}

// PollingError reports a failed polling cycle; the service retries on the next interval.
type PollingError struct {
	Err error
}
