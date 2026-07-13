# Capability Map — AMS Universe × Pulse Coverage

**Phase:** 1 — Product Understanding
**Produced:** S16 close (2026-07-11)

This document maps every verified AMS capability against the current Pulse
implementation to establish the supported / partial / missing baseline before
active validation begins. Every AMS claim cites a real capture or source;
every Pulse claim cites a repo path.

---

## Coverage Legend

| Symbol | Meaning |
|--------|---------|
| FULL | End-to-end pipeline: AMS → poll/webhook → normalize → ClickHouse/in-mem → Pulse API → UI |
| PARTIAL | Pipeline exists but with documented gaps, approximations, or missing UI surface |
| MISSING | AMS provides the data; Pulse does not consume, store, or expose it |
| N/A | AMS capability not applicable to a monitoring product (e.g., AMS-side config only) |
| UNKNOWN | Not confirmed either way; needs live validation |

---

## 1. Broadcast Lifecycle

**AMS sources:**
- `GET /{app}/rest/v2/broadcasts/list/{offset}/{size}` — primary poll
  (agents/handoffs/real-ams-captures/LiveApp_list.json)
- States on wire: `created`, `broadcasting`, `finished`,
  `terminated_unexpectedly`
- `POST /{app}/rest/v2/broadcasts/create`, `PUT …/{id}`, `DELETE …/{id}`
- Webhook events: `liveStreamStarted`, `liveStreamEnded`, `vodReady`
  (docs/AMS-INTEGRATION.md)

**Pulse pipeline:**
- Poll: `server/internal/collector/restpoller/restpoller.go` every 5 s
  (`DefaultPollInterval`, restpoller.go:26)
- Normalization: `server/internal/collector/normalize.go` —
  `NormalizeBroadcast()`
- State translation: `terminated_unexpectedly` → `EventStreamPublishEnd`
  with `reason=terminated_unexpectedly`
  (restpoller.go:detectEnded, lines 222–265)
- Webhook path: `server/internal/collector/webhook/webhook.go` (fail-closed;
  AMS 3.0.3 cannot sign — unsigned-mode not yet built per O3 decision)
- In-memory aggregate: `LiveStreamList` via `getLiveStreams` endpoint
- Stored in: `server_events` table — columns `event_type`, `stream_id`,
  `app`, `ts` (ClickHouse, migration 0001_init.sql)
- Pulse API: `GET /api/v1/live/streams`, `GET /api/v1/live/overview`

**Coverage: FULL (REST path); PARTIAL (webhook)**

**Gaps / Assumptions to Validate:**
- `terminated_unexpectedly` status appears in real AMS on encoder crash.
  Validate: kill the ffmpeg publisher mid-stream and confirm Pulse emits
  `stream_publish_end` event (scenario TC-F-02).
- Webhook path is untested against real AMS because AMS 3.0.3 cannot
  HMAC-sign hooks (O3 closed-N/A). Validate: confirm poll path detects
  stream-end within 10 s of actual stop (the 5 s poll + 5 s processing
  budget).
- Multi-page pagination: `pageSize=200` is hardcoded in
  `amsclient.ListBroadcastsPaged()`. If an AMS app has >200 concurrent
  streams, the poll loop iterates pages. Validate this with the
  bulk-publish scenario (500 streams) used in CI WO-4 — but against real
  AMS, not mock.

---

## 2. Viewer Counts

### 2a. Inline Broadcast Counts (5 s poll)

**AMS source fields (BroadcastDTO):**
`hlsViewerCount`, `webRTCViewerCount`, `rtmpViewerCount`, `dashViewerCount`
(agents/handoffs/real-ams-captures/LiveApp_list.json)

**Pulse pipeline:**
- `normalize.go` lines 83–91: sum → `viewer_count`; per-protocol breakdown
  stored as `vc_webrtc`, `vc_hls`, `vc_rtmp`, `vc_dash`
- `server_events` columns: `viewer_count`, `vc_webrtc`, `vc_hls`,
  `vc_rtmp`, `vc_dash`
- Pulse API: `GET /api/v1/live/overview` → `total_viewers`;
  `GET /api/v1/live/streams` → per-stream viewer breakdown

**Coverage: FULL**

**Known semantics (must be disclosed in docs):**
- RTMP pull viewers — two DIFFERENT AMS fields, do not conflate:
  `totalRTMPWatchersCount = -1` ("untracked") appears only in the
  dedicated statistics endpoint capture (`broadcast-statistics_test123.json`)
  — an endpoint Pulse never calls (dead code, §2b). The inline
  `rtmpViewerCount` in the BroadcastDTO poll path — the field Pulse
  actually consumes — is `0` in all real captures. **normalize.go does
  NOT clamp negative counts**: `normalize.go:83` is a plain sum
  (`HlsViewerCount + WebRTCViewerCount + RTMPViewerCount + DashViewerCount`).
  If the inline field can ever be negative on a real deployment, the
  summed `viewer_count` would silently corrupt → AV-16.
- HLS counts are segment-request-based; CDN-cached segments are not
  counted. Pulse documentation must disclose this.
- WebRTC phantom viewer bug (GitHub #4839, v2.5.3): cluster mode may
  report +1 WebRTC viewer per published stream even with zero real
  viewers. Standalone (test server) is unaffected. Validate on cluster
  mode if available.

### 2b. Dedicated Statistics Endpoint

**AMS source:** `GET /{app}/rest/v2/broadcasts/{id}/broadcast-statistics`
(agents/handoffs/real-ams-captures/broadcast-statistics_test123.json)

**Pulse pipeline:**
- `amsclient.BroadcastStatistics()` was dead code (no runtime caller —
  BUG-001) and was **DELETED in S26/D-088**. Viewer counts come from the
  inline BroadcastDTO fields (§2a), validated within ±2% (TC-V-03).
- The endpoint's real-AMS wire shape (incl. `totalRTMPWatchersCount=-1` =
  untracked) stays documented in
  `agents/handoffs/real-ams-captures/broadcast-statistics_test123.json`;
  the qa/mock-ams `/statistics` stub is retained to mirror the real
  surface.

**Coverage: NOT CONSUMED (deliberate — S26/D-088, BUG-001 FIXED; inline
counts cover the need with one list call instead of N per-stream calls)**

**Assumptions to Validate:** none remaining — the no-caller assumption was
confirmed and resolved by deletion (S26); inline-count accuracy was
validated ±2% (TC-V-03, S17).

---

## 3. WebRTC Per-Peer Client Stats

**AMS source:** `GET /{app}/rest/v2/broadcasts/{id}/webrtc-client-stats/0/100`
(agents/handoffs/real-ams-captures/webrtc-client-stats_test123.json —
returns `[]` with no live viewers)

**AMS fields:** `statId`, `videoRoundTripTime` (seconds), `audioRoundTripTime`
(seconds), `videoJitter` (seconds), `audioJitter` (seconds),
`videoPacketLostRatio` (0..1), `audioPacketLostRatio` (0..1),
`outboundRtpList`, `inboundRtpList`

**Pulse pipeline:**
- `server/pkg/amsclient/client.go:494` — `WebRTCClientStats()`
- Polled **conditionally**: only when `status == "broadcasting"` AND
  `webRTCViewerCount > 0` (restpoller.go:pollApp, confirmed)
- Normalization: `normalize.go:NormalizeWebRTCStats()` lines 163–190
  - RTT: `avgNonZero(videoRoundTripTime, audioRoundTripTime) × 1000` → ms
  - Jitter: `avgNonZero(videoJitter, audioJitter) × 1000` → ms
  - Packet loss: `avgNonZero(video…Ratio, audio…Ratio) × 100` → percent
- Stored in: `server_events` columns `rtt_ms`, `jitter_ms`,
  `packet_loss_pct` (ClickHouse, migration 0001_init.sql)
- Pulse API: `GET /api/v1/qoe/ingest` — `IngestStream.rtt_ms`,
  `jitter_ms`, `packet_loss_pct`

**Coverage: PARTIAL**

**Gaps / Assumptions to Validate:**
- The endpoint returns `[]` when no WebRTC viewers are active (confirmed
  from real capture). Validation requires a stream with at least one live
  WebRTC viewer.
- Maximum 100 peers per call (`/webrtc-client-stats/0/100`). If >100
  viewers exist, additional pages are silently dropped. Validate the
  behavior at scale.
- Unit conversion (seconds → ms ×1000) needs live confirmation: simulate
  a WebRTC viewer and compare the raw AMS value to the Pulse API value.
  One bad ×1000 vs ×0.001 flip ruins the metric entirely.

---

## 4. RTMP / SRT / WHIP Ingest Metrics

**AMS source fields (BroadcastDTO, inline):**
`bitrate` (bits/sec), `speed` (ratio ~1.0), `packetLostRatio` (0..1),
`packetsLost`, `jitterMs` (ms), `rttMs` (ms), `encoderQueueSize`,
`dropPacketCountInIngestion`, `dropFrameCountInEncoding`, `currentFPS`
(ABSENT in AMS 3.0.3 REST)

**Pulse pipeline:**
- `normalize.go:NormalizeBroadcast()`:
  - `bitrate_kbps = BroadcastDTO.BitRate / 1000.0` (bits/sec → kbps,
    normalize.go:79, curl-verified)
  - `speed_read_kbps = BroadcastDTO.Speed` — MISLEADING: this is the
    AMS realtime ratio (~1.0), NOT kbps. Legacy key retained
    (normalize.go:92 comment)
  - `packet_loss_pct = PacketLostRatio × 100.0` (normalize.go:117)
  - `fps` column: always 0 for REST-polled deployments (AMS 3.0.3
    BroadcastDTO does not include `currentFPS`)
- Health score: `ComputeHealthScore(bitrate_kbps, fps, packet_loss_pct,
  jitter_ms, rtt_ms)` — FPS weight redistributed when fps=0
- Stored: `ingest_stats` events in `server_events` table
- Pulse API: `GET /api/v1/qoe/ingest` — `IngestStream` with
  `health_score`, `bitrate_kbps`, `packet_loss_pct`, `rtt_ms`,
  `jitter_ms`, drop counts

**Coverage: PARTIAL**

**Gaps:**
- `currentFPS` always 0 from REST (AMS 3.0.3 does not include it in
  BroadcastDTO; confirmed by comment at client.go:97). FPS health
  scoring weight is redistributed — test that health_score is still
  meaningful without fps.
- `speed_read_kbps` column name is misleading (stores ratio, not kbps).
  Downstream consumers parsing this key literally would misinterpret it.
  Scheduled for cleanup (open_question from scout A).
- `dropPacketCountInIngestion` and `dropFrameCountInEncoding` are in
  BroadcastDTO — check if they are stored in Pulse events or silently
  dropped.
- SRT and WHIP ingest use the same BroadcastDTO so these metrics apply,
  but SRT-specific packet loss (at-protocol level before AMS receives it)
  is not separately instrumented.

**Kafka alternative:** `ams-instance-stats` Kafka topic carries
CPU/memory fields that REST system-status omits. `PULSE_KAFKA_BROKERS`
env var activates the consumer (`server/internal/collector/kafka/`).
**Coverage of Kafka path: UNKNOWN** — not explored in S16 scouting.

---

## 5. System Health — Standalone Node

**AMS source:** `GET /rest/v2/system-status`
(agents/handoffs/real-ams-captures/system-status.json:
`{osName:Linux, osArch:amd64, javaVersion:17, processorCount:8}`)

**Critical gap:** AMS 3.x `/rest/v2/system-status` does NOT return
`cpu_pct`, `mem_pct`, `disk_pct`, or network metrics. This is a real AMS
wire behavior, not a Pulse bug.

**Pulse pipeline:**
- `amsclient.SystemStats()` at client.go:535 — parses the raw map
- `normalize.go:NormalizeSystemStats()` lines 206–251 — HONEST: does NOT
  fabricate zeros for absent CPU/mem/disk fields. Only osName, osArch,
  javaVersion, processorCount are stored.
- Fallback path: called only when `ClusterNodes()` returns 0 nodes
  (standalone detection)
- Pulse API: Fleet node card shows OS/JVM version; CPU/mem/disk gauges
  are empty for standalone deployments

**Coverage: PARTIAL** (os/jvm shown; resource gauges unavailable without
Kafka or cluster mode)

**Assumptions to Validate:**
- Confirm that Fleet page shows a node card for the standalone AMS
  (161.97.172.146:5080) with OS/version data populated.
- Confirm that CPU/mem/disk fields are absent (blank/null) in the Fleet
  API response, not falsely zero.

---

## 6. Cluster Node Health

**AMS source:** `GET /rest/v2/cluster/nodes` — `ClusterNodeDTO` with
`cpuUsage`, `memoryUsage`, `diskUsage`, `networkInputBps`,
`networkOutputBps`, `jvmMemoryUsage`, `activeStreamCount`, `role`
(server/pkg/amsclient/client.go:134–149)

**Pulse pipeline:**
- `client.go:505` — `ClusterNodes()` returns `(nil, nil)` on 404
  (standalone). This is the real AMS behavior.
- `normalize.go:NormalizeClusterNode()` lines 255–279:
  - `net_in_mbps = NetworkInputBps / 1,000,000`
  - `net_out_mbps = NetworkOutputBps / 1,000,000`
- Stored: `server_events` columns `cpu_pct`, `mem_pct`, `disk_pct`,
  `net_in_mbps`, `net_out_mbps`, `jvm_heap_mb`
- Cluster discovery: `server/internal/cluster/discovery.go` — 30 s
  interval
- Pulse API: `GET /api/v1/fleet/nodes` — `FleetNodeList`

**Coverage: FULL (cluster mode); NOT APPLICABLE (standalone — 404 path)**

**Assumptions to Validate:**
- The test AMS is standalone. Confirm `GET /api/v1/fleet/nodes` returns
  an empty list or a single synthetic node derived from `system-status`.
- If cluster nodes returns 404, confirm no error is logged as a warning
  (it should be silent, per client.go:511 `(nil, nil)` handling).

---

## 7. Application Management

**AMS source:**
- `GET /rest/v2/applications` — string-array envelope (confirmed:
  agents/handoffs/real-ams-captures/applications.json — 16 app names)
- `GET /rest/v2/applications/info` — per-app `liveStreamCount`,
  `vodCount`, `storage` bytes
  (agents/handoffs/real-ams-captures/applications-info.json)

**Pulse pipeline:**
- `amsclient.ListApplications()` at client.go:405 — handles both AMS v3
  string-array and older `{name}` object-array forms
- Polled every 5 s via `restpoller.resolveApps()`
- Used internally to drive per-app broadcast polling
- Not directly surfaced as a Pulse API endpoint (no `/api/v1/apps` route)

**Coverage: PARTIAL** (internal use; not exposed to Pulse API consumers)

**Assumptions to Validate:**
- Confirm 16 apps are discovered. Confirm that only apps in
  `PULSE_AMS_APPLICATIONS` (if set) are polled.
- Confirm that apps with `remoteAllowedCIDR=127.0.0.1` return HTTP 403
  and Pulse logs a warning rather than crashing or silently dropping
  streams.
- The open apps that Pulse can poll from the VPS IP:
  `24x7test`, `Conference`, `LiveApp`, `LiveShopping`, `PetarTest2`,
  `clipcreator`, `demo`, `meet`
  (from scout B `local_env.open_ams_apps`).

---

## 8. VoD / Recording Management

**AMS source:**
- `applications-info.json`: `vodCount`, `storage` per app
- `vodReady` webhook event (path, size_bytes)
- `GET /{app}/rest/v2/vods/list/{offset}/{size}`

**Pulse pipeline:**
- Webhook: `webhook.go:translateWebhook()` — `vodReady` →
  `domain.EventRecordingReady` with `path`, `size_bytes`
- Stored: `server_events` — `event_type=recording_ready`,
  `recording_path`, `recording_size`, `recording_dur_s`
- VoD list REST endpoint is not polled
- `rollup_usage_1d` stores `recording_bytes` (from webhook events)
- Pulse API: `GET /api/v1/reports/usage` — `recording_gb` field

**Coverage: PARTIAL** (webhook path; VoD REST polling not implemented)

**Gaps:**
- VoD events only arrive via webhook. Since AMS 3.0.3 cannot HMAC-sign
  hooks and the webhook listener is fail-closed, VoD recording events
  will NOT reach Pulse in the current prod deployment. This is a
  significant gap for the recording billing use case.
- Validate: does `GET /api/v1/reports/usage` show `recording_gb > 0`
  on the prod AMS that has VoD assets in WebRTCAppEE (1006 VoDs,
  ~24 GB from applications-info.json)? Expected: 0 (no webhook delivery
  without signed hook or polling fallback).

---

## 9. Webhooks / Lifecycle Events

**AMS source:** Outbound POST to `listenerHookURL`. Events:
`liveStreamStarted`, `liveStreamEnded`, `vodReady` (docs/AMS-INTEGRATION.md)

**Pulse pipeline:**
- `server/internal/collector/webhook/webhook.go` — fail-closed
  HMAC-SHA256 validator
- CRITICAL: AMS 3.0.3 cannot HMAC-sign hooks (O3 decision: closed-N/A,
  decisions.md:2404–2410). Empty `SharedSecret` rejects all requests.
  Therefore, **the webhook path is entirely unused** in the current prod
  deployment.

**Coverage: MISSING (effectively, for prod AMS 3.0.3)**

**Assumptions to Validate:**
- Confirm webhook listener is configured in Caddy route
  (`Caddyfile.prod`) and whether any events are arriving.
- Check Pulse logs: `grep "webhook" pulse logs` for signature failure
  messages.
- Document whether REST polling covers the same events within acceptable
  latency (expected: yes for stream start/end; no for vodReady since VoD
  list is not polled).

---

## 10. Beacon SDK / QoE

**AMS relevance:** Beacon SDK is embedded in the video player page (HLS,
WebRTC, native), not in AMS itself. AMS provides the stream; the beacon
SDK measures the viewer-side experience.

**Pulse pipeline:**
- SDK: `sdk/beacon-js/src/` — player adapters for `ams-webrtc`, `hls.js`,
  `video.js`, `native`
- Ingest: `POST /ingest/beacon` — `X-Pulse-Ingest-Token` header; rate
  limit 100 rps/token; body limit 64 KB
- License gate: Enterprise/Pro required; Free tier returns 403
  (`beacon.go:84–88`)
- Stored: `beacon_events` table; aggregated to `viewer_sessions` and
  `rollup_qoe_1h/1d`
- Pulse API: `GET /api/v1/qoe/summary` — startup p50/p95, rebuffer_ratio,
  error_rate

**Coverage: FULL** (pipeline proven by CI test A2; real-viewer validation
is a gap — see section below)

**Assumptions to Validate:**
- Real viewer session: embed beacon SDK in a test page served by AMS,
  play a live stream in a headless browser, verify beacon events appear
  in `GET /api/v1/qoe/summary` within the 120 s ClickHouse rollup
  latency window.
- Verify `player_kind=ams-webrtc` adapter fires `startup_complete` on
  first video frame (ICE connected + first RTP packet received).

---

## 11. Alert System

**Pulse implementation:**
- Threshold rules: viewer_count, bitrate_kbps, cpu_pct, mem_pct,
  disk_pct (live snapshot); rebuffer_ratio, error_rate (ClickHouse
  rollup, Pro+ only)
- Anomaly rules: viewer_count, ingest_bitrate_kbps, cpu_pct, mem_pct,
  disk_pct — Welford rolling baseline, 1-hour window
- Channels: email, Slack, PagerDuty, generic webhook (HMAC-signed),
  Telegram
- Evaluation: every 5 s; cooldown 300 s default
- Files: `server/internal/alert/evaluator.go`, `wave2.go`, `wave3.go`

**AMS source used:** live snapshot from REST poll (viewer_count,
bitrate_kbps, node CPU/mem/disk where cluster mode available)

**Coverage: FULL** (CI-verified by A1–A5b scenarios)

**Assumptions to Validate:**
- `ingest_bitrate_floor lt <threshold>` alert fires within 15 s of
  bitrate drop on real AMS stream (not mock).
- `viewer_count anomaly` fires after a real viewer ramp (not a
  `/control/set_viewers` injection).
- cpu/mem/disk anomaly: standalone AMS does not provide these from REST;
  anomaly detector has no data to baseline against. Confirm
  `GET /api/v1/anomalies` shows no cpu/mem/disk findings for standalone.

---

## 12. Prober (Synthetic Probes)

**Pulse implementation:**
- HLS, DASH, WebRTC, RTMP probes
  (`server/internal/prober/` — `prober.go`, `probe_dash.go`,
  `probe_webrtc_ice.go`, `probe_rtmp.go`)
- Results in ClickHouse `probe_results` table
- Pulse API: `GET /api/v1/probes/{id}/results`

**AMS source:** Probers target AMS protocol endpoints directly (not the
REST management API). They exercise:
- `ws://{ams}/{app}/websocket?streamId=<id>` — WebRTC signaling
- `rtmp://{ams}:1935/{app}/{streamId}` — RTMP handshake
- `http://{ams}:5080/{app}/streams/{streamId}.mpd` — DASH (404 on test
  AMS, DASH muxing disabled)
- `http://{ams}:5080/{app}/streams/{streamId}/playlist.m3u8` — HLS

**Coverage: FULL (WebRTC, RTMP, HLS); MISSING (DASH — disabled on AMS)**

**Critical bug fixed (D-074/D-075):** AMS sends a `notification`
(`subtrackAdded`) message BEFORE the `takeConfiguration` offer. The
WebRTC probe was failing against real AMS because it expected the offer
first. Fixed and proven via WO-B CI scenario.

**Assumptions to Validate:**
- Point a WebRTC probe at `ws://161.97.172.146:5080/LiveApp/websocket`
  with a live `streamId` and confirm `signaling_state=offer_received`,
  `ice_state=connected` in `GET /api/v1/probes/{id}/results`.
- HLS probe against `http://161.97.172.146:5080/LiveApp/streams/{id}/playlist.m3u8`
  — confirm `success=true`, `ttfb_ms > 0`, `bitrate_kbps > 0`.
- RTMP probe against real `rtmp://161.97.172.146:1935/LiveApp` — confirm
  `signaling_state=handshake_complete`.
- DASH: expect `success=false` with `error_code=http_4xx` (DASH not
  enabled). Document this as a known limitation.

---

## 13. Anomaly Detection

**Pulse implementation:**
- Welford online algorithm, 1-hour rolling window (fixed)
- Min 30 samples, 4σ default, 10-tick hysteresis
- Signals: `viewers`, `ingest_bitrate_kbps`, `cpu_pct`, `mem_pct`,
  `disk_pct` (per stream or node), `ams_api_latency_ms` (node; Pulse-measured
  poller RTT — D-087)
- Files: `server/internal/anomaly/anomaly.go`
- Gate: `GET /api/v1/anomalies` — Enterprise tier only

**NOT tracked by anomaly detector:**
- `error_rate` (from beacon rollups) — filed as F9 finding-1
- `rebuffer_ratio` (from beacon rollups) — filed as F9 finding-1

**Coverage: PARTIAL** (stream/node metrics tracked; viewer QoE metrics not
tracked by detector)

**Assumptions to Validate:**
- With a live AMS stream and real viewers: does `viewers` anomaly fire
  when viewer count spikes? Requires patience (30 samples × 60 s tick =
  30 min baseline warmup in prod; PULSE_ANOMALY_TICK_S=5 is CI-only).
- For standalone AMS: cpu_pct/mem_pct/disk_pct baselines will be empty
  (no data source). Confirm `GET /api/v1/anomalies` returns an empty
  list for node metrics on standalone, not an error.

---

## 14. Geo / Device Analytics

**AMS source:** Not applicable — AMS does not provide viewer geo/device
data. Geo is derived from viewer IP (from beacon ingest or WebRTC client
stats `remoteIp` field).

**Pulse pipeline:**
- `viewer_sessions` table: `geo_country`, `geo_region`, `client_device`,
  `client_os`, `client_browser`
- IP hash: `SHA-256(viewer_ip)` for privacy (normalize.go:281)
- Rollups: `rollup_audience_1h/1d` — `geo_country`, `client_device`,
  `protocol` dimensions
- GeoLite2-City.mmdb required for IP-to-geo enrichment
- Pulse API: `GET /api/v1/analytics/geo`, `GET /api/v1/analytics/devices`

**Coverage: PARTIAL** (pipeline defined; requires real beacon traffic and
GeoLite2 DB for non-zero results)

**Assumptions to Validate:**
- Is `GeoLite2-City.mmdb` present in the prod container? Check:
  `docker exec pulse find / -name GeoLite2-City.mmdb 2>/dev/null`
- Without GeoLite2: `geo_country` column should be empty string or
  "unknown", not an error.
- With beacon events from a real browser session: confirm
  `GET /api/v1/analytics/geo` returns non-empty country data.

---

## 15. Usage / Billing Reports

**Pulse pipeline:**
- `rollup_usage_1d` — `viewer_minutes`, `peak_concurrency`,
  `egress_bytes` (always 0 — hardcoded in `mv_usage_1d`),
  `recording_bytes` (from webhook events, currently 0 because webhooks
  don't reach Pulse)
- Pulse API: `GET /api/v1/reports/usage` — `viewer_minutes`,
  `peak_concurrency`, `egress_gb`, `recording_gb`

**Coverage: PARTIAL**

**Gaps:**
- `egress_bytes` is hardcoded 0 in `mv_usage_1d`. AMS does not expose
  per-stream egress bytes directly; this would require CDN log parsing.
  Marked as unimplemented in scout A (open question 8).
- `recording_bytes` is 0 because webhook delivery is blocked.
- `viewer_minutes` is derived from beacon `watch_time_s` — requires real
  beacon sessions.

---

## Summary Table

| Capability | Coverage | Primary Gap |
|------------|----------|-------------|
| Broadcast lifecycle (states) | FULL | terminated_unexpectedly validation |
| Inline viewer counts | FULL | CDN undercount disclosure |
| BroadcastStatistics endpoint | MISSING | Dead code, never called |
| WebRTC per-peer stats | PARTIAL | Requires live WebRTC viewers |
| RTMP ingest metrics | PARTIAL | currentFPS always 0 |
| SRT ingest metrics | PARTIAL | Same as RTMP via BroadcastDTO |
| System health (standalone) | PARTIAL | No CPU/mem from REST |
| Cluster node health | FULL (cluster) | N/A for standalone test AMS |
| Application management | PARTIAL | Not exposed via Pulse API |
| VoD / Recording | PARTIAL | Webhook blocked; VoD REST not polled |
| Webhooks | MISSING (prod) | AMS 3.0.3 can't sign hooks |
| Beacon / QoE | FULL (pipeline) | Real viewer session not validated |
| Alert system | FULL | Standalone CPU/mem alerts empty |
| Prober (WebRTC/RTMP/HLS) | FULL | DASH disabled on test AMS |
| Prober (DASH) | MISSING (prod) | DASH muxing not enabled |
| Anomaly detection | PARTIAL | error_rate/rebuffer_ratio not tracked |
| Geo analytics | PARTIAL | Requires GeoLite2 DB + real beacon |
| Device analytics | PARTIAL | Requires real beacon sessions |
| Usage / billing report | PARTIAL | egress_bytes=0; recording_bytes=0 |
| Kafka CPU/mem (standalone) | UNKNOWN | Not validated in S16 |

---

## Assumptions-to-Validate Master List

| ID | Assumption | Scenario that tests it |
|----|-----------|----------------------|
| AV-01 | `terminated_unexpectedly` status is detected and emits `publish_end` | TC-F-02 (kill publisher) |
| AV-02 | BroadcastStatistics caller is confirmed absent at runtime | Code grep + API delta check |
| AV-03 | WebRTC RTT/jitter unit conversion (×1000) is correct | TC-W-01 (live WebRTC viewer) |
| AV-04 | `currentFPS=0` does not degrade health_score to 0 | TC-I-01 (bitrate drop scenario) |
| AV-05 | Fleet page shows OS/JVM version for standalone AMS | TC-FL-01 (fleet validation) |
| AV-06 | Fleet page does NOT show false-zero CPU/mem for standalone | TC-FL-01 |
| AV-07 | IP-blocked AMS apps log a warning, don't crash the poller | TC-APP-01 (multi-app validation) |
| AV-08 | Webhook events are not reaching Pulse (fail-closed, AMS unsigned) | TC-WH-01 (webhook audit) |
| AV-09 | VoD recording_gb=0 in reports because webhook is blocked | TC-VOD-01 |
| AV-10 | GeoLite2-City.mmdb present in prod container | `docker exec` inspection |
| AV-11 | Geo analytics empty (not error) without real beacon traffic | TC-GEO-01 |
| AV-12 | Anomaly detector returns empty for standalone CPU/mem/disk | TC-ANO-01 |
| AV-13 | DASH probe returns http_4xx (muxing disabled), not a crash | TC-P-04 (DASH probe) |
| AV-14 | WebRTC probe works against live AMS stream (D-074 fix holds) | TC-P-01 (WebRTC probe) |
| AV-15 | Kafka path activates when PULSE_KAFKA_BROKERS set | Not scheduled (S17+) |
| AV-16 | Inline `rtmpViewerCount` (BroadcastDTO poll path) is never negative on live AMS — normalize.go:83 sums WITHOUT clamping, so a negative value would corrupt `viewer_count`; if observed negative, file a Pulse bug to add clamping | TC-V-04 + live poll sampling |

---

## AV Triage — S17 (2026-07-11, live)

All checks executed against: Pulse prod `https://beyondkaira.com/api/v1` (token from
oguz-testing.md:159) and AMS `http://161.97.172.146:5080`. AMS version confirmed:
3.0.3 Enterprise Edition build 20260504\_1443. One cookie login performed; cookie
written to scratchpad only.

| AV | Verdict | Evidence — file / command + observed value |
|----|---------|-------------------------------------------|
| AV-01 | SCHEDULED — TC-F-02 | Kill-publisher scenario; requires live ffmpeg publisher to crash-detect `terminated_unexpectedly`. Covered by TC-F-02 this session. |
| AV-02 | CONFIRMED | `grep -rn "BroadcastStatistics" server/` + `codegraph explore "BroadcastStatistics callers"`: method defined at `client.go:483`; only caller is `client_test.go:625` (a test). No caller in restpoller, normalize, or any runtime code path. Dead code at runtime confirmed. |
| AV-03 | SCHEDULED — TC-W-01 | RTT/jitter unit-conversion requires a live WebRTC viewer session. Covered by TC-W-01. |
| AV-04 | SCHEDULED — TC-I-01 | fps=0 health-score behavior tested by bitrate-drop scenario TC-I-01. |
| AV-05 | CONFIRMED | `GET /api/v1/fleet/nodes` → `{node_id:"beyondkaira-ams", role:"standalone", status:"up", version:"3.0.3", os_name:"Linux", os_arch:"amd64", java_version:"17", processor_count:6}`. OS/JVM populated as expected. |
| AV-06 | CONFIRMED | Same response as AV-05. Fields `cpu_pct`, `mem_pct`, `disk_pct`, `net_in_mbps`, `net_out_mbps` are **absent** (not present in JSON). No false zeros for standalone node. |
| AV-07 | SCHEDULED — TC-APP-01 | S16 had 8+ open apps and some IP-blocked. S17 shows only 4 apps (LiveApp, WebRTCAppEE, live, pulse-test), all returning HTTP 200 — no 403 apps currently exist. TC-APP-01 may need a synthetic 403 fixture. |
| AV-08 | CONFIRMED | `Caddyfile.prod` lines 55–60: `/webhook/*` route present, proxies to `pulse:8092`. `docker logs pulse-prod-pulse-1 \| grep -i webhook` → 2 lines, startup only (`"webhook: listening","addr":":8092","per_source_secrets":0`). Zero delivery events logged. AMS 3.0.3 is not sending lifecycle hooks (cannot HMAC-sign per O3 decision). |
| AV-09 | CONFIRMED | `GET /api/v1/reports/usage` → `recording_gb:0`. VoD bytes not tracked (no webhook delivery). Note: `egress_gb:0.0025` via `bitrate_x_watch_time` estimate (unrelated to webhooks); `viewer_minutes:0.3333` from one beacon session. |
| AV-10 | CONFIRMED ABSENT | `docker exec pulse-prod-pulse-1 sh -c 'find / -name GeoLite2*.mmdb 2>/dev/null'` → no output. Fallback: `/var/lib/pulse/` contains only `pulse_meta.db{,-shm,-wal}`. `PULSE_GEO_MMDB_PATH` commented out in `deploy/.env`. GeoLite2-City.mmdb absent from prod container. |
| AV-11 | CONFIRMED (nuanced) | `GET /api/v1/analytics/geo` → `[{country:"",views:1,uniques:1,watch_time_s:10}]`. Not an error — returns valid JSON. One beacon event exists (D-076 QoE live, enterprise license), but `country=""` because GeoLite2 DB absent (AV-10). Response is non-empty, not empty-as-expected (prod has real traffic). Empty country is the correct behaviour without the mmdb. |
| AV-12 | CONFIRMED | `GET /api/v1/anomalies` → `{items:[],meta:{next_cursor:null}}`. Empty list. No cpu/mem/disk anomaly findings for standalone node (no data source to baseline against). |
| AV-13 | SCHEDULED — TC-P-04 | DASH probe http_4xx behavior requires the prober to run against real AMS; covered by TC-P-04. |
| AV-14 | SCHEDULED — TC-P-01 | D-074 WebRTC probe fix requires a live AMS stream with `subtrackAdded` notification; covered by TC-P-01. |
| AV-15 | BLOCKED — operator decision pending | Kafka consumer path (`server/internal/collector/kafka/`) requires `PULSE_KAFKA_BROKERS` env var and a running Kafka broker. No broker deployed; not scheduled for S17. Reference `docs/operator-expected.md` Kafka question for operator decision. |
| AV-16 | CONFIRMED SAFE | `GET /LiveApp/rest/v2/broadcasts/teststream` sampled ×10 over 50 s (5 s interval, 10:55:53–10:56:38 UTC): `rtmpViewerCount=0` every sample. `publishType=RTMP`, `bitrate=2067136 bits/s` (~2 Mbps), all viewer fields 0. No negative values observed. normalize.go sum-without-clamp is safe for this deployment. Risk note: if future AMS firmware or cluster mode emits negative inline counts, clamping in normalize.go:83 would be needed. |

### Additional live findings (server-scope captures, S17)

- **App inventory change**: S16 documented 16 apps; S17 `GET /rest/v2/applications` (cookie
  auth) shows only 4: `LiveApp`, `WebRTCAppEE`, `live`, `pulse-test`. All 16 S16 apps not in
  this list now return HTTP 404 on app-scope broadcasts poll. `agents/handoffs/real-ams-captures/S17-applications.json`.
- **All remaining apps open**: All 4 apps return HTTP 200 on
  `/{app}/rest/v2/broadcasts/list/0/1` (no auth). No 403 apps on this deployment as of S17.
- **`/rest/v2/applications/info` returns HTTP 405**: Endpoint exists but `GET` is not
  allowed in build 20260504\_1443 (response: "Method Not Allowed"). S16 behavior (returned
  per-app liveStreamCount/vodCount/storage) no longer reproducible. Recorded in
  `agents/handoffs/real-ams-captures/S17-applications-info.json`.
- **AMS version**: `{"versionName":"3.0.3","versionType":"Enterprise Edition","buildNumber":"20260504_1443"}`.
  `agents/handoffs/real-ams-captures/S17-version.json`.
