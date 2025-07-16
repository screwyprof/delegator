package scraper

import (
	"context"
	"errors"
	"time"

	"github.com/screwyprof/delegator/pkg/tzkt"
)

// Sentinel errors for failure cases
var (
	ErrCheckpointRetrieval = errors.New("checkpoint retrieval failed")
	ErrAPIRequestFailed    = errors.New("API request failed")
	ErrSaveBatchFailed     = errors.New("save batch failed")
	ErrConversionFailed    = errors.New("delegation conversion failed")
	ErrInvalidTimestamp    = errors.New("invalid delegation timestamp")
)

// Default configuration values
const (
	DefaultChunkSize    = uint64(10000)
	DefaultPollInterval = 10 * time.Second
)

// Client fetches delegations from the API
// ---------------------------------------
type Client interface {
	GetDelegations(ctx context.Context, req tzkt.DelegationsRequest) ([]tzkt.Delegation, error)
}

// Store provides persistence operations for delegation data
type Store interface {
	// LastProcessedID returns the ID of the last processed delegation
	LastProcessedID(ctx context.Context) (int64, error)
	// SaveBatch saves a batch of delegations. It update the last checkpoint.
	SaveBatch(ctx context.Context, delegations []Delegation) error
}

// SyncResult contains the results of a sync batch operation
type SyncResult struct {
	Count        int
	CheckpointID int64
}

// Clock abstracts time for production and testing
// ------------------------------------------------
type Clock interface {
	After(d time.Duration) <-chan time.Time
	Now() time.Time
}

// Event represents a service lifecycle event
// ------------------------------------------
type Event any

type BackfillDone struct {
	TotalProcessed int64
	Duration       time.Duration
}

type BackfillStarted struct {
	StartedAt    time.Time
	CheckpointID int64
}

type BackfillSyncCompleted struct {
	Fetched      int
	CheckpointID int64
	ChunkSize    uint64
}

type BackfillError struct {
	Err error
}

type PollingSyncCompleted struct {
	Fetched      int
	CheckpointID int64
	ChunkSize    uint64
}

type PollingStarted struct {
	Interval time.Duration
}

type PollingShutdown struct {
	Reason error // Why shutdown occurred (ctx.Err())
}

type PollingError struct {
	Err error
}
