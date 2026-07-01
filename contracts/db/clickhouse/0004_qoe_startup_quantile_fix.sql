-- Pulse ClickHouse schema — migration 0004
-- Owner: INT-01 (D-042 CR: startup-time quantile dilution fix).
-- Executed by `pulse migrate` on startup after 0003.
--
-- BUG (D-042): mv_qoe_1h / mv_qoe_1d computed the startup-time quantile
-- (startup_ms_state) over EVERY matching event type — startup_complete AND
-- heartbeat / rebuffer_end / error — but only startup_complete events carry a
-- real startup_ms; the others are 0. Those zeros diluted the reported median
-- startup time toward 0 (a wrong QoE metric in production) and made
-- TestQuery_QoeSummary_RealStartupP50 flaky right at the 0 boundary.
--
-- FIX: recreate both QoE materialized views so ONLY startup_complete events feed
-- the startup quantile, via the -If combinator (quantilesStateIf). The state type
-- is unchanged (AggregateFunction(quantilesState(0.5,0.95), Float32)) so the
-- existing rollup columns are compatible. Every other aggregate (rebuffer, error,
-- watch time, session count, bitrate) is byte-for-byte unchanged, and the WHERE
-- stays broad so those metrics still see heartbeats/rebuffers/errors.
--
-- Already-aggregated rollup rows are NOT rewritten (AggregatingMergeTree states
-- are immutable); new ingest is correct from here forward. To backfill historical
-- buckets, re-run the MV SELECT into the rollup for the affected range (ops task).
--
-- Variable substitution (same runner as 0001):
--   {db} — target database name (default: pulse)

DROP VIEW IF EXISTS {db}.mv_qoe_1h;
DROP VIEW IF EXISTS {db}.mv_qoe_1d;

-- mv_qoe_1h: populate rollup_qoe_1h from beacon_events (startup quantile fixed)
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_qoe_1h
TO {db}.rollup_qoe_1h AS
SELECT
    toStartOfHour(ts)                              AS bucket,
    app,
    stream_id,
    tenant,
    geo_country,
    client_device,
    quantilesStateIf(0.5, 0.95)(toFloat32(startup_ms), event_type = 'startup_complete') AS startup_ms_state,
    sumState(toUInt64(rebuffer_ms))                AS rebuffer_total_ms,
    sumState(toUInt64(if(event_type = 'rebuffer_end', 1, 0))) AS rebuffer_count,
    sumState(toUInt64(watch_ms))                   AS watch_time_ms,
    sumState(toUInt64(if(event_type = 'error', 1, 0))) AS error_count,
    countState()                                   AS session_count,
    quantilesState(0.5, 0.95)(toFloat32(bitrate_kbps)) AS bitrate_kbps_state
FROM {db}.beacon_events
WHERE event_type IN ('startup_complete', 'heartbeat', 'rebuffer_end', 'error')
GROUP BY bucket, app, stream_id, tenant, geo_country, client_device;

-- mv_qoe_1d: populate rollup_qoe_1d from beacon_events (startup quantile fixed)
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_qoe_1d
TO {db}.rollup_qoe_1d AS
SELECT
    toDate(ts)                                     AS bucket,
    app,
    stream_id,
    tenant,
    geo_country,
    client_device,
    quantilesStateIf(0.5, 0.95)(toFloat32(startup_ms), event_type = 'startup_complete') AS startup_ms_state,
    sumState(toUInt64(rebuffer_ms))                AS rebuffer_total_ms,
    sumState(toUInt64(if(event_type = 'rebuffer_end', 1, 0))) AS rebuffer_count,
    sumState(toUInt64(watch_ms))                   AS watch_time_ms,
    sumState(toUInt64(if(event_type = 'error', 1, 0))) AS error_count,
    countState()                                   AS session_count,
    quantilesState(0.5, 0.95)(toFloat32(bitrate_kbps)) AS bitrate_kbps_state
FROM {db}.beacon_events
WHERE event_type IN ('startup_complete', 'heartbeat', 'rebuffer_end', 'error')
GROUP BY bucket, app, stream_id, tenant, geo_country, client_device;
