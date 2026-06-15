-- Pulse ClickHouse schema — migration 0002
-- Owner: INT-01 (wave 3-plus, D-018 CR-VD38).
-- Executed by `pulse migrate` on startup after 0001_init.sql.
--
-- Adds a dedicated daily concurrency rollup sourced directly from
-- server_events (stream_stats events) rather than viewer_sessions,
-- enabling accurate peak-concurrency queries that survive session-stitching
-- edge cases (origin/edge splits, short sessions).
--
-- Variable substitution (same runner as 0001):
--   {db}               — target database name (default: pulse)
--   {rollup_ttl_days}  — rollup retention in days (default: 395, ~13 months)

-- rollup_concurrency_1d: daily peak-concurrency aggregates per stream
-- AggregatingMergeTree: uses maxState so partial rollups merge correctly.
CREATE TABLE IF NOT EXISTS {db}.rollup_concurrency_1d
(
    bucket              Date,
    app                 LowCardinality(String)  DEFAULT '',
    stream_id           String                  DEFAULT '',
    peak_concurrency    AggregateFunction(max, UInt32)
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(bucket)
ORDER BY (bucket, app, stream_id)
TTL bucket + toIntervalDay({rollup_ttl_days})
SETTINGS index_granularity = 8192;

-- mv_concurrency_1d: populate rollup_concurrency_1d from server_events.
-- Reads stream_stats events (the event type that carries viewer_count
-- snapshots; see domain.EventStreamStats = "stream_stats" in
-- server/internal/domain/types.go and normalize.go in the collector).
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_concurrency_1d
TO {db}.rollup_concurrency_1d AS
SELECT
    toDate(ts)                      AS bucket,
    app,
    stream_id,
    maxState(viewer_count)          AS peak_concurrency
FROM {db}.server_events
WHERE event_type = 'stream_stats'
GROUP BY bucket, app, stream_id;
