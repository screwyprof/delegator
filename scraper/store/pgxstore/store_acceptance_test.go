//go:build acceptance

package pgxstore_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/migrator/migratortest"
	"github.com/screwyprof/delegator/scraper"
	"github.com/screwyprof/delegator/scraper/store/pgxstore"
)

// The store saves each batch and its checkpoint atomically, and tolerates a batch delivered twice:
// the scraper's exactly-once guarantee rests on both.
func TestStore(t *testing.T) {
	t.Parallel()

	t.Run("it starts from checkpoint zero", func(t *testing.T) {
		t.Parallel()

		store, _ := newStore(t)

		id, err := store.LastProcessedID(t.Context())

		require.NoError(t, err)
		assert.Zero(t, id)
	})

	t.Run("it saves a batch and moves the checkpoint to its last id", func(t *testing.T) {
		t.Parallel()

		store, db := newStore(t)

		require.NoError(t, store.SaveBatch(t.Context(), delegations(101, 102, 103)))

		assert.Equal(t, int64(3), countDelegations(t, db))
		assertCheckpoint(t, store, 103)
	})

	t.Run("it ignores delegations it already has when a batch is delivered again", func(t *testing.T) {
		t.Parallel()

		store, db := newStore(t)
		require.NoError(t, store.SaveBatch(t.Context(), delegations(101, 102)))

		// Act: an overlapping batch, as after a retry whose first attempt had in fact committed
		err := store.SaveBatch(t.Context(), delegations(102, 103))

		// Assert
		require.NoError(t, err)
		assert.Equal(t, int64(3), countDelegations(t, db), "102 must not be stored twice")
		assertCheckpoint(t, store, 103)
	})

	t.Run("it stores nothing when the checkpoint can't be updated", func(t *testing.T) {
		t.Parallel()

		store, db := newStore(t)

		// Make the checkpoint step, the last in the transaction, fail.
		_, err := db.Exec(t.Context(), "DROP TABLE scraper_checkpoint")
		require.NoError(t, err)

		// Act
		err = store.SaveBatch(t.Context(), delegations(101, 102))

		// Assert: the rows were inserted earlier in the same transaction, so they must be gone too
		require.ErrorIs(t, err, pgxstore.ErrCheckpointFailed)
		assert.Zero(t, countDelegations(t, db), "rows and checkpoint must commit together or not at all")
	})

	t.Run("it does nothing for an empty batch", func(t *testing.T) {
		t.Parallel()

		store, db := newStore(t)

		require.NoError(t, store.SaveBatch(t.Context(), nil))
		assert.Zero(t, countDelegations(t, db))
		assertCheckpoint(t, store, 0)
	})
}

// newStore returns a store over a fresh, migrated database, and a pool to inspect it with.
func newStore(t *testing.T) (*pgxstore.Store, *pgxpool.Pool) {
	t.Helper()

	db := migratortest.CreateScraperTestDatabase(t, "../../../migrator/migrations", 0)
	t.Cleanup(db.Close)

	return pgxstore.New(db), db
}

// delegations builds scraper delegations with the given ids, in order.
func delegations(ids ...int64) []scraper.Delegation {
	out := make([]scraper.Delegation, 0, len(ids))
	for _, id := range ids {
		out = append(out, scraper.Delegation{
			ID:        id,
			Timestamp: time.Date(2025, 1, 15, 10, 0, int(id%60), 0, time.UTC),
			Amount:    1000 + id,
			Delegator: "tz1Test",
			Level:     4500000 + id,
		})
	}

	return out
}

func countDelegations(t *testing.T, db *pgxpool.Pool) int64 {
	t.Helper()

	var n int64
	require.NoError(t, db.QueryRow(t.Context(), "SELECT count(*) FROM delegations").Scan(&n))

	return n
}

func assertCheckpoint(t *testing.T, store *pgxstore.Store, want int64) {
	t.Helper()

	got, err := store.LastProcessedID(t.Context())
	require.NoError(t, err)
	assert.Equal(t, want, got, "checkpoint")
}
