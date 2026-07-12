-- Migration 0009 — mv_recording_1d materialized view (BUG-002 fix).
-- Companion to mv_usage_1d: populates rollup_usage_1d.recording_bytes
-- from server_events WHERE event_type = 'recording_ready'.
--
-- Without this MV, recording_ready events are stored in server_events
-- (auditable) but never flow into the billing rollup — recording_gb is
-- structurally 0 even after the VoD REST poll emits events (BUG-002).
--
-- Column notes (checked against 0001_init.sql server_events DDL):
--   tenant       — NOT a column in server_events; use literal '' AS tenant
--   geo_country  — LowCardinality(String) DEFAULT '' in server_events; use as-is
--   client_device — LowCardinality(String) DEFAULT '' in server_events; use as-is
--   protocol     — LowCardinality(String) DEFAULT '' in server_events; use as-is
--
-- SummingMergeTree will sum recording_bytes across rows sharing the same
-- ORDER BY key (bucket, app, stream_id, tenant, geo_country, client_device, protocol).
-- Use SELECT sum(recording_bytes) in queries — never FINAL — to aggregate
-- across unmerged parts.
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_recording_1d
TO {db}.rollup_usage_1d AS
SELECT
    toDate(ts)       AS bucket,
    app,
    stream_id,
    node_id,
    ''               AS tenant,
    geo_country,
    client_device,
    protocol,
    toFloat64(0)     AS viewer_minutes,
    toUInt32(0)      AS peak_concurrency,
    toUInt64(0)      AS egress_bytes,
    recording_size   AS recording_bytes
FROM {db}.server_events
WHERE event_type = 'recording_ready';
