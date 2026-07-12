-- Pulse metadata store schema — migration 0003 (PostgreSQL)
-- Embedded copy of contracts/db/meta/postgres/0003_vod_poll_state.sql.
-- Sync command: cp contracts/db/meta/postgres/0003_vod_poll_state.sql server/internal/store/meta/sql/postgres_0003_vod_poll_state.sql
-- Source of truth: contracts/db/meta/0003_vod_poll_state.sql (SQLite)
-- This file is the faithful PostgreSQL translation of that file.
-- Keep both files in sync: any structural change to the SQLite original
-- must be mirrored here.
--
-- Mapping rules (SQLite → PostgreSQL):
--   INTEGER NOT NULL DEFAULT 0   → BIGINT NOT NULL DEFAULT 0 (for epoch-ms)
--   INSERT OR IGNORE             → INSERT ... ON CONFLICT (<pk_col>) DO NOTHING
--   strftime('%s','now')*1000    → (EXTRACT(EPOCH FROM NOW())::bigint * 1000)
--   PRIMARY KEY (a, b)           → PRIMARY KEY (a, b) (unchanged)

-- Migration 0003 — vod_poll_state seen-set table (BUG-002 fix).
-- Persists the set of VoD IDs already ingested per app so the REST poll
-- poller does not double-emit recording_ready events across restarts.
--
-- created_ms: Unix epoch ms of VoD creation (from AMS creationDate). Stored
-- for diagnostic purposes only; deduplication uses (app, vod_id) PK.
CREATE TABLE IF NOT EXISTS vod_poll_state (
    app        TEXT   NOT NULL,
    vod_id     TEXT   NOT NULL,
    created_ms BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (app, vod_id)
);

-- Record this migration (same version string as SQLite original for parity checks).
INSERT INTO schema_migrations(version, applied_at)
    VALUES ('0003', (EXTRACT(EPOCH FROM NOW())::bigint * 1000))
    ON CONFLICT (version) DO NOTHING;
