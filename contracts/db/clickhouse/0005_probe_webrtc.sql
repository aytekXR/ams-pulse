-- Pulse ClickHouse schema — migration 0005
-- Owner: INT-01 (wave 3-plus, D-072 CR-1: WebRTC probe fields).
-- Executed by `pulse migrate` on startup after 0004_qoe_startup_quantile_fix.sql.
--
-- Adds WebRTC signaling probe columns to probe_results:
--   connect_time_ms   — ms from WebSocket dial to first server signaling message;
--                       0 when the probe is not WebRTC or connection failed.
--   signaling_state   — final WebRTC signaling state string (offer_received,
--                       ws_timeout, ws_refused, ws_error); empty for non-WebRTC.
-- Fresh installs get both columns from 0001; existing installs get them via ALTER.
-- IF NOT EXISTS ensures idempotency on repeated migrations.

ALTER TABLE {db}.probe_results ADD COLUMN IF NOT EXISTS connect_time_ms UInt32 DEFAULT 0;
ALTER TABLE {db}.probe_results ADD COLUMN IF NOT EXISTS signaling_state LowCardinality(String) DEFAULT '';
