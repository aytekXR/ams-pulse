-- Pulse metadata store schema — migration 0001 (SKELETON).
-- Owner: BE-02. Runs on SQLite (single node, default) and Postgres (HA option);
-- keep DDL to the common subset both support.
--
-- Holds configuration and small relational state (PRD §7.10) — NEVER metrics.

-- TODO(BE-02): users              — local users, hashed credentials, roles (SSO in Phase 3)
-- TODO(BE-02): api_tokens         — bearer tokens for the Data API and beacon ingest
-- TODO(BE-02): ams_sources        — configured AMS endpoints (REST URL, credentials vaulted, log path, Kafka brokers)
-- TODO(BE-02): cluster_nodes      — discovered nodes cache with role labels (F7)
-- TODO(BE-02): alert_rules        — rule definitions (metric, condition, window, scope, severity)
-- TODO(BE-02): alert_channels     — channel configs (type + encrypted settings)
-- TODO(BE-02): alert_history      — firing/resolution log
-- TODO(BE-02): report_schedules   — scheduled exports (F6), tenant mapping rules
-- TODO(BE-02): license            — license key / signed offline license state

SELECT 1; -- placeholder
