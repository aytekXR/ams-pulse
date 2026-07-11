# Scenario Matrix — E2E Validation Scenarios

**Phases:** 3 (User Scenarios) + 4 (Automated Validation Scripts)
**Produced:** S16 close (2026-07-11)
**Corrected:** S17 (2026-07-11) — live-verified deltas below

## ⚠ S17 Corrections (live-verified against AMS 3.0.3 build 20260504_1443)

The S16 matrix embedded several unverified assumptions that the first live run
refuted. The scenario scripts under `qa/realams/scenarios/` implement the
CORRECTED behavior; rows below are kept as originally written for provenance.

1. **HLS URL form:** this build serves HLS at flat `/{app}/streams/{id}.m3u8`.
   The `/{app}/streams/{id}/playlist.m3u8` form used in TC-P-04/TC-P-05/TC-V
   rows always 404s (it was never valid here).
2. **Stop semantics:** implicitly-created RTMP broadcasts (no REST pre-create)
   are **deleted** on stop — `GET /broadcasts/{id}` → 404 — they do NOT
   transition to `finished` (nor to `terminated_unexpectedly` on SIGKILL).
   Terminal ground truth = `finished` **or** object-removed-404; scripts record
   which form was observed. `finished`/`terminated_unexpectedly` presumably
   apply to REST-pre-created broadcasts (to verify in S18).
3. **App inventory drift:** 16 apps (8 IP-blocked) → **4 apps, all open**
   (`LiveApp`, `WebRTCAppEE`, `live`, `pulse-test`). TC-APP-02 (403 handling)
   has no live trigger and SKIPs until a blocked test app is created.
   `GET /rest/v2/applications` returns an array of plain STRINGS.
4. **`GET /rest/v2/applications/info` → HTTP 405** (S16 captured it working).
   VoD ground truth now via per-app `GET /{app}/rest/v2/vods/count`.
5. **`versionType` = `"Enterprise Edition"`** (S16 capture said `"Enterprise"`).
6. **VoDs wiped** with the app reset; S17 created one small test VoD on
   `pulse-test` (mp4 muxing temporarily enabled, then restored) so TC-WH-03 /
   TC-A-09 keep a live ground truth.

All scenarios run against real AMS 3.0.3 at `http://161.97.172.146:5080`
and Pulse v0.3.0 at `https://beyondkaira.com` (prod) or
`http://127.0.0.1:18090` (pulse-realams isolated stack).

---

## Parity-Check Philosophy

### Ground Truth Rule

AMS REST API output is ground truth. Pulse output must match within the
tolerance windows defined per scenario. **Never accept the Pulse UI alone
as evidence.** Always curl both endpoints.

### Tolerance Windows

| Source of lag | Window |
|--------------|--------|
| AMS REST poll cycle | 5 s |
| Pulse in-memory update | ≤1 s |
| ClickHouse insert + MV propagation | 2–5 s |
| ClickHouse rollup (1h bucket) | up to 2 s after batch flush |
| Anomaly detector tick | 60 s (prod) / 5 s (CI) |
| Total count convergence after state change | **15 s** (assert after this window) |
| Total QoE convergence after beacon events | **120 s** (rollup_qoe_1h) |

### Viewer Count Tolerance

- `viewer_count` may differ by ±1 during transitions (race between AMS
  update and Pulse poll). Assert within ±2 after 15 s.
- HLS viewer counts are approximate (segment-request based; CDN caching).
  Accept ±5% for HLS counts.
- RTMP pull viewers: `totalRTMPWatchersCount = -1` from AMS
  (`broadcast-statistics_test123.json`). Pulse must clamp to 0. Assert
  Pulse shows 0, not -1.
- WebRTC counts are most accurate; assert exact match after 15 s.

### Numeric Precision

- `bitrate_kbps`: AMS reports in bits/sec; Pulse divides by 1000
  (`normalize.go:79`). Assert Pulse `bitrate_kbps * 1000 == AMS bitrate`
  within ±5% (ffmpeg output is approximate).
- `packet_loss_pct`: AMS reports 0..1 fraction; Pulse multiplies by 100
  (`normalize.go:117`). Assert exact transformation.
- `rtt_ms` / `jitter_ms` (WebRTC client): AMS reports in seconds; Pulse
  multiplies by 1000 (`normalize.go:171–173`). Assert exact conversion.

---

## Scenario Table

### L — Lifecycle Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-L-01 | Normal broadcast lifecycle | 1. Start ffmpeg RTMP publisher to LiveApp/val-stream<br>2. Wait 15 s<br>3. Stop publisher<br>4. Wait 15 s | `GET /LiveApp/rest/v2/broadcasts/val-stream` → `status: broadcasting` then `finished` | `GET /api/v1/live/streams` → stream appears with `status: broadcasting`; disappears after stop (or `status: finished`) | Yes | P0 |
| TC-L-02 | Multiple concurrent broadcasts | 1. Start 5 publishers simultaneously on LiveApp<br>2. Wait 15 s<br>3. Check counts<br>4. Stop all | `GET /LiveApp/rest/v2/broadcasts/list/0/20` → 5 streams with `status: broadcasting` | `GET /api/v1/live/overview` → `total_publishers >= 5` | Yes | P0 |
| TC-L-03 | Publisher crash (terminated_unexpectedly) | 1. Start publisher<br>2. `docker kill` the ffmpeg container (abrupt kill)<br>3. Wait 15 s | `GET /LiveApp/rest/v2/broadcasts/{id}` → `status: terminated_unexpectedly` | `GET /api/v1/live/streams` → stream gone or `status: terminated`; server_events has `event_type: stream_publish_end` | Yes | P0 |
| TC-L-04 | Rapid start/stop cycling | 1. Start publisher, stop after 5 s<br>2. Repeat 5 times in 2 min | AMS `status` transitions correctly each cycle | Pulse live/overview `total_publishers` matches AMS after each cycle; no phantom streams persist | Yes | P1 |
| TC-L-05 | Simultaneous start and stop | 1. Start 10 publishers<br>2. After 5 s, stop 5 while starting 5 new ones | AMS list shows 10 streams at all times | Pulse total_publishers stays at 10 throughout the transition | Yes | P1 |
| TC-L-06 | Long-duration stream | 1. Start publisher<br>2. Leave for 10+ minutes<br>3. Check no drift | AMS stream status `broadcasting` throughout | Pulse stream visible continuously; no phantom finish events; bitrate_kbps stable | Manual | P2 |

---

### V — Viewer Analytics Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-V-01 | HLS viewer count single viewer | 1. Start publisher on LiveApp/val-hls<br>2. Start 1 HLS viewer (curl loop)<br>3. Wait 15 s | `GET /LiveApp/rest/v2/broadcasts/val-hls` → `hlsViewerCount >= 1` | `GET /api/v1/live/streams` → `vc_hls >= 1` for stream; `viewer_count >= 1` | Yes | P0 |
| TC-V-02 | WebRTC viewer count | 1. Start publisher<br>2. Start 1 Playwright WebRTC viewer<br>3. Wait 15 s | `GET /LiveApp/rest/v2/broadcasts/{id}` → `webRTCViewerCount == 1` | `GET /api/v1/live/streams` → `vc_webrtc == 1` | Yes | P0 |
| TC-V-03 | Viewer count cross-check (AMS inline vs. Pulse) | 1. Active stream with 2 HLS + 1 WebRTC viewer<br>2. Wait 15 s<br>3. Compare | AMS: `hlsViewerCount=2, webRTCViewerCount=1, viewer_total=3` | Pulse: `viewer_count=3` within ±2 | Yes | P0 |
| TC-V-04 | RTMP viewer count — inline poll path (AV-16) | 1. Sample the LIVE poll path AMS field Pulse actually consumes | `GET /LiveApp/rest/v2/broadcasts/{id}` → inline `rtmpViewerCount == 0` (real captures; the `-1` seen in `broadcast-statistics` belongs to a dead-code endpoint Pulse never calls) | Pulse `vc_rtmp == 0`; `viewer_count` not corrupted. If inline `rtmpViewerCount` is EVER negative → normalize.go:83 sums without clamping → file a Pulse bug (AV-16) | Yes | P0 |
| TC-V-05 | Viewer count ramp (10 → 30 HLS viewers) | 1. Start publisher<br>2. Ramp 10 viewers, wait 15 s, ramp to 30, wait 15 s | AMS hlsViewerCount approaches 30 | Pulse viewer_count approaches AMS count within ±5% (HLS tolerance) | Manual | P1 |
| TC-V-06 | Viewer join then leave | 1. Active stream, 5 viewers<br>2. Stop 3 viewers<br>3. Wait 15 s | AMS `hlsViewerCount` drops from 5 to 2 | Pulse `viewer_count` drops accordingly within ±2 | Yes | P1 |
| TC-V-07 | WebRTC per-peer stats (live) | 1. Active stream with 1 WebRTC viewer<br>2. Wait 15 s | `GET /LiveApp/rest/v2/broadcasts/{id}/webrtc-client-stats/0/100` → non-empty array with `videoRoundTripTime`, `videoJitter` | `GET /api/v1/qoe/ingest` → `rtt_ms > 0`, `jitter_ms > 0`, `packet_loss_pct >= 0` | Manual | P1 |
| TC-V-08 | Unit conversion check (RTT seconds → ms) | During TC-V-07, compare raw values | AMS `videoRoundTripTime = 0.025` (25 ms in seconds) | Pulse `rtt_ms == 25` (× 1000 conversion, `normalize.go:171`) | Manual | P0 |
| TC-V-09 | BroadcastStatistics dead-code confirmation | 1. Check code: does any path call `BroadcastStatistics()`? | N/A (code inspection) | `grep -r "BroadcastStatistics" server/` returns only the definition at `client.go:483`, no callers | Yes (grep) | P1 |
| TC-V-10 | Beacon QoE with real viewer | 1. Embed beacon SDK in AMS player page<br>2. Play stream in headless browser<br>3. Wait 120 s for rollup | N/A (beacon is client-side) | `GET /api/v1/qoe/summary` → `startup_p50_ms > 0`, `session_count > 0` | Manual | P1 |

---

### I — Ingest Health Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-I-01 | Normal ingest metrics | 1. Start 2 Mbps publisher on LiveApp/val-ingest<br>2. Wait 15 s | `GET /LiveApp/rest/v2/broadcasts/val-ingest` → `bitrate ≈ 2000000` (bits/sec) | `GET /api/v1/qoe/ingest` → `bitrate_kbps ≈ 2000` (within ±10%); `health_score > 80` | Yes | P0 |
| TC-I-02 | Bitrate conversion check | During TC-I-01 | AMS `bitrate = 2048000` (bits/sec) | Pulse `bitrate_kbps = 2048` (divide by 1000, `normalize.go:79`) | Yes | P0 |
| TC-I-03 | Speed field is ratio, not kbps | During TC-I-01 | AMS `speed ≈ 1.0` (ratio, ~1.0 for real-time) | Pulse `speed_read_kbps ≈ 1.0` stored, NOT confused with kbps; docs must clarify | Yes | P1 |
| TC-I-04 | Bitrate drop — health score degradation | 1. Start publisher at 2000 kbps<br>2. Re-publish at 200 kbps (simulate degraded encoder)<br>3. Wait 15 s | AMS `bitrate drops from ~2000000 to ~200000` | Pulse `health_score` drops (from ~100 to ~50); alert fires if rule set | Yes | P0 |
| TC-I-05 | Packet loss (simulated) | Start publisher; use `tc netem loss 10%` on ffmpeg container NIC if available | AMS `packetLostRatio > 0` | Pulse `packet_loss_pct ≈ 10` (× 100); `health_score` decreases | Manual | P1 |
| TC-I-06 | FPS always 0 (AMS 3.0.3) | Inspect any active ingest | AMS `currentFPS` absent from BroadcastDTO (`client.go:97` comment) | Pulse `fps = 0` in ingest events; `health_score` not falsely penalized (FPS weight redistributed) | Yes (inspect) | P0 |
| TC-I-07 | Drop counts in BroadcastDTO | Inspect during TC-I-04 | AMS `dropFrameCountInEncoding`, `dropPacketCountInIngestion` present in BroadcastDTO | Confirm whether Pulse stores these or silently drops them (check server_events schema vs. ingest fields) | Yes (inspect) | P1 |

---

### F — Failure Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-F-01 | Publisher disconnect (graceful stop) | 1. Start publisher<br>2. Stop ffmpeg cleanly<br>3. Wait 15 s | AMS `status: finished` | Pulse stream removed from `/live/streams`; `stream_publish_end` event in server_events | Yes | P0 |
| TC-F-02 | Publisher crash (terminated_unexpectedly) | 1. Start publisher<br>2. `docker kill -s KILL` ffmpeg container<br>3. Wait 20 s | AMS `status: terminated_unexpectedly` | Pulse emits `stream_publish_end` with `reason: terminated_unexpectedly`; stream removed from live | Yes | P0 |
| TC-F-03 | AMS unavailable | 1. `docker stop antmedia`<br>2. Wait 60 s<br>3. `docker start antmedia`, wait 30 s | N/A (AMS down) | During downtime: `GET /api/v1/healthz` returns 503 or degraded; streams marked stale. After recovery: streams reappear within 15 s of AMS poll resuming | Manual | P0 |
| TC-F-04 | Pulse restart | 1. Record current live stream state<br>2. Restart pulse container<br>3. Wait until healthz=ok<br>4. Check live state | AMS stream state unchanged during Pulse restart | After restart: Pulse `live/overview` reflects current AMS state (re-polled on startup); no duplicate events | Manual | P0 |
| TC-F-05 | AMS re-login after session expiry | 1. Start pub, wait<br>2. Force AMS session expiry (restart AMS)<br>3. Check Pulse auto-re-authenticates | AMS returns 401 on stale JSESSIONID | Pulse logs re-auth warning; poll resumes within ≤10 s (re-login throttle ≥3 s per `client.go:250`) | Manual | P1 |
| TC-F-06 | Invalid stream key publish | 1. Publish RTMP with a stream key that has token=invalid (if token control enabled) | AMS rejects the publish; no stream appears in broadcasts list | Pulse shows no stream for that ID; no phantom stream created | Manual | P1 |
| TC-F-07 | IP-blocked AMS app polling | 1. Check apps with `remoteAllowedCIDR=127.0.0.1` (e.g., `TEST`, `Icomms`) | AMS returns HTTP 403 for `GET /TEST/rest/v2/broadcasts/list/0/10` from VPS IP | Pulse logs a per-app warning; does NOT crash; continues polling open apps; `GET /api/v1/live/overview` still works for open apps | Yes | P0 |
| TC-F-08 | Network reconnection after disconnect | 1. Publisher active<br>2. Disconnect publisher container from Docker bridge<br>3. Wait 10 s<br>4. Reconnect | AMS: stream may go to `finished` or `terminated_unexpectedly` during gap | Pulse detects the end event; after reconnect and re-publish, a new stream_publish_start is emitted | Manual | P1 |
| TC-F-09 | Expired publisher token attempt | 1. Enable `publishTokenControl` on a test-only AMS app (NOT LiveApp)<br>2. Call `inject_expired_token_publish()` (validation-environment.md §6.6)<br>3. Wait 15 s | AMS rejects the publish; no stream appears in the broadcasts list | Pulse shows no phantom stream for that ID; overview counts unchanged | Manual | P1 |

---

### H — Health Monitoring Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-H-01 | Fleet view — standalone node | 1. Confirm `GET /rest/v2/cluster/nodes` returns 404<br>2. Check Pulse fleet | AMS `/rest/v2/system-status` → `{osName:Linux, osArch:amd64, javaVersion:17, processorCount:8}` | `GET /api/v1/fleet/nodes` → node card shows OS/JVM version; cpu_pct/mem_pct NOT shown (null/absent, not false-zero) | Yes | P0 |
| TC-H-02 | Healthz endpoint | `curl /api/v1/healthz` | N/A | HTTP 200 with `{status: ok}`; 503 if ClickHouse or AMS poll is failing | Yes | P0 |
| TC-H-03 | Prometheus metrics endpoint | `curl /metrics` | N/A | HTTP 200 with Prometheus text exposition; counter for streams, viewers, probe results | Yes | P1 |
| TC-H-04 | Alert: ingest_bitrate_floor | 1. Create alert rule `metric=ingest_bitrate_floor op=lt threshold=99999`<br>2. Start publisher at 2000 kbps | AMS `bitrate ≈ 2000000 bits/sec` (2000 kbps < 99999) | Alert fires within 15 s; `GET /api/v1/alerts/history?state=firing` shows the rule; alert resolves when threshold updated | Yes | P0 |
| TC-H-05 | Alert: viewer_count threshold | 1. Create rule `metric=viewer_count op=gt threshold=0` for stream val-alert<br>2. Add 1 viewer | AMS `webRTCViewerCount=1` | Alert fires within 15 s | Yes | P1 |
| TC-H-06 | Alert: cpu_pct — standalone empty baseline | Standalone AMS; no cluster nodes | AMS does NOT return cpu_pct via REST | Alert rule for cpu_pct should not fire with false data; `GET /api/v1/anomalies` returns empty for node cpu | Yes | P0 |
| TC-H-07 | Alert delivery — Slack channel | 1. Create Slack webhook channel<br>2. Create alert rule that fires<br>3. Wait for delivery | N/A | Slack message received within 30 s of alert firing; `GET /api/v1/alerts/history` shows `delivery_failure=0` | Manual | P1 |

---

### S — Stress Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-S-01 | Many concurrent publishers (20 streams) | 1. Start 20 ffmpeg publishers on LiveApp<br>2. Wait 15 s | AMS `GET /LiveApp/rest/v2/broadcasts/list/0/20` → 20 streams `broadcasting` | Pulse `total_publishers >= 20`; `GET /api/v1/live/streams` returns all 20; no poll errors in Pulse logs | Yes | P1 |
| TC-S-02 | Many concurrent publishers (100 streams) | 1. Use `start_bulk_publishers 100 LiveApp`<br>2. Supplement with AMS REST-created streams for count only | AMS `count` endpoint returns >= 100 | Pulse total_publishers >= 100; pagination works (>200 page breaks if mock, but real AMS may impose limits) | Manual | P2 |
| TC-S-03 | Rapid simultaneous start/stop | 1. Start 10 publishers<br>2. Immediately stop all<br>3. Repeat 3 times in 60 s | AMS broadcast count goes 0→10→0 three times | Pulse live/overview transitions correctly; no phantom streams after final stop; no panic in logs | Yes | P1 |
| TC-S-04 | AMS+Pulse simultaneous restart | 1. Active streams present<br>2. Restart both AMS and Pulse at the same time | N/A | After both recover: Pulse re-discovers all AMS streams within 30 s; no duplicate publish_start events | Manual | P2 |

---

### P — Probe Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-P-01 | WebRTC probe — live stream | 1. Start publisher on LiveApp/val-probe<br>2. Create WebRTC probe targeting `ws://161.97.172.146:5080/LiveApp/websocket` with streamId=val-probe<br>3. Wait 120 s | AMS sends `notification(subtrackAdded)` then `takeConfiguration(offer)` (confirmed from real capture D-074) | `GET /api/v1/probes/{id}/results` → `success=true`, `signaling_state=offer_received`, `ice_state=connected`, `connect_time_ms > 0`, `rtt_ms` non-null | Yes | P0 |
| TC-P-02 | WebRTC probe — no active stream | 1. Create WebRTC probe with a non-existent streamId | AMS sends `play_finished` notification (stream not found) | Probe result: `success=false`, `signaling_state=ws_error` or appropriate error code | Yes | P0 |
| TC-P-03 | RTMP probe | 1. Create RTMP probe targeting `rtmp://161.97.172.146:1935/LiveApp`<br>2. Wait 90 s | AMS completes C0/C1/S0/S1/S2/C2 handshake | `success=true`, `signaling_state=handshake_complete`, `connect_time_ms > 0` | Yes | P0 |
| TC-P-04 | HLS probe — live stream | 1. Active publisher on LiveApp/val-hls<br>2. Create HLS probe: `http://161.97.172.146:5080/LiveApp/streams/val-hls/playlist.m3u8`<br>3. Wait 60 s | AMS serves valid M3U8 playlist | `success=true`, `ttfb_ms > 0`, `bitrate_kbps > 0`, `segment_ttfb_ms > 0` | Yes | P0 |
| TC-P-05 | HLS probe — no stream (404) | Probe non-existent stream playlist | AMS returns HTTP 404 | `success=false`, `error_code=http_4xx` | Yes | P0 |
| TC-P-06 | DASH probe — AMS without DASH | Create DASH probe on test AMS (DASH muxing disabled) | AMS returns 404 for `.mpd` URL | `success=false`, `error_code=http_4xx`; no crash; documented as expected | Yes | P0 |
| TC-P-07 | Probe interval and scheduling | 1. Create probe with 60 s interval<br>2. Wait 5 min | N/A | `GET /api/v1/probes/{id}/results` shows 4–5 result rows with consistent timing; no missing intervals | Manual | P1 |
| TC-P-08 | Multiple simultaneous probes | Create 3 probes (WebRTC + RTMP + HLS) for same stream | All AMS protocol endpoints respond | All 3 probes show success=true concurrently in results | Yes | P1 |

---

### A — Analytics Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-A-01 | Audience analytics — viewer minutes | 1. Real WebRTC session with beacon SDK, 5 min watch<br>2. Wait 120 s for rollup | N/A (client-side) | `GET /api/v1/analytics/audience` → `watch_time_s >= 300`; `viewer_minutes >= 5` | Manual | P1 |
| TC-A-02 | Audience analytics — peak concurrency | 1. Active stream, 3 concurrent viewers<br>2. Wait 5 min<br>3. Check rollup | N/A | `GET /api/v1/analytics/audience` → `peak_concurrency >= 3` in rollup | Manual | P1 |
| TC-A-03 | Geo analytics — real beacon | 1. Beacon session from a real browser with known country IP | N/A | `GET /api/v1/analytics/geo` → `{country: <expected>}` appears | Manual | P2 |
| TC-A-04 | Device analytics — beacon player kind | 1. Beacon session with `player_kind=hls.js` | N/A | `GET /api/v1/analytics/devices` → `client_browser` or `client_device` populated | Manual | P2 |
| TC-A-05 | QoE summary — startup time | 1. Beacon SDK in player, play stream, fire `startup_complete` with `startup_ms=450`<br>2. Wait 120 s | N/A | `GET /api/v1/qoe/summary` → `startup_p50_ms ≈ 450` | Yes | P1 |
| TC-A-06 | QoE summary — rebuffer ratio | 1. Beacon fires `rebuffer_start` + `rebuffer_end` with `rebuffer_ms=2000`; `heartbeat` with `watch_ms=10000` | N/A | `GET /api/v1/qoe/summary` → `rebuffer_ratio ≈ 0.2` (2000/10000) | Yes | P1 |
| TC-A-07 | Usage report — viewer minutes accumulation | 1. 10-viewer HLS session, 5 min each<br>2. Wait for rollup | N/A | `GET /api/v1/reports/usage` → `viewer_minutes ≈ 50` | Manual | P1 |
| TC-A-08 | Usage report — egress bytes always 0 | Any active session | N/A (AMS does not expose per-stream egress) | `GET /api/v1/reports/usage` → `egress_gb == 0`; documented as unimplemented | Yes | P1 |
| TC-A-09 | Recording report — 0 because webhook blocked | Active AMS VoDs (~24 GB in WebRTCAppEE per applications-info.json) | AMS `vodCount > 0`, storage bytes present | `GET /api/v1/reports/usage` → `recording_gb == 0` (no webhook delivery); document gap | Yes | P0 |

---

### AN — Anomaly Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-AN-01 | Viewer count anomaly baseline warmup | 1. Start stream, ~5 viewers, wait 30 min (prod tick=60 s) | AMS viewer counts stable at ~5 | `GET /api/v1/anomalies` → no anomaly for stream after 30 min stable | Manual | P1 |
| TC-AN-02 | Viewer count anomaly spike | After TC-AN-01 baseline: spike viewers to 50+ | AMS `webRTCViewerCount` jumps | `GET /api/v1/anomalies` → `metric=viewers` anomaly flag with `sigma > 4.0` and `observed` near 50 | Manual | P1 |
| TC-AN-03 | CPU/mem anomaly — standalone empty | Standalone AMS, no cluster nodes | AMS system-status has no CPU/mem | `GET /api/v1/anomalies` → no `cpu_pct` or `mem_pct` anomaly flags (empty, not error 500) | Yes | P0 |
| TC-AN-04 | Ingest bitrate anomaly | 1. Stable stream at 2000 kbps (30 min baseline)<br>2. Drop to 100 kbps<br>3. Wait for detection | AMS `bitrate` drops by 95% | `GET /api/v1/anomalies` → `metric=ingest_bitrate_kbps` anomaly with high sigma | Manual | P2 |
| TC-AN-05 | error_rate NOT tracked (known gap) | 1. Beacon fires many `error` events<br>2. Wait 60 s | N/A | `GET /api/v1/anomalies` → no `error_rate` anomaly (not tracked by detector per F9 finding) | Yes | P1 |

---

### WH — Webhook Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-WH-01 | Webhook audit — no events arriving | 1. Check Pulse logs for webhook requests<br>2. Start/stop publisher | AMS sends lifecycle hooks to `listenerHookURL` (if configured) but cannot HMAC-sign | Pulse webhook listener (`POST /webhook/ams`) logs zero successful deliveries; all rejected with HMAC validation error or not configured | Manual | P0 |
| TC-WH-02 | Poll path covers webhook gap | During TC-WH-01: confirm REST poll detects start/end | AMS stream `status` transitions in REST poll | Pulse emits `stream_publish_start` and `stream_publish_end` events via REST poll (no webhook needed) | Yes | P0 |
| TC-WH-03 | VoD recording gap | AMS has VoD recordings (WebRTCAppEE, ~24 GB) | AMS `vodCount > 0` in applications-info | Pulse `recording_bytes = 0` in reports — webhook `vodReady` never delivered; document as known gap | Yes | P0 |

---

### FL — Fleet Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-FL-01 | Standalone node discovery | `GET /rest/v2/cluster/nodes` returns 404; `GET /rest/v2/system-status` returns OS info | AMS: `{osName:Linux, osArch:amd64, javaVersion:17, processorCount:8}` | `GET /api/v1/fleet/nodes` → 0 nodes (no cluster) OR 1 synthetic node from system-status; OS/JVM fields populated; CPU/mem/disk fields absent (null), not false-zero | Yes | P0 |
| TC-FL-02 | AMS version display | `GET /rest/v2/version` | AMS: `{versionName:"3.0.3", versionType:"Enterprise"}` | Fleet node card or system info shows AMS version `3.0.3 Enterprise` | Yes | P0 |

---

### APP — Application Management Scenarios

| ID | Scenario | Steps | AMS Ground Truth | Pulse Assertion | Auto | Priority |
|----|----------|-------|-----------------|-----------------|------|---------|
| TC-APP-01 | Multi-app polling — open apps | Pulse polls all apps | AMS: 8 apps accessible from VPS IP (`24x7test`, `Conference`, `LiveApp`, `LiveShopping`, `PetarTest2`, `clipcreator`, `demo`, `meet`) | Pulse `live/streams` shows streams from all 8 open apps; no streams from IP-blocked apps (`Icomms`, `TEST`, etc.) | Yes | P0 |
| TC-APP-02 | IP-blocked app 403 handling | Check logs for blocked apps | AMS returns HTTP 403 for blocked apps from VPS IP | Pulse logs warning per app, does NOT crash; continues polling open apps | Yes | P0 |
| TC-APP-03 | App auto-discovery | `PULSE_AMS_APPLICATIONS` env not set | AMS `/rest/v2/applications` returns 16 apps | Pulse discovers and attempts polling for all 16 apps; 8 succeed, 8 return 403 (logged) | Yes | P1 |

---

### DOC — Documentation Gap Scenarios

These are not automated but drive Phase 6 documentation deliverables.

| ID | Scenario | Finding | Documentation Action |
|----|----------|---------|---------------------|
| TC-DOC-01 | HLS viewer count CDN limitation | AMS HLS counts degrade behind CDN | Add to Troubleshooting: "HLS viewer counts behind a CDN" |
| TC-DOC-02 | RTMP pull count = -1 | `totalRTMPWatchersCount=-1` is untracked, not a bug | Add to FAQ: "Why does RTMP viewer count show 0?" |
| TC-DOC-03 | FPS always 0 (AMS 3.x REST) | `currentFPS` not in BroadcastDTO in AMS 3.x | Add to Known Limitations: "FPS metric availability" |
| TC-DOC-04 | Webhook requires HMAC / AMS 3.x can't sign | Webhook events don't reach Pulse in default AMS 3.x config | Add to Installation Guide: "Webhook configuration and AMS 3.x limitation" |
| TC-DOC-05 | CPU/mem unavailable standalone AMS | REST system-status omits resource metrics | Add to Fleet Monitoring Guide: "Resource metrics require cluster mode or Kafka" |
| TC-DOC-06 | Egress bytes always 0 | No AMS egress data source; CDN logs needed | Add to Reports Guide: "Egress measurement limitation" |

---

## Summary by Phase and Priority

| Priority | Count | Notes |
|----------|-------|-------|
| P0 | 24 | Must pass before any marketplace claim |
| P1 | 20 | Required for full feature coverage claim |
| P2 | 6 | Optional / stretch scenarios |

P0 scenarios cover the critical user story arcs: broadcast lifecycle,
viewer count accuracy, ingest health, failure recovery, prober
correctness, webhook gap, fleet discovery, and recording gap disclosure.
All P0 auto scenarios can run unattended via `make validate-realams-p0`.
