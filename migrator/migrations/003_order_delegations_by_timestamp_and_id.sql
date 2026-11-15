-- +migrate Up notransaction
-- Delegations are paged newest first by (timestamp, id): id breaks timestamp ties, which are
-- common (every delegation in a block shares its timestamp). Index the full sort key so both
-- the ORDER BY and the (timestamp, id) cursor comparison are served straight from the index.
--
-- CONCURRENTLY (hence notransaction) so neither reads nor scraper inserts block while the
-- indexes build, and the new indexes exist before the old ones are dropped, so queries always
-- have one to use. If a build is interrupted it leaves an INVALID index that IF NOT EXISTS
-- would skip on retry: drop it by hand, then re-run.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_delegations_timestamp_id ON delegations (timestamp DESC, id DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_delegations_year_timestamp_id ON delegations (year, timestamp DESC, id DESC);

DROP INDEX CONCURRENTLY IF EXISTS idx_delegations_timestamp;
DROP INDEX CONCURRENTLY IF EXISTS idx_delegations_year_timestamp;
