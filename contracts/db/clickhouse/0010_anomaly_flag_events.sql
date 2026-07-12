-- 0010_anomaly_flag_events.sql — BUG-008 phase 2: persist anomaly flag events.
-- Owner: A1 (D-086). Executed by `pulse migrate` after 0009_recording_mv.sql.
--
-- Closes the GET /anomalies ?from/?to gap (ADR-0009): flag events are written
-- by Detector.UpdateBaselines on every tick and queried by QueryFlagHistory.
-- See docs/adr/0009-anomaly-flag-event-store.md §2 for schema rationale.
--
-- Capacity: ~17 flag-events/day at Enterprise scale (100 nodes + 500 streams).
-- At 90-day retention: ≈1,530 rows — trivially small for ClickHouse.
--
-- Column notes:
--   id          — UUID generated per event (String, not UUID type for driver compat)
--   metric      — LowCardinality(String): small closed set (viewers, cpu_pct, …)
--   node_id     — denormalised from scope for WHERE without JSONExtractString
--   app         — denormalised from scope
--   stream_id   — denormalised from scope
--   scope       — PLAIN String (raw JSON from scopeJSON(); high-cardinality values
--                 would blow the LowCardinality dictionary; stored byte-for-byte so
--                 WarmHysteresis key-identity is guaranteed — ADR §3, Risk-4)
--   observed    — live metric value at detection time
--   expected    — baseline mean at detection time
--   sigma       — z-score = |observed - expected| / effStddev
--   detected_at — tick timestamp (UTC); all events of one tick share this value
--                 (NOT time.Now() at HTTP request time — ADR §4 / Consequences §1)
--
-- ORDER BY (detected_at, metric, scope) matches the primary query pattern:
--   keyset pagination ordered by (detected_at ASC, id ASC); the id tiebreaker
--   is not in the table ORDER BY (no need for physical ordering on id), but the
--   QueryFlagHistory query imposes it explicitly in ORDER BY + WHERE.
--
-- TTL: same {retention_days} placeholder as probe_results and server_events
--   (runner.go:216 substitutes it at migration apply time).
--
-- Variables substituted by runner.go:
--   {db}             — target ClickHouse database (e.g. "pulse")
--   {retention_days} — raw event retention in days (Config.RetentionDays)

CREATE TABLE IF NOT EXISTS {db}.anomaly_flag_events
(
    id          String,
    metric      LowCardinality(String),
    node_id     String,
    app         String,
    stream_id   String,
    scope       String,
    observed    Float64,
    expected    Float64,
    sigma       Float64,
    detected_at DateTime64(3, 'UTC')
) ENGINE = MergeTree()
ORDER BY (detected_at, metric, scope)
TTL toDate(detected_at) + toIntervalDay({retention_days});
