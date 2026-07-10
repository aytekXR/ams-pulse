-- 0006_probe_results_ttl.sql
--
-- Fix: probe_results TTL now respects {retention_days} (D-072 verifier finding #6).
--
-- Background: 0001_init.sql originally hardcoded toIntervalDay(90) for the
-- probe_results table while all other raw tables already used the {retention_days}
-- placeholder.  This migration repairs existing deployments where 0001 was already
-- applied with the hardcoded value.
--
-- For fresh installs the fix in 0001 is sufficient; this ALTER is a safe no-op
-- when the current TTL expression already matches (same value, different path).
--
-- Variables substituted by runner.go:
--   {db}             — target ClickHouse database (e.g. "pulse")
--   {retention_days} — raw event retention in days (Config.RetentionDays)

ALTER TABLE {db}.probe_results MODIFY TTL toDate(ts) + toIntervalDay({retention_days});
