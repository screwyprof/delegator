package pgxstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/screwyprof/delegator/web/store/dbrow"
	"github.com/screwyprof/delegator/web/tezos"
)

// Sentinel errors for store operations
var (
	ErrQueryFailed = errors.New("delegation query failed")
)

// DelegationsFinder implements delegation querying using pgx
type DelegationsFinder struct {
	pool *pgxpool.Pool
}

// New creates a new PostgreSQL delegations finder with an existing connection pool
// Returns the finder and a closer function
func New(pool *pgxpool.Pool) (*DelegationsFinder, func()) {
	finder := &DelegationsFinder{pool: pool}
	closer := func() {
		pool.Close()
	}
	return finder, closer
}

// FindDelegations queries delegations based on the provided criteria
// Uses LIMIT n+1 technique for efficient pagination without separate count query
func (f *DelegationsFinder) FindDelegations(ctx context.Context, criteria tezos.DelegationsCriteria) (*tezos.DelegationsPage, error) {
	query, args := f.buildQuery(criteria)

	rows, err := f.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrQueryFailed, err)
	}
	defer rows.Close()

	var delegations []tezos.Delegation
	for rows.Next() {
		var dbRow dbrow.Delegation
		err := rows.Scan(&dbRow.ID, &dbRow.Timestamp, &dbRow.Amount, &dbRow.Delegator, &dbRow.Level)
		if err != nil {
			return nil, fmt.Errorf("%w: scan failed: %w", ErrQueryFailed, err)
		}

		// Convert database row to domain model
		delegation := tezos.Delegation{
			ID:        dbRow.ID,
			Timestamp: dbRow.Timestamp,
			Amount:    dbRow.Amount,
			Delegator: dbRow.Delegator,
			Level:     dbRow.Level,
		}
		delegations = append(delegations, delegation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrQueryFailed, err)
	}

	// Determine if there are more pages using LIMIT n+1 technique
	hasMore := len(delegations) > int(criteria.Size)
	if hasMore {
		// Remove the extra record we requested to detect "has more"
		delegations = delegations[:criteria.Size]
	}

	return &tezos.DelegationsPage{
		Delegations: delegations,
		HasMore:     hasMore,
		Number:      criteria.Page,
		Size:        criteria.Size,
	}, nil
}

// buildQuery constructs the SQL query and arguments based on the criteria
// Uses LIMIT pageSize+1 to efficiently detect if there are more pages
func (f *DelegationsFinder) buildQuery(criteria tezos.DelegationsCriteria) (string, []any) {
	var conditions []string
	var args []any
	argCount := 0

	baseQuery := "SELECT id, timestamp, amount, delegator, level FROM delegations"

	// Add year filter if specified (0 means no year filtering)
	if criteria.Year > 0 {
		argCount++
		conditions = append(conditions, fmt.Sprintf("year = $%d", argCount))
		args = append(args, criteria.Year)
	}

	// Build WHERE clause if we have conditions
	query := baseQuery
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Add ordering
	query += " ORDER BY timestamp DESC"

	// Calculate LIMIT and OFFSET from page-based criteria
	limit := criteria.Size + 1 // Request one extra to detect "has more"
	offset := (criteria.Page - 1) * criteria.Size

	// Add LIMIT (always present for pagination)
	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, limit)

	// Add OFFSET (if not first page)
	if offset > 0 {
		argCount++
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, offset)
	}

	return query, args
}
