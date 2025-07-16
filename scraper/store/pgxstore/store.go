package pgxstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/screwyprof/delegator/scraper"
	"github.com/screwyprof/delegator/scraper/store/dbrow"
)

// Sentinel errors for store operations
var (
	ErrTransactionFailed     = errors.New("transaction failed")
	ErrTempTableFailed       = errors.New("temporary table operation failed")
	ErrCopyFailed            = errors.New("bulk copy operation failed")
	ErrInsertFailed          = errors.New("insert operation failed")
	ErrCheckpointFailed      = errors.New("checkpoint update failed")
	ErrLastProcessedIDFailed = errors.New("failed to get last processed ID")
)

// Store implements scraper.Store interface using pgx
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new PostgreSQL store with an existing connection pool
// Returns the store and a closer function
func New(pool *pgxpool.Pool) (*Store, func()) {
	store := &Store{pool: pool}
	closer := func() {
		pool.Close()
	}
	return store, closer
}

// LastProcessedID returns the last processed delegation ID (checkpoint)
func (s *Store) LastProcessedID(ctx context.Context) (int64, error) {
	var lastID int64
	err := s.pool.QueryRow(ctx, "SELECT COALESCE(last_id, 0) FROM scraper_checkpoint").Scan(&lastID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrLastProcessedIDFailed, err)
	}
	return lastID, nil
}

// SaveBatch saves a batch of delegations using pgx CopyFrom for maximum performance
// Uses a temporary table approach to handle duplicate detection efficiently
func (s *Store) SaveBatch(ctx context.Context, delegations []scraper.Delegation) error {
	if len(delegations) == 0 {
		return nil
	}

	// Convert scraper.Delegation to [][]any format for pgx.CopyFromRows
	rows := dbrow.ScraperDelegationsToRows(delegations)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionFailed, err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // No-op if commit succeeds

	// Create temporary table for bulk insert
	_, err = tx.Exec(ctx, `
		CREATE TEMPORARY TABLE temp_delegations (
			id BIGINT,
			timestamp TIMESTAMP WITH TIME ZONE,
			amount BIGINT,
			delegator TEXT,
			level BIGINT
		) ON COMMIT DROP
	`)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTempTableFailed, err)
	}

	// Use CopyFrom for extremely fast bulk insert into temporary table
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"temp_delegations"},
		[]string{"id", "timestamp", "amount", "delegator", "level"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCopyFailed, err)
	}

	// Insert from temporary table to main table with conflict resolution
	// created_at will be populated by database DEFAULT CURRENT_TIMESTAMP
	_, err = tx.Exec(ctx, `
		INSERT INTO delegations (id, timestamp, amount, delegator, level)
		SELECT id, timestamp, amount, delegator, level
		FROM temp_delegations
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInsertFailed, err)
	}

	// Since delegations are sorted by ID, the last one has the highest ID
	checkpointID := delegations[len(delegations)-1].ID

	// Update checkpoint (singleton table with proper upsert)
	_, err = tx.Exec(ctx, `
		INSERT INTO scraper_checkpoint (single_row, last_id) VALUES (TRUE, $1) 
		ON CONFLICT (single_row) DO UPDATE SET last_id = $1
	`, checkpointID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCheckpointFailed, err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionFailed, err)
	}

	return nil
}
