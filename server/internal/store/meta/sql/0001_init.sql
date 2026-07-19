-- Pulse metadata store schema — migration 0001
-- Owner: INT-01 (finalized wave-1). Runs on SQLite (single-node default)
-- and PostgreSQL (HA option). DDL is the common subset both support.
--
-- Portability notes (commented where SQLite and Postgres diverge):
--   * SQLite uses INTEGER for all integer sizes; Postgres uses SERIAL/BIGSERIAL.
--     We use INTEGER / TEXT and rely on the application layer for type mapping.
--   * Boolean: SQLite stores as INTEGER 0/1; Postgres as BOOLEAN.
--     DDL uses INTEGER for SQLite compat; Postgres adapter maps transparently.
--   * JSON fields: SQLite stores as TEXT; Postgres stores as JSONB.
--     DDL uses TEXT; Postgres adapter may use JSONB column type.
--   * UUID primary keys: stored as TEXT (SQLite compat; Postgres can use uuid type).
--   * Timestamps: stored as INTEGER (Unix epoch ms) for full compat.
--
-- Schema migrations tracked in `schema_migrations`.
-- Never store metrics here — metrics live in ClickHouse (ARCHITECTURE §3).

PRAGMA foreign_keys = ON; -- SQLite: enforce FK constraints

-- ── schema_migrations ─────────────────────────────────────────
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     TEXT      NOT NULL PRIMARY KEY,
    applied_at  INTEGER   NOT NULL  -- Unix epoch ms
);

-- ── users ─────────────────────────────────────────────────────
-- Local user accounts. SSO (LDAP/OAuth) is a Phase 3 addition.
CREATE TABLE IF NOT EXISTS users (
    id          TEXT      NOT NULL PRIMARY KEY,   -- UUID
    username    TEXT      NOT NULL UNIQUE,
    pw_hash     TEXT      NOT NULL,               -- bcrypt hash; NEVER plain text
    role        TEXT      NOT NULL DEFAULT 'viewer',  -- 'admin' | 'viewer'
    created_at  INTEGER   NOT NULL,
    updated_at  INTEGER   NOT NULL
);

-- ── api_tokens ────────────────────────────────────────────────
-- Bearer tokens for the REST API and ingest endpoint.
-- Raw token value is NEVER stored — only the keyed hash.
-- hash_alg: 'hmac-sha256' (new; PULSE_SECRET_KEY required) | 'sha256' (legacy).
-- New installs: HMAC-SHA256(HKDF(secret), token). Legacy rows keep 'sha256'.
-- See server/internal/store/meta/meta.go HashToken() and LookupToken().
CREATE TABLE IF NOT EXISTS api_tokens (
    id          TEXT      NOT NULL PRIMARY KEY,   -- UUID
    user_id     TEXT      REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT      NOT NULL,               -- 'api' | 'ingest'
    name        TEXT      NOT NULL,
    token_hash  TEXT      NOT NULL UNIQUE,        -- keyed hash of raw_token (see hash_alg)
    hash_alg    TEXT      NOT NULL DEFAULT 'sha256', -- 'hmac-sha256' | 'sha256'
    scopes      TEXT      NOT NULL DEFAULT '[]',  -- JSON array of scope strings
    expires_at  INTEGER,                          -- Unix epoch ms; NULL = non-expiring
    last_used_at INTEGER,
    created_at  INTEGER   NOT NULL
);

-- ── ingest_tokens ─────────────────────────────────────────────
-- Per-stream ingest tokens for beacon POST endpoint.
-- Separate table from api_tokens for access pattern reasons
-- (ingest hot path looks up by token hash; separate index helps).
CREATE TABLE IF NOT EXISTS ingest_tokens (
    id          TEXT      NOT NULL PRIMARY KEY,
    name        TEXT      NOT NULL,
    token_hash  TEXT      NOT NULL UNIQUE,
    stream_id   TEXT,                             -- optional: restrict to one stream
    app         TEXT,                             -- optional: restrict to one app
    rate_limit  INTEGER   NOT NULL DEFAULT 1000,  -- max events/min
    expires_at  INTEGER,
    created_at  INTEGER   NOT NULL,
    revoked     INTEGER   NOT NULL DEFAULT 0      -- 0=active, 1=revoked
);

-- ── ams_sources ───────────────────────────────────────────────
-- Configured AMS data sources. Credential fields are AES-256-GCM
-- encrypted at rest using the instance key; never stored in plain text.
-- credential_enc: encrypted blob (base64) containing password/secret.
-- credential_env_ref: alternative — reference to an env var holding the cred.
CREATE TABLE IF NOT EXISTS ams_sources (
    id              TEXT    NOT NULL PRIMARY KEY,
    name            TEXT    NOT NULL,
    source_type     TEXT    NOT NULL,  -- 'rest_poll' | 'kafka' | 'webhook' (log_tail removed D-155; collector deleted D-062)
    rest_url        TEXT,              -- AMS REST base URL
    rest_user       TEXT,              -- AMS REST username
    credential_enc  TEXT,              -- AES-256-GCM encrypted credential (write-only output)
    credential_env_ref TEXT,           -- env var name holding the credential (alternative)
    log_path        TEXT,              -- path to AMS analytics log file
    kafka_brokers   TEXT,              -- JSON array of broker host:port strings
    webhook_path    TEXT,              -- HTTP path Pulse listens on for webhooks
    webhook_secret_enc TEXT,           -- AES-256-GCM encrypted per-source HMAC secret (B7)
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

-- ── cluster_nodes ─────────────────────────────────────────────
-- Discovered AMS cluster nodes. Auto-discovered within 2 min (F7).
-- Cached here; source of truth is AMS itself.
CREATE TABLE IF NOT EXISTS cluster_nodes (
    id          TEXT    NOT NULL PRIMARY KEY,  -- UUID
    node_id     TEXT    NOT NULL UNIQUE,       -- AMS node identifier
    source_id   TEXT    REFERENCES ams_sources(id) ON DELETE SET NULL,
    role        TEXT    NOT NULL DEFAULT 'standalone',  -- 'origin' | 'edge' | 'standalone'
    status      TEXT    NOT NULL DEFAULT 'up',          -- 'up' | 'degraded' | 'down'
    last_seen   INTEGER NOT NULL,
    version     TEXT,                          -- AMS version string
    meta        TEXT    NOT NULL DEFAULT '{}', -- JSON: extra k/v from discovery
    created_at  INTEGER NOT NULL
);

-- ── alert_rules ───────────────────────────────────────────────
-- Alert rule definitions (F5).
-- maintenance_windows: JSON array of {start_cron, duration_s} objects.
-- scope: JSON object with optional node_id, app, stream_id.
-- channel_ids: JSON array of alert_channels.id references.
-- enabled: 0 = rule is not evaluated at all (completely suspended).
--          Distinct from muted: enabled=0 skips evaluation entirely;
--          muted=1 evaluates and records firings but suppresses notifications.
CREATE TABLE IF NOT EXISTS alert_rules (
    id                  TEXT    NOT NULL PRIMARY KEY,
    name                TEXT    NOT NULL,               -- human-readable display name
    metric              TEXT    NOT NULL,               -- e.g. rebuffer_ratio, bitrate_kbps
    operator            TEXT    NOT NULL,               -- gt, lt, gte, lte, eq
    threshold           REAL    NOT NULL,
    window_s            INTEGER NOT NULL,               -- evaluation window in seconds
    scope               TEXT    NOT NULL DEFAULT '{}',  -- JSON: node_id, app, stream_id filters
    severity            TEXT    NOT NULL DEFAULT 'warning',  -- info | warning | critical
    cooldown_s          INTEGER NOT NULL DEFAULT 300,
    group_by            TEXT,                           -- dimension key for grouping
    enabled             INTEGER NOT NULL DEFAULT 1,     -- 0=not evaluated, 1=active
    muted               INTEGER NOT NULL DEFAULT 0,     -- 0=notifications sent, 1=suppressed
    maintenance_windows TEXT    NOT NULL DEFAULT '[]',  -- JSON array
    channel_ids         TEXT    NOT NULL DEFAULT '[]',  -- JSON array of channel IDs
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
);

-- ── alert_channels ────────────────────────────────────────────
-- Notification channel configs. config_enc is AES-256-GCM encrypted.
-- config_public: non-secret display config (e.g. email address, channel name).
CREATE TABLE IF NOT EXISTS alert_channels (
    id          TEXT    NOT NULL PRIMARY KEY,
    type        TEXT    NOT NULL,              -- email | slack | telegram | pagerduty | webhook
    name        TEXT    NOT NULL,
    config_enc  TEXT,                          -- encrypted secret config (write-only)
    config_public TEXT  NOT NULL DEFAULT '{}', -- non-secret config fields (JSON)
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

-- ── alert_history ─────────────────────────────────────────────
-- Alert firing and resolution log.
CREATE TABLE IF NOT EXISTS alert_history (
    id              TEXT    NOT NULL PRIMARY KEY,
    alert_id        TEXT    NOT NULL,           -- firing instance ID (pairs firing/resolved)
    rule_id         TEXT    NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    state           TEXT    NOT NULL,           -- firing | resolved
    severity        TEXT    NOT NULL,
    ts              INTEGER NOT NULL,           -- Unix epoch ms
    metric          TEXT    NOT NULL,
    value           REAL,
    threshold       REAL,
    scope           TEXT    NOT NULL DEFAULT '{}',  -- JSON
    cooldown_until  INTEGER,                    -- Unix epoch ms
    group_key       TEXT,
    test            INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_alert_history_rule_ts ON alert_history(rule_id, ts);
CREATE INDEX IF NOT EXISTS idx_alert_history_alert_id ON alert_history(alert_id);

-- ── report_schedules ──────────────────────────────────────────
-- Scheduled report export configurations (F6).
-- scope: JSON {app, tenant} filters.
-- whitelabel_header: JSON object for PDF white-label (Phase 3).
CREATE TABLE IF NOT EXISTS report_schedules (
    id                  TEXT    NOT NULL PRIMARY KEY,
    cron                TEXT    NOT NULL,               -- cron expression (UTC)
    format              TEXT    NOT NULL DEFAULT 'csv', -- csv | pdf
    scope               TEXT    NOT NULL DEFAULT '{}',  -- JSON
    tenant_mapping      TEXT,                           -- tenant mapping rule reference
    whitelabel_header   TEXT,                           -- JSON; NULL until Phase 3
    last_run_at         INTEGER,
    next_run_at         INTEGER,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
);

-- ── tenants ───────────────────────────────────────────────────
-- Tenant definitions for multi-tenant billing (F6).
-- stream_pattern: SQL LIKE or regex pattern matching stream names.
-- meta_tag_key/value: match on beacon meta field.
CREATE TABLE IF NOT EXISTS tenants (
    id              TEXT    NOT NULL PRIMARY KEY,
    name            TEXT    NOT NULL UNIQUE,
    stream_pattern  TEXT,                           -- pattern match on stream_id
    meta_tag_key    TEXT,                           -- beacon meta field key
    meta_tag_value  TEXT,                           -- beacon meta field value
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

-- ── license ───────────────────────────────────────────────────
-- License key / tier state.
-- One row per installation (singleton; enforced by app layer).
-- signature: base64 ed25519 signature over key + claims.
-- offline_path: path to offline license file (Phase 3).
CREATE TABLE IF NOT EXISTS license (
    id              TEXT    NOT NULL PRIMARY KEY DEFAULT 'singleton',
    license_key     TEXT,                           -- raw license key (may be NULL for free)
    tier            TEXT    NOT NULL DEFAULT 'free', -- free | pro | enterprise
    signature       TEXT,                           -- ed25519 signature
    claims          TEXT    NOT NULL DEFAULT '{}',  -- JSON: limits, expiry, etc.
    offline_path    TEXT,                           -- path to offline license file
    valid           INTEGER NOT NULL DEFAULT 1,
    expires_at      INTEGER,
    activated_at    INTEGER,
    updated_at      INTEGER NOT NULL
);

-- ── probes ────────────────────────────────────────────────────
-- Synthetic stream probe configurations (F10).
-- Results are stored in ClickHouse (probe_results table) for efficient
-- time-range queries. This table holds config only.
-- last_result_id: reference to the most recent probe result (denormalized
-- for fast GET /probes listing without ClickHouse lookup).
CREATE TABLE IF NOT EXISTS probes (
    id              TEXT    NOT NULL PRIMARY KEY,
    name            TEXT    NOT NULL,
    url             TEXT    NOT NULL,               -- stream URL to probe
    protocol        TEXT,                           -- hls | webrtc | rtmp | dash
    interval_s      INTEGER NOT NULL DEFAULT 60,    -- probe interval in seconds
    timeout_s       INTEGER NOT NULL DEFAULT 10,
    enabled         INTEGER NOT NULL DEFAULT 1,
    last_result_id  TEXT,                           -- most recent ClickHouse result ID
    last_success    INTEGER,                        -- 0/1 from last run
    last_run_at     INTEGER,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

-- ── anomaly_baselines ─────────────────────────────────────────
-- Rolling-window statistics for anomaly detection (F9).
-- Stored in meta store because baselines are low-cardinality, mutated
-- in-place, and never queried with time-range predicates
-- (see INT-01 design note in clickhouse/0001_init.sql).
-- scope: JSON {node_id, app, stream_id} — NULL fields match all.
-- window_s: rolling window size in seconds used to compute stats.
CREATE TABLE IF NOT EXISTS anomaly_baselines (
    id          TEXT    NOT NULL PRIMARY KEY,
    metric      TEXT    NOT NULL,
    scope       TEXT    NOT NULL DEFAULT '{}',  -- JSON scope filter
    window_s    INTEGER NOT NULL,               -- rolling window in seconds
    mean        REAL    NOT NULL DEFAULT 0,
    stddev      REAL    NOT NULL DEFAULT 0,
    sample_count INTEGER NOT NULL DEFAULT 0,
    last_updated INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_anomaly_baselines_uniq
    ON anomaly_baselines(metric, scope, window_s);

-- ── Initial data ──────────────────────────────────────────────
-- Record this migration
INSERT OR IGNORE INTO schema_migrations(version, applied_at)
    VALUES ('0001', strftime('%s', 'now') * 1000);
