-- Migration 0004 — audit_log table (S40 / D-102).
-- Append-only record of "who changed what, when" for every mutating admin/config
-- API call (alert rules & channels, users, tokens, probes, report schedules, AMS
-- sources, tenants). Gates SOC 2 / ISO 27001 buyers, who require an actor trail.
--
-- Design notes:
--   * Append-only: rows are INSERTed, never UPDATEd or DELETEd by the app.
--   * NO foreign keys to api_tokens / users — an audit row MUST survive token
--     revocation and user deletion (that is precisely when it matters most).
--   * actor_token_id is the api_tokens.id of the caller (always present — every
--     mutating route is behind bearer auth); actor_user_id links to users.id for
--     OIDC-minted sessions and is '' for manually-created service tokens.
--   * ts is Unix epoch ms (matches nowMS() everywhere else in this store).
--   * detail_json is an optional small JSON blob ('' when absent) — e.g. the
--     created/updated object summary. Full before/after diffs are a later phase.
CREATE TABLE IF NOT EXISTS audit_log (
    id             TEXT    NOT NULL PRIMARY KEY,   -- UUID
    ts             INTEGER NOT NULL,               -- Unix epoch ms
    actor_token_id TEXT    NOT NULL DEFAULT '',    -- api_tokens.id of the caller
    actor_user_id  TEXT    NOT NULL DEFAULT '',    -- users.id (OIDC); '' for service tokens
    actor_name     TEXT    NOT NULL DEFAULT '',    -- token display name
    action         TEXT    NOT NULL,               -- e.g. 'alert_rule.create'
    object_type    TEXT    NOT NULL,               -- e.g. 'alert_rule'
    object_id      TEXT    NOT NULL DEFAULT '',    -- affected resource id ('' for none)
    remote_addr    TEXT    NOT NULL DEFAULT '',    -- request source IP
    detail_json    TEXT    NOT NULL DEFAULT ''     -- optional JSON context ('' when absent)
);

-- Newest-first reads are the operator view; the composite index makes the
-- (ts DESC, id DESC) keyset pagination in ListAuditLog an index scan.
CREATE INDEX IF NOT EXISTS idx_audit_log_ts ON audit_log(ts DESC, id DESC);

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
    VALUES ('0004', strftime('%s','now') * 1000);
