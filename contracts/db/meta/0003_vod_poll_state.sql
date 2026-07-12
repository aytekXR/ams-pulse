-- Migration 0003 — vod_poll_state seen-set table (BUG-002 fix).
-- Persists the set of VoD IDs already ingested per app so the REST poll
-- poller does not double-emit recording_ready events across restarts.
--
-- Design: seen-set keyed on (app, vod_id). OQ-1 confirmed that AMS 3.0.3
-- exposes a stable vodId field on the vods/list endpoint, making the
-- seen-set approach safe (avoids near-collision risk of a HWM-by-creationDate).
--
-- created_ms: Unix epoch ms of VoD creation (from AMS creationDate). Stored
-- for diagnostic purposes only; deduplication uses (app, vod_id) PK.
CREATE TABLE IF NOT EXISTS vod_poll_state (
    app        TEXT    NOT NULL,
    vod_id     TEXT    NOT NULL,
    created_ms INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (app, vod_id)
);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
    VALUES ('0003', strftime('%s','now') * 1000);
