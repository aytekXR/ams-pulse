-- 0007_probe_webrtc_ice.sql
--
-- WO-B phase-2a: add ice_state column for WebRTC ICE negotiation outcome.
-- Owner: INT-01 (D-074 CR).
-- Executed by `pulse migrate` on startup after 0006_probe_results_ttl.sql.
--
-- Adds the WebRTC ICE state column to probe_results:
--   ice_state — terminal ICE state of the WebRTC media path check;
--               empty string for non-WebRTC probes or when ICE was not attempted.
--               Values: "connected" | "failed" | "timeout" | "" (key absent in API).
-- LowCardinality(String) matches the storage pattern used for signaling_state (0005).
-- Fresh installs get this column from 0001; existing installs get it via ALTER.
-- IF NOT EXISTS ensures idempotency on repeated migrations.
--
-- Variables substituted by runner.go:
--   {db} — target ClickHouse database (e.g. "pulse")

ALTER TABLE {db}.probe_results ADD COLUMN IF NOT EXISTS ice_state LowCardinality(String) DEFAULT '';
