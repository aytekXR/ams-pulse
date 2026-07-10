-- Pulse metadata store schema — migration 0002 (PostgreSQL)
-- Source of truth: contracts/db/meta/0002_anomaly_alert_rule.sql (SQLite)
-- This file is the faithful PostgreSQL translation of that file.
-- Keep both files in sync: any structural change to the SQLite original
-- must be mirrored here.
--
-- Mapping rules (SQLite → PostgreSQL):
--   INSERT OR IGNORE         → INSERT ... ON CONFLICT (<pk_column>) DO NOTHING
--   strftime('%s','now')*1000 → (EXTRACT(EPOCH FROM NOW())::bigint * 1000)
--   REAL                     → DOUBLE PRECISION
--   INTEGER (general)        → INTEGER (unchanged for these columns)
--   No PRAGMA lines in this migration (source has none either).

-- Migration 0002 — anomaly alert rule fields (WO-B S11).
-- Adds rule_type, sigma, min_samples to alert_rules.
-- rule_type DEFAULT 'threshold' preserves all existing rows as threshold rules.
-- sigma NULL → engine uses anomaly.DefaultSigma (4.0).
-- min_samples NULL → engine uses anomaly.MinSamples (30).
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS rule_type    TEXT             NOT NULL DEFAULT 'threshold';
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS sigma        DOUBLE PRECISION;
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS min_samples  INTEGER;

-- Record this migration (same version string as SQLite original for parity checks).
INSERT INTO schema_migrations(version, applied_at)
    VALUES ('0002', (EXTRACT(EPOCH FROM NOW())::bigint * 1000))
    ON CONFLICT (version) DO NOTHING;
