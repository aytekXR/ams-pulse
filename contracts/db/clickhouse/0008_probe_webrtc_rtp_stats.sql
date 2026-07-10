-- 0008_probe_webrtc_rtp_stats.sql
--
-- WO-B phase-2b: add rtt_ms, jitter_ms, loss_pct columns for WebRTC RTP statistics.
-- Owner: INT-01 (D-075 CR).
-- Executed by `pulse migrate` on startup after 0007_probe_webrtc_ice.sql.
--
-- Adds three WebRTC RTP statistics columns to probe_results:
--   rtt_ms    — RTT of the selected ICE candidate pair in ms (seconds→ms);
--               NULL = not measured (ICE not connected, or deadline expired during hold).
--   jitter_ms — inbound-RTP inter-arrival jitter in ms per RFC 3550;
--               NULL = not measured.
--   loss_pct  — inbound-RTP packet loss percent, 0-100; clamped >= 0;
--               NULL = not measured (no packets received or lost, or ICE not connected).
--
-- Nullable (vs the sentinel-default pattern of earlier columns) is deliberate:
--   0.0 is a valid measurement for all three metrics (zero loss, zero jitter,
--   zero RTT in loopback), so NULL is the only unambiguous "not measured" sentinel.
--   D-075 phase-2b.
--
-- Fresh AND existing installs both get these columns via this ALTER (0001's base
-- table predates them; the full migration sequence runs on fresh installs too).
-- IF NOT EXISTS ensures idempotency on repeated migrations.
--
-- Variables substituted by runner.go:
--   {db} — target ClickHouse database (e.g. "pulse")

ALTER TABLE {db}.probe_results ADD COLUMN IF NOT EXISTS rtt_ms Nullable(Float32);
ALTER TABLE {db}.probe_results ADD COLUMN IF NOT EXISTS jitter_ms Nullable(Float32);
ALTER TABLE {db}.probe_results ADD COLUMN IF NOT EXISTS loss_pct Nullable(Float32);
