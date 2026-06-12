-- Pulse ClickHouse schema — migration 0001
-- Owner: INT-01 (finalized wave-1). Executed by `pulse migrate` on startup.
--
-- Design intent (PRD §7.10, ARCHITECTURE §3, §4):
--   * Raw event tables partitioned by day, TTL 90 days (configurable via
--     {retention_days} variable substitution; default 90).
--   * Hourly/daily rollup tables (SummingMergeTree or AggregatingMergeTree).
--     SummingMergeTree for additive metrics (views, watch_time, egress);
--     AggregatingMergeTree for non-additive (percentiles, uniques via HLL).
--     Rollup TTL: 13 months so any historical query answers in <3s (F2).
--   * Geo and device dimensions in rollups (gaps #2/#3/#9).
--   * probe_results in ClickHouse (see design note below).
--   * Single-node embedded deployment by default; no cluster/replication DDL in v1.
--   * Low-footprint tuning for 2-vCPU sidecar (ARCHITECTURE §3.6).
--
-- Variable substitution in `pulse migrate`:
--   {db}               — target database name (default: pulse)
--   {retention_days}   — raw event retention in days (default: 90)
--   {rollup_ttl_days}  — rollup retention in days (default: 395, ~13 months)
--
-- Design note — probe_results placement (INT-01 ruling):
--   Probe results are time-series data (timestamp + metrics per check) with
--   high write frequency and efficient range-query requirements. They belong in
--   ClickHouse (`probe_results` table), NOT the meta store. Probe config (URL,
--   interval, enabled) is small relational state and belongs in the meta store
--   (`probes` table in 0001_init.sql for meta). The `GET /probes/{id}/results`
--   API endpoint queries ClickHouse.
--
-- Design note — anomaly_baselines placement (INT-01 ruling):
--   Anomaly baselines are rolling-window statistics (mean, stddev) computed
--   periodically and stored as config-like state. They are low-cardinality,
--   mutated in-place, and never queried with range predicates. They belong in
--   the meta store (`anomaly_baselines` table), NOT ClickHouse.

CREATE DATABASE IF NOT EXISTS {db};

-- ═══════════════════════════════════════════════════════════════
-- Raw event tables
-- ═══════════════════════════════════════════════════════════════

-- server_events: normalized ServerEvent stream from the collector
-- Partition by day for efficient TTL drops; ORDER BY for stream+time queries.
CREATE TABLE IF NOT EXISTS {db}.server_events
(
    -- Core envelope
    version         UInt8        DEFAULT 1,
    event_type      LowCardinality(String),   -- stream_publish_start, node_stats, etc.
    ts              DateTime64(3, 'UTC'),       -- Unix epoch ms as DateTime64
    source          LowCardinality(String),    -- rest_poll, log_tail, kafka, webhook
    node_id         String,
    app             LowCardinality(String)     DEFAULT '',
    stream_id       String                     DEFAULT '',

    -- Publish events
    publish_type    LowCardinality(String)     DEFAULT '',

    -- Stream stats
    viewer_count    UInt32                     DEFAULT 0,
    vc_webrtc       UInt32                     DEFAULT 0,
    vc_hls          UInt32                     DEFAULT 0,
    vc_rtmp         UInt32                     DEFAULT 0,
    vc_dash         UInt32                     DEFAULT 0,
    vc_other        UInt32                     DEFAULT 0,
    bitrate_kbps    Float32                    DEFAULT 0,

    -- WebRTC client stats
    client_id       String                     DEFAULT '',
    rtt_ms          Float32                    DEFAULT 0,
    jitter_ms       Float32                    DEFAULT 0,
    packet_loss_pct Float32                    DEFAULT 0,

    -- Ingest stats (F4)
    fps             Float32                    DEFAULT 0,
    keyframe_interval_s Float32               DEFAULT 0,

    -- Node stats
    cpu_pct         Float32                    DEFAULT 0,
    mem_pct         Float32                    DEFAULT 0,
    disk_pct        Float32                    DEFAULT 0,
    net_in_mbps     Float32                    DEFAULT 0,
    net_out_mbps    Float32                    DEFAULT 0,
    jvm_heap_mb     Float32                    DEFAULT 0,

    -- Recording
    recording_path  String                     DEFAULT '',
    recording_size  UInt64                     DEFAULT 0,
    recording_dur_s UInt32                     DEFAULT 0,

    -- Viewer join/leave
    viewer_id       String                     DEFAULT '',
    protocol        LowCardinality(String)     DEFAULT '',
    ip_hash         String                     DEFAULT '',
    user_agent      String                     DEFAULT '',
    watch_time_s    UInt32                     DEFAULT 0,

    -- Collector-added enrichment (geo + device)
    geo_country     LowCardinality(String)     DEFAULT '',
    geo_region      LowCardinality(String)     DEFAULT '',
    client_device   LowCardinality(String)     DEFAULT '',
    client_os       LowCardinality(String)     DEFAULT '',
    client_browser  LowCardinality(String)     DEFAULT ''
)
ENGINE = MergeTree()
PARTITION BY toDate(ts)
ORDER BY (stream_id, ts, event_type)
TTL toDate(ts) + toIntervalDay({retention_days})
SETTINGS
    index_granularity = 8192,
    min_bytes_for_wide_part = 10485760;   -- 10 MB: keep narrow parts for small deployments

-- beacon_events: viewer QoE events from the beacon SDK
CREATE TABLE IF NOT EXISTS {db}.beacon_events
(
    version         UInt8         DEFAULT 1,
    session_id      String,                    -- client-generated UUID
    stream_id       String,
    app             LowCardinality(String)      DEFAULT '',
    ts              DateTime64(3, 'UTC'),

    -- Event type
    event_type      LowCardinality(String),    -- startup_complete, heartbeat, error, etc.

    -- startup_complete
    startup_ms      UInt32                     DEFAULT 0,

    -- heartbeat
    watch_ms        UInt32                     DEFAULT 0,
    bitrate_kbps    Float32                    DEFAULT 0,
    buffer_ms       UInt32                     DEFAULT 0,
    dropped_frames  UInt32                     DEFAULT 0,

    -- rebuffer_end
    rebuffer_ms     UInt32                     DEFAULT 0,

    -- error
    error_code      LowCardinality(String)     DEFAULT '',
    error_fatal     UInt8                      DEFAULT 0,

    -- bitrate_change
    bitrate_from    Float32                    DEFAULT 0,
    bitrate_to      Float32                    DEFAULT 0,

    -- resolution_change
    resolution_from LowCardinality(String)     DEFAULT '',
    resolution_to   LowCardinality(String)     DEFAULT '',

    -- Player info
    player_kind     LowCardinality(String)     DEFAULT '',
    sdk_version     LowCardinality(String)     DEFAULT '',

    -- Customer metadata (tenant, etc.)
    tenant          LowCardinality(String)     DEFAULT '',

    -- Collector-added enrichment
    geo_country     LowCardinality(String)     DEFAULT '',
    geo_region      LowCardinality(String)     DEFAULT '',
    client_device   LowCardinality(String)     DEFAULT '',
    client_os       LowCardinality(String)     DEFAULT '',
    client_browser  LowCardinality(String)     DEFAULT ''
)
ENGINE = MergeTree()
PARTITION BY toDate(ts)
ORDER BY (session_id, ts)
TTL toDate(ts) + toIntervalDay({retention_days})
SETTINGS
    index_granularity = 8192,
    min_bytes_for_wide_part = 10485760;

-- viewer_sessions: session-stitched aggregates (one row per session).
-- ReplacingMergeTree: the collector upserts on session_id; final state is
-- determined by the highest `updated_at`. Merged in the background; use
-- FINAL keyword or deduplication logic in queries.
CREATE TABLE IF NOT EXISTS {db}.viewer_sessions
(
    session_id      String,
    stream_id       String,
    app             LowCardinality(String)     DEFAULT '',
    node_id         String                     DEFAULT '',
    started_at      DateTime64(3, 'UTC'),
    ended_at        DateTime64(3, 'UTC')       DEFAULT toDateTime64(0, 3),
    updated_at      DateTime64(3, 'UTC'),       -- ReplacingMergeTree version column

    -- QoE summary for the session
    startup_ms      UInt32                     DEFAULT 0,
    watch_time_s    UInt32                     DEFAULT 0,
    rebuffer_count  UInt16                     DEFAULT 0,
    rebuffer_ms     UInt32                     DEFAULT 0,
    error_count     UInt16                     DEFAULT 0,
    peak_bitrate    Float32                    DEFAULT 0,

    -- Protocol + device
    protocol        LowCardinality(String)     DEFAULT '',
    geo_country     LowCardinality(String)     DEFAULT '',
    geo_region      LowCardinality(String)     DEFAULT '',
    client_device   LowCardinality(String)     DEFAULT '',
    client_os       LowCardinality(String)     DEFAULT '',
    client_browser  LowCardinality(String)     DEFAULT '',

    -- Tenant
    tenant          LowCardinality(String)     DEFAULT ''
)
ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY toDate(started_at)
ORDER BY (stream_id, session_id)
TTL toDate(started_at) + toIntervalDay({retention_days})
SETTINGS index_granularity = 8192;

-- probe_results: synthetic stream probe check results (F10).
-- ClickHouse is chosen because results are time-series with high write
-- frequency and efficient range-query requirements (see design note above).
CREATE TABLE IF NOT EXISTS {db}.probe_results
(
    id          String,
    probe_id    String,
    ts          DateTime64(3, 'UTC'),
    success     UInt8                     DEFAULT 0,
    ttfb_ms     UInt32                    DEFAULT 0,
    error_code  LowCardinality(String)    DEFAULT '',
    error_msg   String                    DEFAULT '',
    bitrate_kbps Float32                  DEFAULT 0
)
ENGINE = MergeTree()
PARTITION BY toDate(ts)
ORDER BY (probe_id, ts)
TTL toDate(ts) + toIntervalDay(90)
SETTINGS index_granularity = 8192;

-- ═══════════════════════════════════════════════════════════════
-- Rollup tables (SummingMergeTree for additive, AggregatingMergeTree
-- for non-additive metrics)
-- ═══════════════════════════════════════════════════════════════
--
-- SummingMergeTree: ClickHouse automatically sums columns marked with
-- numeric types when merging parts with the same ORDER BY key. Simple,
-- low-overhead, ideal for views/watch_time/egress. Chosen for audience
-- and usage rollups where all metrics are additive sums or max/counts.
--
-- AggregatingMergeTree: used for QoE rollups that require mergeable
-- aggregates (HLL for uniques, quantile states for p50/p95). Higher
-- storage overhead but enables accurate percentile computation.

-- rollup_audience_1h: hourly audience aggregates
-- AggregatingMergeTree for uniq (HLL) + quantile states
CREATE TABLE IF NOT EXISTS {db}.rollup_audience_1h
(
    bucket      DateTime,                      -- truncated to hour
    app         LowCardinality(String)         DEFAULT '',
    stream_id   String                         DEFAULT '',
    node_id     String                         DEFAULT '',
    tenant      LowCardinality(String)         DEFAULT '',
    geo_country LowCardinality(String)         DEFAULT '',
    client_device LowCardinality(String)       DEFAULT '',
    protocol    LowCardinality(String)         DEFAULT '',

    views       AggregateFunction(count, UInt64),
    uniq_viewers AggregateFunction(uniq, String),
    watch_time_s AggregateFunction(sum, UInt64),
    peak_concurrency AggregateFunction(max, UInt32)
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(bucket)
ORDER BY (bucket, app, stream_id, tenant, geo_country, client_device, protocol)
TTL bucket + toIntervalDay({rollup_ttl_days})
SETTINGS index_granularity = 8192;

-- rollup_audience_1d: daily audience aggregates (same shape, day buckets)
CREATE TABLE IF NOT EXISTS {db}.rollup_audience_1d
(
    bucket      Date,
    app         LowCardinality(String)         DEFAULT '',
    stream_id   String                         DEFAULT '',
    node_id     String                         DEFAULT '',
    tenant      LowCardinality(String)         DEFAULT '',
    geo_country LowCardinality(String)         DEFAULT '',
    client_device LowCardinality(String)       DEFAULT '',
    protocol    LowCardinality(String)         DEFAULT '',

    views       AggregateFunction(count, UInt64),
    uniq_viewers AggregateFunction(uniq, String),
    watch_time_s AggregateFunction(sum, UInt64),
    peak_concurrency AggregateFunction(max, UInt32)
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(bucket)
ORDER BY (bucket, app, stream_id, tenant, geo_country, client_device, protocol)
TTL bucket + toIntervalDay({rollup_ttl_days})
SETTINGS index_granularity = 8192;

-- rollup_qoe_1h: hourly QoE rollup from beacon_events
-- AggregatingMergeTree for quantile states (startup p50/p95)
CREATE TABLE IF NOT EXISTS {db}.rollup_qoe_1h
(
    bucket      DateTime,
    app         LowCardinality(String)         DEFAULT '',
    stream_id   String                         DEFAULT '',
    tenant      LowCardinality(String)         DEFAULT '',
    geo_country LowCardinality(String)         DEFAULT '',
    client_device LowCardinality(String)       DEFAULT '',

    startup_ms_state    AggregateFunction(quantilesState(0.5, 0.95), Float32),
    rebuffer_total_ms   AggregateFunction(sum, UInt64),
    rebuffer_count      AggregateFunction(sum, UInt64),
    watch_time_ms       AggregateFunction(sum, UInt64),
    error_count         AggregateFunction(sum, UInt64),
    session_count       AggregateFunction(count, UInt64),
    bitrate_kbps_state  AggregateFunction(quantilesState(0.5, 0.95), Float32)
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(bucket)
ORDER BY (bucket, app, stream_id, tenant, geo_country, client_device)
TTL bucket + toIntervalDay({rollup_ttl_days})
SETTINGS index_granularity = 8192;

-- rollup_qoe_1d: daily QoE rollup
CREATE TABLE IF NOT EXISTS {db}.rollup_qoe_1d
(
    bucket      Date,
    app         LowCardinality(String)         DEFAULT '',
    stream_id   String                         DEFAULT '',
    tenant      LowCardinality(String)         DEFAULT '',
    geo_country LowCardinality(String)         DEFAULT '',
    client_device LowCardinality(String)       DEFAULT '',

    startup_ms_state    AggregateFunction(quantilesState(0.5, 0.95), Float32),
    rebuffer_total_ms   AggregateFunction(sum, UInt64),
    rebuffer_count      AggregateFunction(sum, UInt64),
    watch_time_ms       AggregateFunction(sum, UInt64),
    error_count         AggregateFunction(sum, UInt64),
    session_count       AggregateFunction(count, UInt64),
    bitrate_kbps_state  AggregateFunction(quantilesState(0.5, 0.95), Float32)
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(bucket)
ORDER BY (bucket, app, stream_id, tenant, geo_country, client_device)
TTL bucket + toIntervalDay({rollup_ttl_days})
SETTINGS index_granularity = 8192;

-- rollup_usage_1d: daily usage for billing (F6)
-- SummingMergeTree: all metrics are additive (viewer_minutes, egress_bytes).
-- geo + device dimensions included per gap #9.
CREATE TABLE IF NOT EXISTS {db}.rollup_usage_1d
(
    bucket          Date,
    app             LowCardinality(String)     DEFAULT '',
    stream_id       String                     DEFAULT '',
    node_id         String                     DEFAULT '',
    tenant          LowCardinality(String)     DEFAULT '',
    geo_country     LowCardinality(String)     DEFAULT '',
    client_device   LowCardinality(String)     DEFAULT '',
    protocol        LowCardinality(String)     DEFAULT '',

    -- Additive usage metrics (summed by SummingMergeTree)
    viewer_minutes  Float64                    DEFAULT 0,
    peak_concurrency UInt32                    DEFAULT 0,
    egress_bytes    UInt64                     DEFAULT 0,
    recording_bytes UInt64                     DEFAULT 0
)
ENGINE = SummingMergeTree((viewer_minutes, egress_bytes, recording_bytes))
PARTITION BY toYYYYMM(bucket)
ORDER BY (bucket, app, stream_id, tenant, geo_country, client_device, protocol)
TTL bucket + toIntervalDay({rollup_ttl_days})
SETTINGS index_granularity = 8192;

-- ═══════════════════════════════════════════════════════════════
-- Materialized views
-- ═══════════════════════════════════════════════════════════════

-- mv_audience_1h: populate rollup_audience_1h from viewer_sessions
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_audience_1h
TO {db}.rollup_audience_1h AS
SELECT
    toStartOfHour(started_at)          AS bucket,
    app,
    stream_id,
    node_id,
    tenant,
    geo_country,
    client_device,
    protocol,
    countState()                        AS views,
    uniqState(session_id)               AS uniq_viewers,
    sumState(toUInt64(watch_time_s))    AS watch_time_s,
    maxState(toUInt32(1))               AS peak_concurrency
FROM {db}.viewer_sessions
GROUP BY bucket, app, stream_id, node_id, tenant, geo_country, client_device, protocol;

-- mv_audience_1d: populate rollup_audience_1d from viewer_sessions
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_audience_1d
TO {db}.rollup_audience_1d AS
SELECT
    toDate(started_at)                  AS bucket,
    app,
    stream_id,
    node_id,
    tenant,
    geo_country,
    client_device,
    protocol,
    countState()                        AS views,
    uniqState(session_id)               AS uniq_viewers,
    sumState(toUInt64(watch_time_s))    AS watch_time_s,
    maxState(toUInt32(1))               AS peak_concurrency
FROM {db}.viewer_sessions
GROUP BY bucket, app, stream_id, node_id, tenant, geo_country, client_device, protocol;

-- mv_qoe_1h: populate rollup_qoe_1h from beacon_events
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_qoe_1h
TO {db}.rollup_qoe_1h AS
SELECT
    toStartOfHour(ts)                              AS bucket,
    app,
    stream_id,
    tenant,
    geo_country,
    client_device,
    quantilesState(0.5, 0.95)(toFloat32(startup_ms)) AS startup_ms_state,
    sumState(toUInt64(rebuffer_ms))                AS rebuffer_total_ms,
    sumState(toUInt64(if(event_type = 'rebuffer_end', 1, 0))) AS rebuffer_count,
    sumState(toUInt64(watch_ms))                   AS watch_time_ms,
    sumState(toUInt64(if(event_type = 'error', 1, 0))) AS error_count,
    countState()                                   AS session_count,
    quantilesState(0.5, 0.95)(toFloat32(bitrate_kbps)) AS bitrate_kbps_state
FROM {db}.beacon_events
WHERE event_type IN ('startup_complete', 'heartbeat', 'rebuffer_end', 'error')
GROUP BY bucket, app, stream_id, tenant, geo_country, client_device;

-- mv_qoe_1d: populate rollup_qoe_1d from beacon_events
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_qoe_1d
TO {db}.rollup_qoe_1d AS
SELECT
    toDate(ts)                                     AS bucket,
    app,
    stream_id,
    tenant,
    geo_country,
    client_device,
    quantilesState(0.5, 0.95)(toFloat32(startup_ms)) AS startup_ms_state,
    sumState(toUInt64(rebuffer_ms))                AS rebuffer_total_ms,
    sumState(toUInt64(if(event_type = 'rebuffer_end', 1, 0))) AS rebuffer_count,
    sumState(toUInt64(watch_ms))                   AS watch_time_ms,
    sumState(toUInt64(if(event_type = 'error', 1, 0))) AS error_count,
    countState()                                   AS session_count,
    quantilesState(0.5, 0.95)(toFloat32(bitrate_kbps)) AS bitrate_kbps_state
FROM {db}.beacon_events
WHERE event_type IN ('startup_complete', 'heartbeat', 'rebuffer_end', 'error')
GROUP BY bucket, app, stream_id, tenant, geo_country, client_device;

-- mv_usage_1d: populate rollup_usage_1d from viewer_sessions
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_usage_1d
TO {db}.rollup_usage_1d AS
SELECT
    toDate(started_at)                      AS bucket,
    app,
    stream_id,
    node_id,
    tenant,
    geo_country,
    client_device,
    protocol,
    toFloat64(watch_time_s) / 60.0          AS viewer_minutes,
    toUInt32(1)                             AS peak_concurrency,  -- summed per key
    toUInt64(0)                             AS egress_bytes,      -- populated from server_events stream_stats
    toUInt64(0)                             AS recording_bytes
FROM {db}.viewer_sessions;
