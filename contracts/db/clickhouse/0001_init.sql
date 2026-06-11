-- Pulse ClickHouse schema — migration 0001 (SKELETON).
-- Owner: BE-01. Finalized before Phase 1 implementation.
--
-- Design intent (PRD §7.10):
--   * Raw event tables partitioned by day, TTL 90 days (configurable).
--   * Hourly/daily materialized-view rollups, TTL 13 months, so any 13-month
--     query answers from rollups in < 3s (F2 acceptance criteria).
--   * Single-node embedded deployment by default; no cluster/replication DDL in v1.

-- TODO(BE-01): server_events       — normalized ServerEvent stream (contracts/events/ams-server-event.schema.json)
-- TODO(BE-01): beacon_events       — viewer QoE events (contracts/events/beacon-event.schema.json)
-- TODO(BE-01): viewer_sessions     — session-stitched aggregates (one row per session, ReplacingMergeTree)
-- TODO(BE-01): rollup_audience_1h / _1d   — views, uniques, watch time, peak concurrency
-- TODO(BE-01): rollup_qoe_1h / _1d        — startup, rebuffer ratio, error rate
-- TODO(BE-01): rollup_usage_1d            — viewer-minutes, egress estimate per app/stream/tenant (F6)

SELECT 1; -- placeholder so the file is a valid statement until tables land
