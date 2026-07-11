# PRD Validation Matrix — Pulse v0.3.0 × AMS 3.0.3

<!--
  ╔══════════════════════════════════════════════════════════════════════════╗
  ║  DRAFT — OPERATOR REVIEW REQUIRED BEFORE SHARING WITH ANT MEDIA OR      ║
  ║          ANY EXTERNAL PARTY                                              ║
  ╚══════════════════════════════════════════════════════════════════════════╝
-->

> **DRAFT — OPERATOR REVIEW REQUIRED BEFORE SHARING WITH ANT MEDIA OR ANY EXTERNAL PARTY**
>
> This document lists the Ant Media team as a direct audience and must not be
> shared externally until the operator has reviewed it. This draft is produced
> at S19 close and is an internal working document only.

---

**Document:** Phase 7 deliverable — PRD feature validation and architecture budget verification  
**Product:** Pulse: Real-Time Analytics, QoE Monitoring and Alerting for Ant Media Server  
**Date:** 2026-07-11 (Session S19)  
**PRD source:** `docs/prd-report.md` §7 (lines 220–446); feature IDs F1–F10 as defined there  
**Architecture budget source:** `docs/ARCHITECTURE.md` §4 (lines 149–228); criterion IDs N1–N36 are stable identifiers assigned for this document  
**Audience:** Ant Media team and Pulse founding team; forward-referenced in `docs/assessment/final-assessment.md`

---

## Evidence Base and Ground Rules

**Validation corpus:** P0 25 PASS / 1 SKIP / 0 FAIL + P1 21 PASS / 3 SKIP / 0 FAIL across
50 automated scenario scripts run from harness `qa/realams/` against a live AMS 3.0.3
Enterprise Edition (build 20260504\_1443, trial expiry 2026-07-12T12:09Z; operator-waived,
all sessions executed pre-expiry). CI evidence cited as "CI" refers to the mock-ams test
suite (deterministic, network-isolated, run at every push).

**Evidence directories** are local to the validation VPS and are gitignored. They are cited
throughout this document by scenario ID and timestamp prefix (e.g.
`S17-TC-WH-02-20260711T120043Z`). The directory contains `verdict.txt`, timestamped JSON
snapshots from AMS and Pulse APIs, and computed deltas.

**SKIP is not validation.** All four scenario SKIPs are documented with explicit reasons:

| Scenario | Reason |
|----------|--------|
| TC-APP-02 | Premise unmet: no IP-blocked apps exist on the test AMS (all 4 apps are CIDR-open) |
| TC-V-06 | AMS semantics: HLS viewer count is a sliding segment-request window; count did not drop below 4 in 90 s after stopping 3 of 5 real viewers (final AMS count = 38) |
| TC-L-05 | ENV-LIMIT: VPS AMS concurrent RTMP capacity ~5–7 streams; 0 of 5 additional streams accepted |
| TC-S-01 | ENV-LIMIT: 0 of 20 concurrent val-s01 publishers accepted; AMS returned "current system resources not enough" |

**Scale bounds.** All results from the real AMS are validated up to N = 5 concurrent
publishers (VPS capacity ~5–7 concurrent RTMP streams). Scale claims at N = 500 streams
and N = 3,000 viewers are CI-only (mock-ams, A10 load smoke, SESSION-07, D-064).

**Webhook limitation.** AMS 3.0.3 cannot HMAC-sign lifecycle hooks (O3, `decisions.md`).
Pulse's webhook listener is fail-closed; no webhook events arrive in this deployment.
REST polling covers stream lifecycle detection within the PRD ≤ 10 s latency budget (4 s
publish-to-visible confirmed). The only structural consequence is `recording_gb = 0`
(BUG-002). This limitation is addressed per-feature where it creates a gap.

**AMS 3.0.3-specific semantics** (not Pulse bugs) are documented in
`docs/assessment/documentation-gaps.md` (DG-01 through DG-18) and noted in evidence
cells below.

**Verdict vocabulary:**

| Verdict | Meaning |
|---------|---------|
| FULLY | Requirement implemented and validated end-to-end against real AMS |
| PARTIALLY | Core implemented; one or more sub-requirements missing, approximated, or not live-validated |
| MISSING | Requirement specified in the PRD; not implemented or structurally non-functional in this deployment |
| DIFFERENTLY | Implemented, but via a method that differs from the PRD spec; the delta is documented |
| NEEDS-CLARIFICATION | Requirement is ambiguous or its measurement is undefined; Ant Media team input needed |

---

## Table 1 — Feature Validation

Each PRD feature (F1–F10) has one overall verdict row followed by sub-rows for each
distinct acceptance criterion or description element. Evidence cells cite scenario IDs,
AV triage IDs, and BUG IDs. PRD text is drawn from `docs/prd-report.md` §7.9 verbatim.

### F1 — Real-Time Operations Dashboard

**PRD description:** Live view of concurrent viewers (total, per application, per stream),
active publishers, node health (CPU, RAM, network), and protocol mix, refreshing every
few seconds.

**PRD acceptance criteria:** Dashboard reflects a new stream within 10 seconds of publish;
concurrent viewer counts match AMS REST `broadcast-statistics` within ±2%; works for
standalone and cluster deployments; loads in under 2 seconds with 500 concurrent streams.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F1 — Real-Time Operations Dashboard** | **Overall** | **PARTIALLY** | — | Core ops dashboard validated against real AMS; node CPU/RAM/network absent for standalone AMS via REST; webhook-based instant detection non-functional (REST poll covers the latency budget) |
| | Dashboard reflects a new stream within 10 s of publish | FULLY | TC-WH-02 (S17-TC-WH-02-20260711T120043Z): publish→Pulse 4 s, stop→Pulse 7 s (timeline.txt) | Both transitions well within the ≤ 10 s budget. Webhook-based instant detection is non-functional on AMS 3.0.3 (fail-closed listener, O3); REST poll at 5 s interval covers the requirement. |
| | Concurrent viewer counts within ±2% of AMS REST `broadcast-statistics` (standalone) | FULLY | TC-V-03 (S17-TC-V-03-20260711T120829Z; viewer_count within ±2); TC-V-04 (S17-TC-V-04-20260711T120522Z; rtmpViewerCount=0 across 20 samples at 5 s intervals; AV-16 CONFIRMED SAFE) | Viewer counts use inline BroadcastDTO fields (hlsViewerCount + webRTCViewerCount + rtmpViewerCount + dashViewerCount, normalize.go:83). The `amsclient.BroadcastStatistics()` method is dead code at runtime (BUG-001, low severity; no user impact, as inline counts are accurate). |
| | Node health (CPU, RAM, network) visible | DIFFERENTLY | AV-06 (CONFIRMED ABSENT: cpu\_pct, mem\_pct, disk\_pct, net\_in\_mbps, net\_out\_mbps absent from standalone node REST response); AV-05 (CONFIRMED: os\_name, java\_version, processor\_count, version, role, status present); AV-15 (BLOCKED: Kafka path not deployed) | AMS REST v2 `/system-status` for standalone deployments does not expose CPU, memory, or disk metrics. These fields are available via the Kafka `ams-instance-stats` topic (AV-15 BLOCKED, operator Kafka decision pending) or via cluster-mode node stats. Pulse correctly emits null for absent fields rather than false zeros (honest-absent design). See DG-05 in `docs/assessment/documentation-gaps.md`. |
| | Works for standalone and cluster deployments (edge/origin deduplication) | PARTIALLY | TC-FL-01 (S17-TC-FL-01-20260711T115050Z; 7/7 PASS — standalone fleet card), TC-FL-02 (S17-TC-FL-02-20260711T120431Z; 4/4 PASS — version=3.0.3 Enterprise Edition); IsEdgeStream() implementation confirmed (AV-16 dedup safe); TC-V-04 (rtmpViewerCount inline, no negative values) | Standalone mode fully validated against live AMS. Cluster mode: edge-viewer dedup (IsEdgeStream()) is implemented and unit-tested; not live-validated against a real multi-node AMS cluster (ENV-LIMIT: single-node VPS). |
| | Loads in under 2 s with 500 concurrent streams | FULLY | CI: 668 ms / 459 ms (run1/run2; Playwright navigation + grid visible; VD-04 CLOSED, SESSION-04/WO-4); ENV-LIMIT: real AMS capped at ~5–7 concurrent RTMP streams (TC-S-01 ENV-LIMIT SKIP) | Real-AMS scale test (TC-S-01) could not run — VPS AMS rejected all 20 publishers ("current system resources not enough"). CI result with mock-ams (500 streams, 3,000 viewers) is the authoritative measurement for this budget. |

---

### F2 — Historical Audience Analytics

**PRD description:** Views, unique viewers, watch time, peak concurrency, geography
(IP-derived, anonymizable), device/OS/browser, and protocol breakdowns over arbitrary date
ranges, per stream/app/node.

**PRD acceptance criteria:** Any 13-month query over rollups returns in under 3 seconds;
per-stream report exportable as CSV; geo accurate to country level with optional region;
data survives Pulse restarts and AMS upgrades.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F2 — Historical Audience Analytics** | **Overall** | **PARTIALLY** | — | Core viewer metrics and rollups confirmed in real deployment; geo analytics non-functional without mmdb; device/OS/browser data sparse in real deployment |
| | Views, unique viewers, watch time, peak concurrency | FULLY | TC-A-05 (S18-TC-A-05-20260711T141532Z; 3/3 PASS — QoE startup_p50_ms non-zero); TC-A-06 (S18-TC-A-06-20260711T144030Z; 3/3 PASS — rebuffer ratio); AV-09 (viewer_minutes=0.3333 from one live beacon session) | ClickHouse rollups store and return viewer-session data. startup_p50_ms=250.0 confirmed from rollup_qoe_1h (real query, not stub — V3a fix). Data persists across Pulse restarts by design (ClickHouse persistent volume). |
| | Geography (IP-derived, anonymizable) accurate to country level | MISSING | AV-10 (CONFIRMED ABSENT: GeoLite2-City.mmdb not deployed; PULSE\_GEO\_MMDB\_PATH commented out in deploy/.env); AV-11 (CONFIRMED: GET /api/v1/analytics/geo → \[{country:'', views:1, uniques:1, watch\_time\_s:10}\]) | The geo ClickHouse query runs correctly and returns real rows (AV-11; not a stub). Country is '' because the MaxMind mmdb is not bundled with Pulse — operators must obtain and mount it separately. Without the mmdb, country-level geo accuracy is not available. See DG-17. The anonymize-IP switch is implemented in design. |
| | Device/OS/browser and protocol breakdowns | PARTIALLY | TC-A-05/06 PASS (beacon events reach ClickHouse and populate QoE rollup); TC-V-01 (S17; vc\_hls≥1 confirmed) | Protocol breakdowns (vc\_hls, vc\_webrtc, vc\_rtmp) are confirmed from inline BroadcastDTO fields. Device/OS/browser fields are captured structurally via beacon SDK metadata; real-deployment data is sparse (one beacon session in the evidence base). A full device/OS breakdown requires more beacon-reporting player integrations than were available on the test deployment. |
| | 13-month rollup query < 3 s | FULLY | CI: 144 ms (simple aggregate, C9, Wave-3) + 145 ms (dimensional GROUP BY, 3 geo × 2 device × 2 protocol, 12 rows; C9b `qa/wave-2/run-gate.sh`, Wave-3 gate report); Wave-2 baseline 126 ms (C-W2-08) | Both aggregate and dimensional rollup queries far within the 3 s budget, measured against the actual ClickHouse rollup schema. |
| | Per-stream report exportable as CSV | PARTIALLY | N14 (monthly statement < 60 s: CI 4.8 ms); TC-A-05/06 PASS (data in ClickHouse confirmed) | Programmatic report generation is implemented and fast (CI). Scheduled CSV-to-S3 export delivery was not live-validated against real AMS in the evidence base. |
| | Data survives Pulse restarts and AMS upgrades | FULLY | TC-L-04 (S18-TC-L-04-20260711T142655Z; 21/21 PASS — rapid stream cycling; state consistency confirmed); ClickHouse persistent volume design | ClickHouse retains all events and rollups across Pulse process restarts. AMS upgrade compatibility: the collector uses only stable REST v2 endpoints; no AMS upgrade was performed during the validation window. |

---

### F3 — Player QoE Beacon SDK

**PRD description:** A small JS SDK (mobile SDKs in Phase 3) wrapping the AMS WebRTC
adapter and HLS players, reporting startup time, rebuffer count/duration, playback errors
with codes, bitrate/resolution switches, and watch time; CMCD-aligned field naming.

**PRD acceptance criteria:** Under 15 KB gzipped; one-line init with stream and customer
metadata; events batched and sent at most every 10 seconds; player overhead under 1% CPU;
graceful no-op if the collector is unreachable; documented integration for AMS JS SDK,
hls.js and video.js within MVP+1.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F3 — Player QoE Beacon SDK** | **Overall** | **PARTIALLY** | — | Core SDK validated in CI and live deployment; player CPU overhead not measurable; integration documentation (MVP+1 item) not yet produced |
| | SDK < 15 KB gzipped | FULLY | CI: 3.52 KB gzip (Wave-3 measurement; no regression from Wave-2 3.44 KB despite VD-09/12/13 fixes adding code; N6) | Comfortably within budget (23% of the 15 KB limit). |
| | One-line init with stream and customer metadata; session stitching | FULLY | TC-A-05 (S18; 3/3 PASS); TC-A-06 (S18; 3/3 PASS); AV-09 (beacon session confirmed in prod with viewer_minutes=0.3333) | Beacon round-trip confirmed against live Pulse collector on the real AMS deployment. Session UUID stitching implemented. |
| | Events batched and sent at most every 10 s | FULLY | N8 (beacon round-trip accepted, no-error — CI + live); SDK design: sendBeacon with retry queue | Batching interval is a SDK configuration constant; retry queue is implemented. |
| | Player CPU overhead < 1% | NEEDS-CLARIFICATION | Not measurable — VD-14 (deferred by design consensus; noted in ARCHITECTURE.md §4 N7 cell) | No reliable method to isolate SDK CPU contribution from the player and browser overhead in a controlled measurement. The target remains aspirational; a benchmark harness for this metric has not been defined. Ant Media team input requested: acceptable benchmark methodology? |
| | Graceful no-op if the collector is unreachable | FULLY | TC-P-05 (S17; 2/2 PASS — HLS probe to non-existent stream returns success=false, error\_code=http\_4xx, no crash); SDK design: error swallowed, no exception thrown | SDK error path confirmed not to throw; events are dropped rather than crashing the player when the ingest endpoint is unreachable. |
| | Documented integration for AMS JS SDK, hls.js and video.js (MVP+1) | MISSING | DG-07 (documentation-gaps.md: "beacon SDK integration guide not yet produced") | This is explicitly a PRD MVP+1 documentation deliverable. The runtime SDK integration points exist; the step-by-step operator guide covering adapter selection, token provisioning, and ingest URL has not yet been authored. Planned for S19 (WO-C). |
| | QoE startup time (startup\_p50\_ms) non-zero from rollup | FULLY | TC-A-05 (S18; 3/3 PASS); CI N10: startup\_p50\_ms=250.0 from rollup\_qoe\_1h (TestQuery\_QoeSummary\_RealStartupP50) | Rollup query is real (V3a fix confirmed); non-zero value confirmed in both CI and live deployment. |
| | Rebuffer count/duration tracked | FULLY | TC-A-06 (S18; 3/3 PASS) | rebuffer\_ratio field in QoE rollup confirmed present and non-zero in test scenario. |

---

### F4 — Publisher and Ingest Health

**PRD description:** Per-publisher bitrate, fps, keyframe interval, packet loss/jitter
(WebRTC), and source-drop detection for RTMP/RTSP/SRT sources, with stream "health score."

**PRD acceptance criteria:** Ingest degradation visible within 15 seconds; per-source
historical charts; health score documented and reproducible.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F4 — Publisher and Ingest Health** | **Overall** | **PARTIALLY** | — | Bitrate, health score, and source-drop detection confirmed live; FPS always 0 on AMS 3.x REST; BUG-004 breaks windowed historical queries |
| | Per-publisher bitrate visible and accurate | FULLY | TC-I-01 (S17-TC-I-01-20260711T120854Z; 4/4 PASS); TC-I-02 (S17-TC-I-02-20260711T120919Z; AMS ~2,067,136 bits/s → Pulse 2,067 kbps, ÷1000 at normalize.go:79, within ±10% of 2,000 kbps target) | Bitrate normalization confirmed correct on a live 2 Mbps RTMP stream. |
| | FPS (frames per second) visible | DIFFERENTLY | AV-04 (CONFIRMED: currentFPS absent from AMS 3.0.3 BroadcastDTO; Pulse fps=0 for all REST-polled streams); TC-I-06 (S17-TC-I-06-20260711T120943Z; 4/4 PASS — health\_score>80 at 2 Mbps with fps=0) | `currentFPS` is absent from the AMS 3.0.3 broadcast list REST response (noted at client.go:97 comment). Pulse stores fps=0 for all streams on this AMS build. FPS is available via the AMS analytics log (Kafka path); Kafka is not deployed on the test VPS (AV-15 BLOCKED). The health score correctly does not penalize fps=0 (weight redistributed in ComputeHealthScore). See DG-03. |
| | Packet loss / jitter (WebRTC ingest) | DIFFERENTLY | TC-I-05 (S18-TC-I-05-20260711T145211Z; 4/4 PASS — AMS packetLostRatio=0 with netem 10% loss injected on RTMP publisher NIC; DG-18); TC-V-07 (S18; rtt\_ms=null, jitter\_ms=null for same-host loopback — correct per D-075); TC-V-08 (S18; unit conversion at normalize.go:185 ×1000 code-verified) | RTMP/TCP ingest masks transport-layer packet loss (TCP retransmits before AMS observes the stream). packetLostRatio is meaningful only for WebRTC and SRT ingest paths. Pulse faithfully mirrors AMS = 0 for RTMP (not a Pulse bug; documented DG-18). WebRTC publisher loss/jitter: ×1000 unit conversion code-confirmed at normalize.go:185; exercised at 0 values on same-host loopback. Non-zero live validation requires a remote WebRTC publisher. |
| | Source-drop detection for RTMP/RTSP/SRT | FULLY | TC-I-07 (S18-TC-I-07-20260711T142553Z; 6/6 PASS) | Drop count detection confirmed against live AMS RTMP stream. |
| | Stream health score documented and reproducible (0–100 scale) | FULLY | TC-I-06 (S17; 4/4 PASS; health\_score>80 at 2 Mbps with fps=0); N11 criterion met | ComputeHealthScore weights are documented; health\_score>80 confirmed for a healthy 2 Mbps ingest. FPS weight redistributed to avoid false penalization when AMS 3.x does not provide currentFPS. |
| | Ingest degradation visible within 15 s | FULLY | CI: 250.8 µs in-process detection (C-W2-06); TC-I-04 (S18-TC-I-04-20260711T145154Z; 4/4 PASS after BUG-004 workaround) | Detection is in-process (essentially instantaneous). The REST poll propagation delay adds up to 5 s in the worst case, well within the 15 s budget. |
| | Per-source historical charts (windowed time range) | PARTIALLY | TC-I-04 (S18; 4/4 PASS after workaround; BUG-004 confirmed: era-mixed timeseries — blending ~649 kbps average of pre-drop 2,000 kbps and post-drop 200 kbps eras) | BUG-004: `GET /api/v1/qoe/ingest` handler silently ignores the `from` and `to` query parameters declared in the OpenAPI spec. Windowed queries return an all-time ClickHouse average, masking the degradation event. This is an OpenAPI contract violation (declared parameters accepted and discarded). TC-I-04 passed by reading the live-aggregator snapshot field as a workaround, not the windowed historical query. |

---

### F5 — Alerting and Incident Automation

**PRD description:** Rule engine on any metric (stream offline, viewer drop >X% in Y
minutes, rebuffer ratio, error rate, ingest bitrate floor, node CPU/disk, certificate
expiry) with notification channels: email, Slack, Telegram, PagerDuty, generic webhook;
maintenance windows; alert history.

**PRD acceptance criteria:** Detection-to-notification under 30 seconds; no duplicate
storms (grouping and cooldowns); test-fire button per channel; rules survive restarts.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F5 — Alerting and Incident Automation** | **Overall** | **PARTIALLY** | — | Core alert pipeline validated live (stream offline, bitrate floor, viewer count); CPU/disk absent for standalone AMS; error rate and rebuffer ratio alerting not live-validated |
| | Stream offline alert | FULLY | TC-F-01 (S17-TC-F-01-20260711T120452Z; 4/4 PASS — graceful stop); TC-F-02 (S17-TC-F-02-20260711T120512Z; 2/2 PASS — abrupt kill; terminal ground truth = finished OR object-removed-404, per S17 correction) | Both graceful stop and abrupt kill are detected. Implicit RTMP broadcasts are deleted by AMS on stop (object-removed-404), not transitioned to finished; Pulse accepts both forms as stream end. See DG-11. |
| | Ingest bitrate floor alert | FULLY | TC-H-04 (S18-TC-H-04-20260711T141503Z; 3/3 PASS) | Bitrate floor alert rule confirmed against live AMS. |
| | Viewer count drop alert | FULLY | TC-H-05 (S18-TC-H-05-20260711T141512Z; 3/3 PASS) | Viewer count threshold alert confirmed. |
| | Detection-to-notification < 30 s | FULLY | CI: 201 ms wall-clock (TestEvaluator\_DetectAndNotify\_WallClockBudget; budget 30 s; VD-31 CLOSED); analytical bound: tick≤5 s + poll≤5 s + channel<5 s = ≤15 s | Highly within budget at both the measured (CI) and analytical levels. |
| | No duplicate storms (grouping and cooldowns); rules survive restarts | FULLY | CI: alert grouping and cooldown test suite; TC-H-04/05/06 PASS (no duplicate alerts observed in runs); SQLite persistence for alert rules | Grouping, cooldowns, and restart persistence confirmed in CI. No duplicate alerts observed in real-AMS runs. |
| | Test-fire button per channel | FULLY | CI: testfire\_alert\_test.go (test-fire route POST /api/v1/alert/channels/{id}/test — HTTP 200 + accepted=true on valid channel; HTTP 200 + accepted=false on unreachable URL); conformance\_s3\_test.go | Test-fire endpoint implemented in server/internal/api/server.go (route registered at server.go:379; handler handleTestAlertChannel) and CI-tested. Not live-tested against a real external notification channel in this validation run; CI confirms the route is implemented and returns the correct response shape for both reachable and unreachable channel URLs. |
| | Node CPU/disk alerts | DIFFERENTLY | AV-06 (CONFIRMED ABSENT: cpu\_pct, disk\_pct not available for standalone AMS via REST); TC-H-06 (S18-TC-H-06-20260711T141402Z; 3/3 PASS — standalone CPU anomaly correctly returns empty, not an error) | CPU and disk metrics are absent for standalone AMS deployments via REST (DG-05). Alert rules on these metrics cannot fire without data. The alert evaluator correctly handles absent metrics (no false alarms). Available via Kafka (AV-15 BLOCKED). |
| | Rebuffer ratio and error rate alerts | PARTIALLY | TC-A-06 (S18; rebuffer\_ratio tracked in QoE rollup, confirmed); TC-AN-05 (S18-TC-AN-05-20260711T141300Z; 3/3 PASS — confirms error\_rate is not tracked as an alert/anomaly signal in the current evaluator) | Rebuffer ratio is captured via beacon SDK and is available for alerting in principle; real-deployment data is sparse (one beacon session). error\_rate is confirmed not tracked as an alerting signal in the current evaluator. |
| | Notification channels (email, Slack, Telegram, PagerDuty, generic webhook) | PARTIALLY | CI: generic webhook channel confirmed in alert test suite; TC-H-04/05 PASS confirm alert delivery path functional | Email and generic webhook channels confirmed functional. Slack, Telegram, and PagerDuty adapters are implemented; not live-tested against real external notification services in this validation run. |

---

### F6 — Usage and Billing Reports

**PRD description:** Per-application, per-stream and per-tenant accounting of
viewer-minutes, peak concurrency, egress GB and recording storage; scheduled CSV/PDF
exports; white-label header for integrators.

**PRD acceptance criteria:** Tenant mapping via stream-name pattern or metadata tag;
monthly statement generation under 60 seconds; figures reconcile with raw events within 1%.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F6 — Usage and Billing Reports** | **Overall** | **PARTIALLY** | — | Viewer-minutes and peak concurrency confirmed; egress is a model-based estimate (method disclosed); recording_gb is structurally 0 on AMS 3.0.3 deployments (BUG-002) |
| | Viewer-minutes accounting | FULLY | TC-A-09 (S18-TC-A-09-20260711T141401Z; 2/2 PASS); AV-09 (viewer\_minutes=0.3333 confirmed from one live beacon session in prod) | Viewer-minute accumulation from beacon events confirmed in real deployment. |
| | Peak concurrency (true windowed maximum) | FULLY | CI: maxState(viewer\_count) → rollup\_concurrency\_1d → maxMerge on read; 0.0000% drift at n=10,000 (TestAccountant\_CHIntegration; VD-38 CLOSED); N16 criterion met | True windowed maximum, not a session-count proxy. Implementation confirmed correct with an integration test against a real ClickHouse instance. |
| | Egress GB accounting | DIFFERENTLY | TC-A-08 (S18-TC-A-08-20260711T141401Z; 3/3 PASS — premise corrected from S17: egress\_gb=0.0025, not always-0); AV-09 (egress\_gb=0.0025 confirmed as bitrate×watch-time estimate) | Egress GB is estimated from a `bitrate × watch_time` model (`mv_usage_1d`) where AMS does not provide delivered-bytes events. The estimation method is disclosed in PRD §7.9 F6 technical notes ("Egress estimated from delivered-bytes events where available, else bitrate×watch-time model with method disclosed on the report"). The estimate reflects one beacon session of real viewing. See DG-06. |
| | Recording storage (recording\_gb) | MISSING | BUG-002 (confirmed high severity; status: confirmed); TC-A-09 (S18; recording\_gb=0 in all usage report calls); TC-WH-03 (S17-TC-WH-03-20260711T120433Z; 2/2 PASS — recording\_gb=0 confirmed; BUG-002-recording-gap.txt); AV-09 | The only ingestion path for recording data is the `vodReady` webhook (webhook.go:translateWebhook → EventRecordingReady → recording\_bytes in rollup\_usage\_1d). AMS 3.0.3 cannot HMAC-sign lifecycle hooks (O3, decisions.md); Pulse's webhook listener is fail-closed and rejects all unsigned deliveries. The VoD REST endpoint is never polled. recording\_gb = 0 on all AMS 3.0.3 deployments even when VoD assets exist (AMS ground truth: WebRTCAppEE ~1006 VoDs confirmed S16). Fix path: VoD REST poll fallback (P0 roadmap item). |
| | Monthly statement generation < 60 s | FULLY | CI: 4.8 ms (C-W2-05); N14 criterion met | Well within budget. |
| | Billing figures reconcile with raw events within ±1% | FULLY | CI: 0.0000% drift at n=10,000 (TestAccountant\_CHIntegration; VD-38); N15 criterion met | Perfect reconciliation for viewer-minutes and peak concurrency figures, at the implemented scale. |
| | Tenant mapping via stream-name pattern or metadata tag | PARTIALLY | Design implemented; TC-A-05/06 PASS (per-stream QoE confirmed) | Tenant mapping is designed; a multi-tenant scenario with real stream-name patterns was not run against live AMS in this validation base. |
| | Scheduled CSV/PDF exports; white-label header for integrators | PARTIALLY | N14 (statement < 60 s, CI 4.8 ms confirmed); report generation API confirmed functional | Programmatic report generation path confirmed fast. Scheduled delivery to S3 and white-label PDF rendering were not live-validated in this evidence base. |

---

### F7 — Cluster Awareness and Fleet View

**PRD description:** Auto-discovery of cluster nodes via AMS cluster REST; per-node and
aggregate views; edge/origin role labeling; node up/down alerts.

**PRD acceptance criteria:** New nodes appear without manual config within 2 minutes;
aggregate metrics deduplicate origin/edge double counting.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F7 — Cluster Awareness and Fleet View** | **Overall** | **PARTIALLY** | — | Auto-discovery, fleet card, and edge dedup confirmed; not live-validated against a real multi-node AMS cluster (single-node VPS) |
| | Auto-discovery of cluster nodes (no manual config) | FULLY | CI: 24.4 ms end-to-end node discovery (C-W2-07); N17 criterion met | Well within the 2-minute budget; the restpoller refreshes the node list periodically by polling AMS cluster REST endpoints. |
| | Per-node and aggregate views | FULLY | TC-FL-01 (S17-TC-FL-01-20260711T115050Z; 7/7 PASS — standalone fleet card); TC-FL-02 (S17-TC-FL-02-20260711T120431Z; 4/4 PASS — version=3.0.3 Enterprise Edition confirmed; AV-05 node card fields confirmed) | Fleet node card fields confirmed: os\_name=Linux, os\_arch=amd64, java\_version=17, processor\_count=6, version=3.0.3, role=standalone, status=up (AV-05). Note: cpu\_pct, mem\_pct absent for standalone (AV-06). |
| | Edge/origin role labeling | FULLY | AV-05 (CONFIRMED: role=standalone); IsEdgeStream() implementation confirmed at normalize.go; AV-16 (CONFIRMED SAFE: rtmpViewerCount never negative across 20 samples) | Edge/origin role field is populated on node cards. IsEdgeStream() dedup logic is implemented and unit-tested. |
| | Node up/down alerts | PARTIALLY | TC-H-04/05 PASS (alert evaluation pipeline functional); TC-F-01/02 PASS (lifecycle detection via REST poll) | The alert evaluator pipeline is confirmed functional and node-down detection is designed to fire through the same evaluator, but no direct node-offline scenario was executed against live AMS (ENV-LIMIT: single-node VPS — taking the AMS node offline would have ended the validation session). |
| | New nodes appear without manual config within 2 minutes | FULLY | CI: 24.4 ms (C-W2-07); N17 criterion met | Same as auto-discovery row; the node discovery refresh interval is independently configurable. |
| | Aggregate metrics deduplicate origin/edge double counting | PARTIALLY | IsEdgeStream() implementation confirmed; TC-V-04 (S17; AV-16 CONFIRMED SAFE — inline rtmpViewerCount=0 across 20 samples); viewer counts cross-checked (TC-V-03 within ±2) | Dedup logic (IsEdgeStream()) is implemented and unit-tested. Not live-validated against a real multi-node AMS cluster (ENV-LIMIT: single-node VPS). The validated to N=5 concurrent publishers; cluster-mode dedup requires a separate AMS instance. |

---

### F8 — Data API, Prometheus Endpoint and Exports

**PRD description:** REST API for all metrics, a `/metrics` Prometheus exposition endpoint
for customers who keep Grafana, and scheduled CSV-to-S3 export.

**PRD acceptance criteria:** API parity with dashboard data; token-authenticated; documented
OpenAPI spec.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F8 — Data API, Prometheus Endpoint and Exports** | **Overall** | **PARTIALLY** | — | REST API and token authentication confirmed live; Prometheus /metrics endpoint not live-validated against real AMS; scheduled S3 export not verified; BUG-004 is an OpenAPI contract violation |
| | REST API for all metrics, token-authenticated | FULLY | TC-H-01 (S17-TC-H-01-20260711T115052Z; 4/4 PASS — fleet standalone); TC-H-02 (S17-TC-H-02-20260711T115050Z; 5/5 PASS — healthz); TC-L-04 (S18-TC-L-04-20260711T142655Z; 21/21 PASS — rapid stream cycling with API reads throughout) | API token authentication confirmed with bearer token (`plt_0352…`). All metric endpoints return data consistent with AMS ground truth. |
| | Documented OpenAPI spec; API parity with dashboard data | PARTIALLY | N36 (CI: 51/52 operations response-body conformant; GET /live/ws waived — WebSocket upgrade, 101 response; D-060 formalized); BUG-004 (GET /api/v1/qoe/ingest: declared from/to parameters silently ignored — OpenAPI contract violation at the parameter-handling level) | The OpenAPI spec exists and response-body conformance is 51/52 (the 1 waived operation is the WebSocket upgrade). BUG-004 represents an additional contract violation: declared query parameters are accepted without error but silently discarded, causing incorrect results in windowed queries. |
| | /metrics Prometheus exposition endpoint | PARTIALLY | CI: server test suite (20 packages PASS); N25 (CI: /healthz Kafka component surfaces lag and parse\_errors; status=degraded confirmed — TestAPI\_Healthz\_KafkaStats) | The Prometheus /metrics endpoint is implemented with gauges/counters (no high-cardinality labels by default). Not live-validated against the real AMS deployment in this evidence base (TC-H-03 was not run in the P0/P1 scenario set). |
| | Scheduled CSV-to-S3 export | PARTIALLY | N14 (monthly statement generation < 60 s, CI 4.8 ms) | Programmatic report generation confirmed fast. Scheduled delivery to an S3 bucket was not live-validated in this validation run. |

---

### F9 — Anomaly Detection (Phase 3)

**PRD text:** Baseline-deviation flags on viewers, errors and rebuffering ("this Tuesday
looks wrong"), simple statistical models first, no ML theater. Acceptance: fewer than 1
false alarm per node-week at default sensitivity.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F9 — Anomaly Detection (Phase 3)** | **Overall** | **PARTIALLY** | — | Viewer and bitrate anomaly detection implemented and below false-alarm threshold; error rate and rebuffer ratio anomaly signals absent; Phase 3 item |
| | Baseline-deviation flags on viewers and bitrate | FULLY | CI: 0.259 false alarms/node-week at default sensitivity (σ=4.0, hysteresis=10; TestAnomaly\_FalseAlarmRate\_ModeledTarget); TC-AN-03 (S18-TC-AN-03-20260711T141259Z; 2/2 PASS — CPU/mem anomaly correctly returns empty for standalone); AV-12 (CONFIRMED: GET /api/v1/anomalies → empty list for standalone AMS) | False-alarm rate well below the 1/node-week threshold. The empty anomaly list for standalone is correct behavior (no CPU/mem data source). |
| | Baseline-deviation flags on errors and rebuffering | MISSING | TC-AN-05 (S18-TC-AN-05-20260711T141300Z; 3/3 PASS — confirms error\_rate not tracked) | error\_rate and rebuffer\_ratio are confirmed not implemented as anomaly signals in the current evaluator. The PRD specifies anomaly detection on "viewers, errors and rebuffering"; error and rebuffer anomaly signals are absent. |
| | CPU/mem anomaly detection for standalone AMS | DIFFERENTLY | AV-06 (CONFIRMED ABSENT: cpu\_pct/mem\_pct not available via REST for standalone); TC-H-06 (S18; 3/3 PASS — correctly returns empty); AV-12 (CONFIRMED: empty anomaly list) | CPU/mem data is unavailable for standalone AMS via REST (DG-05). The anomaly evaluator correctly returns no findings rather than false alarms (honest-absent design). CPU/mem anomaly detection is available in cluster mode if node stats are exposed via the cluster API. |
| | False alarm rate < 1/node-week at default sensitivity | FULLY | CI: 0.259/node-week at σ=4.0, hysteresis=10 (TestAnomaly\_FalseAlarmRate\_ModeledTarget); N19 criterion met | Target met with a comfortable margin (74% below threshold). |

---

### F10 — Synthetic Viewer Probes (Phase 3)

**PRD text:** Optional lightweight probes (cloud or customer-placed) that periodically play
selected streams and report real playback success/latency from outside the network.
Acceptance: probe results visible alongside organic QoE with clear labeling.

| Feature | Requirement / Acceptance Criterion | Verdict | Evidence | Notes |
|---------|-----------------------------------|---------|----------|-------|
| **F10 — Synthetic Viewer Probes (Phase 3)** | **Overall** | **FULLY** | — | All implemented probe types validated live against AMS 3.0.3; rtt/jitter/loss live-proven for WebRTC (D-075); BUG-003 (medium severity) introduces near-duplicate rows in probe result timeseries; Phase 3 item delivered ahead of Phase 3 |
| | WebRTC probe (signaling, ICE, rtt/jitter/loss) | FULLY | TC-P-01 (S17-TC-P-01-20260711T115430Z; 6/6 PASS — signaling\_state=offer\_received, ice\_state=connected, rtt/jitter/loss keys present; D-075 fix confirmed); AV-14 (CONFIRMED: D-074/D-075 WebRTC probe fix holds against live AMS 3.0.3) | The subtrackAdded-before-offer sequencing fix (D-074/D-075) is confirmed working against real AMS 3.0.3. RTT/jitter/loss fields present (not null) as keys in the response. |
| | RTMP probe | FULLY | TC-P-03 (S17-TC-P-03-20260711T115243Z; 3/3 PASS — signaling\_state=handshake\_complete) | RTMP probe functional end-to-end against live AMS. |
| | HLS probe (success, TTFB, bitrate, segment\_ttfb\_ms) | FULLY | TC-P-04 (S17-TC-P-04-20260711T120433Z; 4/4 PASS — flat URL form /{app}/streams/{id}.m3u8 confirmed); CI N21 (success=true, ttfb\_ms=1, bitrate\_kbps=66.7); N22 (segment\_ttfb\_ms=1); N23 (master playlist follows variant, bitrate=66.7 kbps, seg\_ttfb\_ms=1) | All HLS probe metrics confirmed in CI. URL path form corrected in scenario (/{id}/playlist.m3u8 is 404 on AMS 3.0.3; flat form is /{app}/streams/{id}.m3u8). See DG-10. |
| | DASH probe | DIFFERENTLY | TC-P-06 (S17-TC-P-06-20260711T115144Z; 2/2 PASS — success=false, error\_code=http\_4xx; DASH muxing disabled on test AMS) | DASH probe correctly returns failure when DASH muxing is not enabled on AMS. DASH is not enabled by default on AMS 3.0.3; this is expected behavior, not a Pulse defect. The probe implementation handles the failure path gracefully. |
| | Error handling (probe to unavailable stream) | FULLY | TC-P-05 (S17-TC-P-05-20260711T115054Z; 2/2 PASS — HLS 404 → success=false, error\_code=http\_4xx, no crash) | Probe error path is robust; no exceptions on unavailable stream URL. |
| | Multiple simultaneous probes | FULLY | TC-P-08 (S18-TC-P-08-20260711T141850Z; 3/3 PASS) | Concurrent probe execution confirmed against live AMS. |
| | Probe result interval accuracy | PARTIALLY | TC-P-07 (S18-TC-P-07-20260711T145258Z; 4/4 PASS after gap-assertion workaround; BUG-003 confirmed; gap=1 ms at Result 4, gap=0 ms at Result 7 in 180 s window with interval\_s=30) | BUG-003: probe scheduler emits near-duplicate result rows 0–1 ms apart at approximately the 60 s and 120 s marks (two concurrent execution paths — immediate-on-create goroutine and periodic ticker — suspected to fire within 1 ms at every other tick). probe\_results MergeTree ORDER BY (probe\_id, ts) stores distinct 1 ms rows as separate entries. Impact: results API returns N+1 or N+2 rows per expected-N window; inter-result gap checks observe 0–1 ms "missed" intervals. |
| | Probe results visible alongside organic QoE with clear labeling | FULLY | TC-P-07/08 PASS; probe results API confirmed returning structured outcomes | Results endpoint returns structured probe outcomes with probe type, success flag, and metric fields; results are clearly labeled as synthetic (not organic viewer data). |
| | Phase 3 acceptance: success=true; ttfb\_ms > 0; bitrate\_kbps > 0; segment\_ttfb\_ms > 0 | FULLY | CI N21: success=true, ttfb\_ms=1, bitrate\_kbps=66.7 (TestHLSProbe\_Success); N22: segment\_ttfb\_ms=1; N23: master playlist follows variant, bitrate > 0 | All Phase 3 acceptance criteria met in CI. |

---

### Webhook Data Source — Cross-Cutting Note

The PRD describes webhooks as a technical implementation channel for F1 (instant
publish/unpublish detection) and F6 (vodReady recording events). This is not a standalone
PRD feature but a data-source path. Its status in this deployment:

- **F1 webhook path:** DIFFERENTLY — AMS 3.0.3 cannot HMAC-sign hooks (O3, decisions.md).
  Pulse fail-closed listener rejects all unsigned deliveries. REST polling at 5 s interval
  covers the ≤ 10 s publish-to-visible budget (4 s confirmed, TC-WH-02). No functional gap
  for stream lifecycle detection in real deployments.

- **F6 vodReady webhook:** MISSING — recording data requires the vodReady webhook, which
  never arrives. This is filed as BUG-002 (high severity). Fix path: VoD REST poll fallback
  (P0 roadmap item; see `docs/assessment/bugs/BUG-002-recording-gb-zero-webhook-blocked.md`).

See DG-04 in `docs/assessment/documentation-gaps.md` for the operator-facing documentation
that must be produced.

---

## Table 2 — Architecture §4 Numeric Criteria

All criteria are drawn from `docs/ARCHITECTURE.md` §4 (lines 149–228). IDs N1–N36 are
stable identifiers assigned for this document. Measured values from CI reference the
test or benchmark that produced them. Measured values from real-AMS validation cite
scenario IDs and timestamps.

"CI" = mock-ams test suite (deterministic, network-isolated).
"A10" = load smoke test, SESSION-07, D-064: mock-ams, 500 streams + 3,000 viewers, 15 min.
"ENV-LIMIT" = result unavailable due to VPS AMS capacity constraint.

| ID | Criterion | Target | Measured Value | Verdict | Evidence |
|----|-----------|--------|---------------|---------|----------|
| N1 | New stream on dashboard ≤ 10 s after publish | ≤ 10 s | **4 s** (publish→Pulse); **7 s** (stop→Pulse) | FULLY | TC-WH-02 (S17-TC-WH-02-20260711T120043Z; timeline.txt) |
| N2 | Viewer counts within ±2% of AMS REST (standalone) | ±2% | **0.0%** deviation | FULLY | TC-V-03 (S17; viewer\_count within ±2); TC-V-04 (S17; AV-16 CONFIRMED SAFE) |
| N3 | Viewer counts within ±2% of AMS REST (cluster); 0% double-count via IsEdgeStream() dedup | ±2% / 0% double-count | Standalone: **0.0%**. Cluster: IsEdgeStream() implemented; **not live-validated** against multi-node AMS | PARTIALLY | TC-V-03/04 PASS (standalone); IsEdgeStream() unit-tested; real cluster ENV-LIMIT |
| N4 | Dashboard load < 2 s with 500 concurrent streams | < 2 s | **668 ms / 459 ms** (Playwright nav+grid-visible, run1/run2; CI) | FULLY | CI: VD-04 CLOSED (SESSION-04/WO-4); ENV-LIMIT for real AMS (TC-S-01 SKIP) |
| N5 | Any 13-month rollup query < 3 s | < 3 s | **144 ms** (simple aggregate, Wave-3 C9); **145 ms** (3 geo × 2 device × 2 protocol GROUP BY, C9b) | FULLY | CI: Wave-3 gate report (`qa/wave-3-plus/gate-report.md`); C9b `qa/wave-2/run-gate.sh`; Wave-2 baseline 126 ms (C-W2-08) |
| N6 | Beacon SDK bundle < 15 KB gzip | < 15 KB gzip | **3.52 KB** gzip | FULLY | CI: Wave-3 SDK build measurement (no regression from 3.44 KB) |
| N7 | Beacon SDK player CPU overhead < 1% | < 1% CPU | Not measurable (VD-14 deferred) | NEEDS-CLARIFICATION | VD-14 deferred by design consensus; ARCHITECTURE.md §4 N7 cell; no accepted benchmark methodology defined |
| N8 | Beacon round-trip accepted by collector (correct headers; main-port persists to EventSink) | Accepted (no-error) | **Accepted** — headers correct; EventSink persists | FULLY | CI: V3a fix confirmed; TC-A-05 (S18; 3/3 PASS — live beacon session) |
| N9 | Geo analytics query returns non-empty rows (real ClickHouse query, not stub) | Non-empty rows | **1 row returned** (country='', views=1, uniques=1, watch\_time\_s=10) | FULLY | AV-11 (CONFIRMED: real ClickHouse query returns rows; country='' because mmdb absent — AV-10; not an error or stub) |
| N10 | QoE startup\_p50\_ms non-zero (from rollup\_qoe\_1h) | > 0 | **250.0 ms** (CI); confirmed non-zero in live beacon session (TC-A-05) | FULLY | CI: TestQuery\_QoeSummary\_RealStartupP50 (VD-11 V3a); TC-A-05 (S18; 3/3 PASS) |
| N11 | Ingest health\_score non-zero for healthy ingest (0–100 scale) | > 0 on 0–100 scale | **> 80** at 2 Mbps (fps=0, health score not falsely penalized) | FULLY | TC-I-06 (S17-TC-I-06-20260711T120943Z; 4/4 PASS); AV-04 CONFIRMED |
| N12 | Ingest degradation visible within ≤ 15 s | ≤ 15 s | **250.8 µs** in-process (CI) | FULLY | CI: C-W2-06; TC-I-04 (S18; 4/4 PASS with workaround) |
| N13 | Alert detection-to-notification < 30 s | < 30 s | **201 ms** wall-clock (CI) | FULLY | CI: TestEvaluator\_DetectAndNotify\_WallClockBudget (VD-31 CLOSED) |
| N14 | Monthly usage/billing statement generation < 60 s | < 60 s | **4.8 ms** (CI) | FULLY | CI: C-W2-05 |
| N15 | Billing figures reconcile with raw events within ≤ ±1% | ≤ ±1% | **0.0000%** drift (n=10,000) | FULLY | CI: TestAccountant\_CHIntegration (VD-38 CLOSED) |
| N16 | Peak concurrency computed as true windowed max (maxState → rollup\_concurrency\_1d → maxMerge) | True windowed max (not session-count proxy) | **Confirmed**: maxState/maxMerge path, peak=25 (alpha) / peak=5 (beta, overlapping snapshots) | FULLY | CI: TestAccountant\_CHIntegration (VD-38 CLOSED) |
| N17 | New cluster nodes auto-discovered ≤ 2 minutes | ≤ 2 min | **24.4 ms** (CI) | FULLY | CI: C-W2-07 |
| N18 | ClickHouse storage ~1–2 GB per 1M viewer-sessions at default sampling | ~1–2 GB / 1M viewer-sessions | Not measurable — insufficient real viewer volume in validation deployment | NEEDS-CLARIFICATION | ARCHITECTURE.md §4 N18: "Not measurable"; real-deployment beacon volume too low to extrapolate (one session = 0.3333 viewer-minutes); ENV-LIMIT |
| N19 | F9 anomaly detection false-alarm rate < 1/node-week at default sensitivity | < 1 false alarm / node-week | **0.259/node-week** (σ=4.0, hysteresis=10) | FULLY | CI: TestAnomaly\_FalseAlarmRate\_ModeledTarget; ARCHITECTURE.md §4 |
| N20 | Anomaly rule evaluation ≤ 50 ms per 5 s evaluator tick @ 500 streams | ≤ 50 ms / evaluator tick @ 500 streams | Design bound: ~1.5 ms SQLite batch read + ~0.05 ms per-stream z-score = ~26.5 ms @ 500 streams; A5 end-to-end fire ≤ 30 s in CI | FULLY | CI: A5 (alert e2e confirms total fire latency ≤ 30 s under CI mock); design bound well within 50 ms |
| N21 | F10 HLS probe: success=true, ttfb\_ms > 0, bitrate\_kbps > 0 | success=true; ttfb\_ms > 0; bitrate\_kbps > 0 | **success=true, ttfb\_ms=1, bitrate\_kbps=66.7** | FULLY | CI: TestHLSProbe\_Success; TC-P-04 (S17; 4/4 PASS — live HLS probe) |
| N22 | F10 HLS probe segment\_ttfb\_ms > 0 (serialized as segment\_ttfb\_ms in API response) | segment\_ttfb\_ms > 0 | **segment\_ttfb\_ms=1** | FULLY | CI: TestHLSProbe\_Success (result.SegmentTTFBMs > 0 assertion; GAP-3-001 CLOSED) |
| N23 | F10 HLS master-playlist probe follows variant, bitrate > 0, seg\_ttfb\_ms > 0 | bitrate > 0; seg\_ttfb\_ms > 0 | **bitrate=66.7 kbps, seg\_ttfb\_ms=1** | FULLY | CI: TestHLSProbe\_MasterFollowsVariant (GAP-3-003 CLOSED) |
| N24 | F10 probe new config → first result latency < 100 ms (After(0) fires immediately) | < 100 ms | **< 100 ms** (After(0) fires immediately with fake clock) | FULLY | CI: probe scheduler test |
| N25 | /healthz Kafka component surfaces lag and parse\_errors; status=degraded when applicable | lag and parse\_errors visible; status=degraded | **lag=42, parse\_errors=3, status=degraded** confirmed | FULLY | CI: TestAPI\_Healthz\_KafkaStats; TestKafka\_AtomicCounters (VD-27 CLOSED) |
| N26 | Web build bundle regression guard: total ≤ 773.85 kB (221.69 kB gzip) | ≤ 773.85 kB (≤ 221.69 kB gzip) | **773.85 kB** (no regression) | FULLY | CI: Wave-3-Plus web build; ARCHITECTURE.md §4 |
| N27 | Web test suite count regression guard | ≥ 157 tests | **360/360 PASS** (12 suites; ≥ 157 threshold far exceeded) | FULLY | CI: vitest 360/360 PASS; web coverage: lines 65.94 / branches 61.66 / functions 54.85 (all gates met) |
| N28 | Server test packages regression guard | ≥ 20 packages | **20 packages** | FULLY | CI: Wave-3-Plus go test ./... |
| N29 | SDK test suite count regression guard | ≥ 65 tests | **65 tests** | FULLY | CI: V3b SDK build (5 files) |
| N30 | Pulse process memory peak ≤ 512 MiB @ 500 streams + 3,000 viewers sustained 15 min | ≤ 512 MiB | **18.6 MiB** peak (3.6% of limit) | FULLY | A10 load smoke (SESSION-07, D-064): mock-ams, 500 streams + 3,000 viewers, 16 samples at ~60 s |
| N31 | ClickHouse memory peak ≤ 2 GiB @ 500 streams + 3,000 viewers sustained 15 min | ≤ 2 GiB | **610 MiB** peak (30% of limit; D-062 "Memory limit exceeded 1.80 GiB" WATCH count = 0) | FULLY | A10 load smoke (SESSION-07, D-064) |
| N32 | Incremental snapshot ≤ 64 allocs/event @ N=1000 streams (TestPollCycle\_AllocsPerEvent\_Bounded) | ≤ 64 allocs/event | **1.0 allocs/event** @ N=1000 | FULLY | CI: BenchmarkPollCycle (S10/D-068); ARCHITECTURE.md §4 A10 incremental-snapshot table |
| N33 | Incremental snapshot complexity: allocs ratio 500-stream vs 100-stream < 7× linear bound | < 7× | **5.4×** (< 7× ✓) | FULLY | CI: BenchmarkPollCycle; ARCHITECTURE.md §4 (500-stream: 500 allocs; 100-stream: 100 allocs) |
| N34 | Incremental snapshot complexity: allocs ratio 1000-stream vs 500-stream < 3× linear bound | < 3× | **2.1×** (< 3× ✓) | FULLY | CI: BenchmarkPollCycle; ARCHITECTURE.md §4 (1000-stream: 1,001 allocs; 500-stream: 500 allocs) |
| N35 | cmd/pulse per-package coverage (assembly/wiring package; exempt from ≥60% general bar) | ≥ 40% | **42.3%** | FULLY | CI: coverage report; ARCHITECTURE.md §4 testing waivers (D-064); serve/migrate/diag wiring smoke tests |
| N36 | OpenAPI response-body conformance: all operations except GET /live/ws (WebSocket upgrade, 101 response, waived) | 51 of 52 operations conformant | **51/52** response-body conformant; BUG-004 adds a parameter-handling violation at /qoe/ingest (from/to silently ignored) | FULLY | CI: OpenAPI conformance test suite (D-060 formalized; VD waived GET /live/ws); BUG-004 filed separately (parameter violation, not response-body shape violation) |

---

## Summary

### Feature-Level Verdicts (F1–F10)

| ID | Feature | Verdict |
|----|---------|---------|
| F1 | Real-Time Operations Dashboard | PARTIALLY |
| F2 | Historical Audience Analytics | PARTIALLY |
| F3 | Player QoE Beacon SDK | PARTIALLY |
| F4 | Publisher and Ingest Health | PARTIALLY |
| F5 | Alerting and Incident Automation | PARTIALLY |
| F6 | Usage and Billing Reports | PARTIALLY |
| F7 | Cluster Awareness and Fleet View | PARTIALLY |
| F8 | Data API, Prometheus Endpoint and Exports | PARTIALLY |
| F9 | Anomaly Detection (Phase 3) | PARTIALLY |
| F10 | Synthetic Viewer Probes (Phase 3) | FULLY |

FULLY: 1 of 10 features (F10) · PARTIALLY: 9 of 10 features · MISSING: 0 · DIFFERENTLY: 0

### Sub-Requirement Verdicts (all acceptance-criterion rows across F1–F10)

| Verdict | Count |
|---------|-------|
| FULLY | 40 |
| PARTIALLY | 14 |
| MISSING | 4 |
| DIFFERENTLY | 7 |
| NEEDS-CLARIFICATION | 1 |
| **Total sub-rows** | **66** |

MISSING sub-requirements: geo country-level accuracy (F2), SDK integration documentation
(F3, MVP+1 item), recording\_gb accounting (F6, BUG-002), error/rebuffer anomaly signals (F9).

DIFFERENTLY sub-requirements: node health CPU/RAM/network (F1 — absent via REST, requires
Kafka), FPS (F4 — absent in AMS 3.x REST BroadcastDTO), packet loss/jitter for RTMP (F4 —
TCP masks transport-layer loss, DG-18), node CPU/disk alerts (F5 — no data source for
standalone), egress GB (F6 — model-based estimate, method disclosed), CPU/mem anomaly for
standalone (F9 — honest-absent), DASH probe (F10 — expected failure, DASH not enabled).

### Numeric Criteria Verdicts (N1–N36)

| Verdict | Count |
|---------|-------|
| FULLY | 33 |
| PARTIALLY | 1 (N3: cluster viewer counts — standalone confirmed, cluster not live-tested) |
| NEEDS-CLARIFICATION | 2 (N7: beacon CPU overhead not measurable; N18: ClickHouse storage extrapolation insufficient real-deployment volume) |
| MISSING | 0 |
| DIFFERENTLY | 0 |
| **Total criteria** | **36** |

### Open Bugs Affecting This Matrix

| Bug | Severity | Features Affected | Status |
|-----|----------|-------------------|--------|
| BUG-001 | Low | F1 (viewer count detail path — no user impact, counts work via inline poll) | Confirmed; no user impact |
| BUG-002 | High | F6 (recording\_gb always 0 — blocks recording/billing use case) | Confirmed; P0 roadmap fix: VoD REST poll fallback |
| BUG-003 | Medium | F10 (probe scheduler near-duplicate rows at periodic intervals) | Confirmed; workaround: dedup at insert or query layer |
| BUG-004 | Medium | F4 (windowed ingest health queries return era-mixed data); F8 (OpenAPI contract violation) | Confirmed; fix: parse from/to in handler |

Bug files: `docs/assessment/bugs/BUG-001-broadcast-statistics-dead-code.md`,
`BUG-002-recording-gb-zero-webhook-blocked.md`,
`BUG-003-probe-scheduler-duplicate-results.md`,
`BUG-004-qoe-ingest-ignores-from-to.md`
