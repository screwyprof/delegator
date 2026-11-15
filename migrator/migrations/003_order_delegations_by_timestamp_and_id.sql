-- +migrate Up notransaction
-- Delegations are paged newest first by (timestamp, id): id breaks timestamp ties, which are
-- common (every delegation in a block shares its timestamp). Index the full sort key so both
-- the ORDER BY and the (timestamp, id) cursor comparison are served straight from the index.
--
-- Everything is CONCURRENTLY (hence notransaction), so reads and scraper inserts never block,
-- and on a normal run the new indexes exist before the old ones are dropped.
--
-- Safe to retry. A run interrupted by a signal (say, while a build waits behind a long transaction)
-- can leave a new index INVALID, which IF NOT EXISTS would skip before dropping the old, working
-- indexes. So any new index left by an earlier attempt is dropped first and rebuilt: a no-op on
-- a first run, and only a repeated build on a retry. Unconditional because a conditional drop
-- needs a DO block, where DROP INDEX can't be CONCURRENTLY and would lock the table.
--
-- The cost: if an earlier attempt was interrupted after it had already dropped an old index, a
-- retry leaves those queries without an index until the rebuild finishes (about 0.2s on the full
-- mainnet history). Slower reads for that long, never wrong ones and never a lock.
DROP INDEX CONCURRENTLY IF EXISTS idx_delegations_timestamp_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_delegations_year_timestamp_id;

CREATE INDEX CONCURRENTLY idx_delegations_timestamp_id ON delegations (timestamp DESC, id DESC);
CREATE INDEX CONCURRENTLY idx_delegations_year_timestamp_id ON delegations (year, timestamp DESC, id DESC);

DROP INDEX CONCURRENTLY IF EXISTS idx_delegations_timestamp;
DROP INDEX CONCURRENTLY IF EXISTS idx_delegations_year_timestamp;
