// Package pgxstore is the web API's PostgreSQL store: it finds pages of delegations by year
// and cursor.
package pgxstore

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
	pgxc "github.com/zolstein/pgx-collect"

	"github.com/screwyprof/delegator/web/store/dbrow"
	"github.com/screwyprof/delegator/web/tezos"
)

// Sentinel errors for store operations.
var (
	ErrQueryFailed = errors.New("delegation query failed")
)

// DelegationsFinder implements delegation querying using pgx.
type DelegationsFinder struct {
	pool *pgxpool.Pool
}

// New creates a PostgreSQL delegations finder over pool. The caller owns pool and closes it.
func New(pool *pgxpool.Pool) *DelegationsFinder {
	return &DelegationsFinder{pool: pool}
}

// FindDelegations queries delegations based on the provided criteria
// Uses LIMIT n+1 technique for efficient pagination without separate count query.
func (f *DelegationsFinder) FindDelegations(
	ctx context.Context,
	criteria tezos.DelegationsCriteria,
) (*tezos.DelegationsPage, error) {
	query, args := NewDelegationsQuery().
		ForCriteria(criteria).
		Build()

	rows, err := f.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrQueryFailed, err)
	}
	defer rows.Close()

	// Use pgx-collect for efficient row collection
	dbDelegations, err := pgxc.CollectRows(rows, pgxc.RowToStructByName[dbrow.Delegation])
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrQueryFailed, err)
	}

	// Convert database rows to domain models
	delegations := make([]tezos.Delegation, 0, len(dbDelegations))
	for _, dbRow := range dbDelegations {
		delegation := tezos.Delegation{
			ID:        dbRow.ID,
			Timestamp: dbRow.Timestamp,
			Amount:    dbRow.Amount,
			Delegator: dbRow.Delegator,
			Level:     dbRow.Level,
		}
		delegations = append(delegations, delegation)
	}

	// Determine if there are more rows in the read direction using LIMIT n+1 technique
	overflowed := uint64(len(delegations)) > criteria.ItemsPerPage()
	if overflowed {
		// Remove the extra record we requested to detect "has more"
		delegations = delegations[:criteria.ItemsPerPage()]
	}

	// Seeking backward reads oldest-first (nearest the cursor first); pages are newest-first
	if criteria.SeeksBackward() {
		slices.Reverse(delegations)
	}

	return tezos.NewDelegationsPage(criteria, delegations, overflowed), nil
}
