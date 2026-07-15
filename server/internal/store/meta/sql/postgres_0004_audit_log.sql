-- Pulse metadata store schema — migration 0004 (PostgreSQL)
-- Embedded copy of contracts/db/meta/postgres/0004_audit_log.sql.
-- Sync command: cp contracts/db/meta/postgres/0004_audit_log.sql server/internal/store/meta/sql/postgres_0004_audit_log.sql
-- Source of truth: contracts/db/meta/0004_audit_log.sql (SQLite)
-- This file is the faithful PostgreSQL translation of that file.
-- Keep both files in sync: any structural change to the SQLite original
-- must be mirrored here.
--
-- Mapping rules (SQLite → PostgreSQL):
--   INTEGER (epoch-ms)           → BIGINT
--   TEXT NOT NULL DEFAULT ''     → TEXT NOT NULL DEFAULT '' (unchanged)
--   INSERT OR IGNORE             → INSERT ... ON CONFLICT (version) DO NOTHING
--   strftime('%s','now')*1000    → (EXTRACT(EPOCH FROM NOW())::bigint * 1000)

-- Migration 0004 — audit_log table (S40 / D-102).
-- Append-only record of "who changed what, when" for every mutating admin/config
-- API call. NO foreign keys — an audit row must survive token revocation and user
-- deletion. See the SQLite source for full design notes.
CREATE TABLE IF NOT EXISTS audit_log (
    id             TEXT   NOT NULL PRIMARY KEY,
    ts             BIGINT NOT NULL,
    actor_token_id TEXT   NOT NULL DEFAULT '',
    actor_user_id  TEXT   NOT NULL DEFAULT '',
    actor_name     TEXT   NOT NULL DEFAULT '',
    action         TEXT   NOT NULL,
    object_type    TEXT   NOT NULL,
    object_id      TEXT   NOT NULL DEFAULT '',
    remote_addr    TEXT   NOT NULL DEFAULT '',
    detail_json    TEXT   NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_audit_log_ts ON audit_log(ts DESC, id DESC);

-- Record this migration (same version string as SQLite original for parity checks).
INSERT INTO schema_migrations(version, applied_at)
    VALUES ('0004', (EXTRACT(EPOCH FROM NOW())::bigint * 1000))
    ON CONFLICT (version) DO NOTHING;
