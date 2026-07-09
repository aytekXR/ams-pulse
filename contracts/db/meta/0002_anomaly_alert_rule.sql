-- Migration 0002 — anomaly alert rule fields (WO-B S11).
-- Adds rule_type, sigma, min_samples to alert_rules.
-- rule_type DEFAULT 'threshold' preserves all existing rows as threshold rules.
-- sigma NULL → engine uses anomaly.DefaultSigma (4.0).
-- min_samples NULL → engine uses anomaly.MinSamples (30).
ALTER TABLE alert_rules ADD COLUMN rule_type    TEXT    NOT NULL DEFAULT 'threshold';
ALTER TABLE alert_rules ADD COLUMN sigma        REAL;
ALTER TABLE alert_rules ADD COLUMN min_samples  INTEGER;

INSERT OR IGNORE INTO schema_migrations(version, applied_at)
    VALUES ('0002', strftime('%s','now') * 1000);
