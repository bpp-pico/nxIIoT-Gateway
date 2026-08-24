-- Store & Forward (§9) needs a per-row "not eligible for retry until" time
-- to implement exponential backoff (§9.4) without hammering a down server.
--
-- SQLite's ALTER TABLE ADD COLUMN rejects a non-constant default (e.g.
-- strftime(...)) on a table that already has rows, since it would need to
-- backfill every existing row — confirmed the hard way against a database
-- with pre-existing data_queue rows from earlier phases (a fresh/empty
-- table doesn't trigger it, which is why this passed in tests before it
-- failed live). A fixed epoch default is used instead; it's functionally
-- equivalent here since "next_attempt_at in the past" just means
-- "immediately eligible", which is exactly right for both existing rows
-- and all new inserts (Insert() doesn't set this column explicitly).
ALTER TABLE data_queue ADD COLUMN next_attempt_at TEXT NOT NULL DEFAULT '1970-01-01T00:00:00.000Z';

CREATE INDEX IF NOT EXISTS idx_data_queue_pending_dispatch ON data_queue(status, next_attempt_at);
