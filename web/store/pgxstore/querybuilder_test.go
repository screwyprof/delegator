package pgxstore_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/screwyprof/delegator/web/store/pgxstore"
	"github.com/screwyprof/delegator/web/tezos"
)

// selectDelegations is the column list every delegations query starts with.
const selectDelegations = "SELECT id, timestamp, amount, delegator, level FROM delegations "

// The builder emits the exact SQL for each pagination mode: page numbers with OFFSET, and
// cursor seeks in either direction, always in the tie-broken (timestamp, id) order.
func TestDelegationsQueryBuilder(t *testing.T) {
	t.Parallel()

	ts := time.Date(2025, 1, 15, 10, 0, 5, 0, time.UTC)
	cursor := &tezos.Cursor{Timestamp: ts, ID: 7}

	testCases := []struct {
		name     string
		criteria tezos.DelegationsCriteria
		wantSQL  string
		wantArgs []any
	}{
		{
			name:     "first page orders by timestamp then id, newest first",
			criteria: tezos.DelegationsCriteria{Page: 1, Size: 50},
			wantSQL:  selectDelegations + "ORDER BY timestamp DESC, id DESC LIMIT $1",
			wantArgs: []any{uint64(51)},
		},
		{
			name:     "page number with year filter uses offset",
			criteria: tezos.DelegationsCriteria{Year: 2025, Page: 3, Size: 10},
			wantSQL:  selectDelegations + "WHERE year = $1 ORDER BY timestamp DESC, id DESC LIMIT $2 OFFSET $3",
			wantArgs: []any{uint64(2025), uint64(11), uint64(20)},
		},
		{
			name:     "after cursor seeks past the position instead of offsetting",
			criteria: tezos.DelegationsCriteria{Year: 2025, Page: 1, Size: 10, After: cursor},
			wantSQL: selectDelegations + "WHERE year = $1 AND (timestamp, id) < ($2, $3) " +
				"ORDER BY timestamp DESC, id DESC LIMIT $4",
			wantArgs: []any{uint64(2025), ts, int64(7), uint64(11)},
		},
		{
			name:     "before cursor reads toward newer rows, nearest first",
			criteria: tezos.DelegationsCriteria{Page: 1, Size: 10, Before: cursor},
			wantSQL:  selectDelegations + "WHERE (timestamp, id) > ($1, $2) ORDER BY timestamp ASC, id ASC LIMIT $3",
			wantArgs: []any{ts, int64(7), uint64(11)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			sql, args := pgxstore.NewDelegationsQuery().ForCriteria(tc.criteria).Build()

			// Assert
			assert.Equal(t, tc.wantSQL, sql)
			assert.Equal(t, tc.wantArgs, args)
		})
	}
}
