<!--
  ╔══════════════════════════════════════════════════════════════════════════╗
  ║  DRAFT — OPERATOR REVIEW REQUIRED BEFORE SHARING WITH ANT MEDIA OR      ║
  ║          ANY EXTERNAL PARTY                                              ║
  ╚══════════════════════════════════════════════════════════════════════════╝
-->

> **DRAFT — OPERATOR REVIEW REQUIRED BEFORE SHARING WITH ANT MEDIA OR ANY EXTERNAL PARTY**
>
> Sections 3 (Marketplace Readiness) and 6 (Open Questions) contain rows that
> explicitly require operator contact before the document is usable externally.
> This draft is produced at S19 close and is an internal working document only.

---

# Pulse v0.3.0 — Final Product Assessment

**Product:** Pulse: Self-Hosted Analytics, QoE Monitoring and Alerting for Ant Media Server  
**Assessed against:** AMS 3.0.3 Enterprise Edition (build 20260504\_1443)  
**Validation program:** Sessions S17–S19, 2026-07-11  
**Validation corpus:** 50 automated scenario scripts (P0 + P1), run from
`qa/realams/` harness against the live AMS deployment at `161.97.172.146:5080`  
**Authors:** ORCH-00 + QA-01 (S17–S19 session agents)  
**Source:** `docs/assessment/prd-validation-matrix.md` (Phase 7),
`docs/assessment/capability-map.md` (Phase 1)  

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Product Completeness Score](#2-product-completeness-score)
3. [Marketplace Readiness Checklist](#3-marketplace-readiness-checklist)
4. [Missing Opportunities](#4-missing-opportunities)
5. [Prioritized Roadmap](#5-prioritized-roadmap)
6. [Open Questions for the Ant Media Team](#6-open-questions-for-the-ant-media-team)

---

## 1. Executive Summary

Pulse is a self-hosted observability product for Ant Media Server that
delivers real-time stream dashboards, historical QoE analytics, fleet health
monitoring, alerting, synthetic probes, usage reports, and anomaly detection
— all through a single-pane-of-glass UI backed by a documented REST API and
Prometheus endpoint.

This assessment is the output of an eight-phase validation program run from
first principles against a real, production-grade AMS 3.0.3 Enterprise
deployment. The program covered:

- **50 automated scenario scripts** exercising 46 distinct functional and
  behavioral requirements (P0 priority: 26 scripts; P1 priority: 24 scripts).
- **Direct AMS-REST cross-checks**: every claim about a viewer count,
  bitrate value, or stream state was verified against the raw AMS REST
  endpoint in the same request window, not inferred from the Pulse UI alone.
- **11 bugs found and filed** (BUG-001 through BUG-011), demonstrating that
  the methodology is capable of finding real defects, not just confirming
  expected behavior.

### Headline numbers

| Metric | Value |
|--------|-------|
| P0 scenario results | 25 PASS / 1 SKIP / 0 FAIL |
| P1 scenario results | 21 PASS / 3 SKIP / 0 FAIL |
| Combined (50 scripts) | **46 PASS / 4 SKIP / 0 FAIL** |
| New stream on dashboard | **4 s** (≤ 10 s PRD requirement met) |
| Stream removal from dashboard | **7 s** (≤ 10 s PRD requirement met) |
| Alert detection-to-notification | **201 ms** wall-clock (≤ 30 s requirement met) |
| Bitrate normalization accuracy | AMS 2,067,136 bits/s → Pulse 2,067 kbps, within ±10% |
| Beacon SDK gzipped bundle | **3.52 KB** (limit: 15 KB) |
| Pulse process memory at load | **18.6 MiB** peak @ 500 streams + 3,000 viewers (limit: 512 MiB) |
| PRD sub-requirements FULLY met | **44 of 66** (66.7% simple; 84.5% weighted) |
| Architecture numeric criteria FULLY met | **33 of 36** (91.7%) |

### What was validated

The core instrumentation pipeline is proven end-to-end: AMS emits stream
lifecycle events, Pulse detects them within the 10 s budget via REST
polling, normalizes bitrate and viewer counts correctly, stores events in
ClickHouse, and surfaces them through a token-authenticated REST API. The
beacon SDK reaches the collector, populates QoE rollups, and is well within
the 15 KB size gate. Alerting, fleet display, and synthetic probes all work
against the live AMS.

### What was not validated

Four scenarios were honestly skipped rather than forced to pass:

1. **TC-APP-02** — IP-blocked app handling: no IP-blocked apps exist on the
   test AMS (all 4 apps answer 200 from the VPS IP). The 403-handling code
   path was not exercised against a live trigger. Unblocked by creating a
   test app with `remoteAllowedCIDR=127.0.0.1`.
2. **TC-V-06** — HLS viewer count decay: AMS hlsViewerCount is a
   sliding segment-request window, not a session count. After stopping 3 of
   5 real viewers the AMS count reached 38 and did not drop below 4 within
   90 s. This is an AMS semantic, not a Pulse defect; documented as DG-01.
3. **TC-L-05** — Simultaneous stream cycling under load: VPS AMS concurrent
   RTMP capacity is approximately 5–7 streams. All additional publisher
   slots were rejected by AMS with "current system resources not enough."
4. **TC-S-01** — 20 concurrent publishers: same capacity constraint.

All scale claims at N = 500 streams and N = 3,000 viewers are backed by CI
(mock-AMS load smoke, SESSION-07, D-064), not the live AMS on this VPS.

### Honest limitations of this assessment

- All real-AMS results are from a **single-node VPS** deployment. Cluster
  mode edge/origin deduplication is implemented and unit-tested but not
  live-validated against a multi-node AMS.
- The **AMS trial license** lapsed 2026-07-12T12:09Z. All core sessions (S17–S28)
  were completed before expiry (operator-waived). **S29/D-091 post-lapse finding:**
  AMS EE SRT ingest is now license-gated. First enforcement observed 2026-07-13:
  SRTAdaptor rejects with "License is suspended. Not accepting the stream". RTMP
  ingest remains unaffected (TCP-stack protocol, no EE gate). SRT scenario
  TC-I-05-SRT is committed and BLOCKED; blocked-scenario-list updated accordingly.
- **Remote-viewer WebRTC stats** (RTT/jitter/loss) were verified by
  code-reading the ×1000 unit conversion at `normalize.go:185`, but
  exercised only at 0 values because all test viewers were on the same
  host as AMS. Non-zero validation requires an off-host WebRTC viewer.
- **Geo enrichment** is structurally non-functional in this deployment
  (GeoLite2-City.mmdb not deployed, `PULSE_GEO_MMDB_PATH` commented out).

---

## 2. Product Completeness Score

### Source

All counts are drawn directly from
`docs/assessment/prd-validation-matrix.md` Summary section (Phase 7
deliverable). The arithmetic is shown in full below.

### Sub-requirement verdict counts (F1–F10, 66 rows)

| Verdict | Count | Meaning |
|---------|------:|---------|
| FULLY | 44 | Implemented and validated end-to-end against real AMS |
| PARTIALLY | 12 | Core implemented; at least one sub-criterion missing or not live-validated |
| DIFFERENTLY | 7 | Implemented via a method that differs from PRD spec; delta documented |
| MISSING | 2 | Specified in PRD; not implemented or structurally non-functional |
| NEEDS-CLARIFICATION | 1 | Requirement ambiguous; Ant Media team input needed |
| **Total** | **66** | |

### Method A — Simple percentage

> Counts only full compliance as "done."

```
44 (FULLY) / 66 (total) = 66.7%
```

### Method B — Weighted percentage (headline score)

> Assigns partial credit: FULLY = 1.0, DIFFERENTLY = 0.75 (functional but
> not as specified), PARTIALLY = 0.5, NEEDS-CLARIFICATION = 0.5, MISSING = 0.

```
(44 × 1.0) + (7 × 0.75) + (12 × 0.5) + (1 × 0.5) + (2 × 0.0)
  = 44.00 + 5.25 + 6.00 + 0.50 + 0.00
  = 55.75

55.75 / 66 = 84.5%
```

**Headline: Product Completeness = 84.5% (weighted) / 66.7% (strict)**

> **Score revision note (S27/D-089):** F3 SDK-docs sub-row updated MISSING → FULLY.
> `docs/beacon-sdk.md` (452 lines) was authored at S19/D-081 (DG-07), but the matrix
> was not updated at that time. The recount adds 1 FULLY, removes 1 MISSING.
> Prior scores (as of S19 close): 65.2% strict / 83.0% weighted.

### Feature-level summary (F1–F10)

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
| F10 | Synthetic Viewer Probes (Phase 3) | **FULLY** |

FULLY: 1 of 10 features (F10). PARTIALLY: 9 of 10. MISSING: 0 at the
feature level (gaps appear at the sub-requirement level).

The single FULLY-validated feature, F10 (Synthetic Viewer Probes), was a
Phase 3 deliverable that shipped ahead of the nominal Phase 3 milestone.

### Architecture numeric criteria (N1–N36)

| Verdict | Count |
|---------|------:|
| FULLY | 33 |
| PARTIALLY | 1 (N3: cluster viewer-count dedup — standalone confirmed, multi-node cluster not live-tested) |
| NEEDS-CLARIFICATION | 2 (N7: beacon SDK CPU overhead not measurable; N18: ClickHouse storage extrapolation insufficient real-deployment volume) |
| MISSING | 0 |
| **Total** | **36** |

```
33 (FULLY) / 36 (total) = 91.7%
```

The two sub-requirements with a MISSING verdict are:
1. **Geo country-level accuracy (F2)** — GeoLite2 mmdb not deployed.
2. **Error and rebuffer anomaly signals (F9)** — `error_rate` and
   `rebuffer_ratio` confirmed absent from the anomaly evaluator.

(SDK integration documentation (F3) was MISSING at S19 close. `docs/beacon-sdk.md`
was authored at S19/D-081 (DG-07, 452 lines); score corrected at S27/D-089.)

(Recording storage (recording\_gb) accounting (F6) was MISSING through S22;
**BUG-002 FIXED S23/D-085** — VoD REST poll + `mv_recording_1d`, live-validated
TC-REC-01 with 0.02% reconciliation → verdict now FULLY.)

---

## 3. Marketplace Readiness Checklist

> **Important:** The actual listing requirements for the Ant Media
> marketplace at antmedia.io are unknown. No operator contact with the
> Ant Media marketplace team has been initiated as of this draft. Rows
> marked **NEEDS-OPERATOR-CONTACT** cannot be assessed without that
> contact. Generic requirements (working product, documentation, support
> channel, licensing) are assessed against what is known.

| # | Requirement | Status | Notes |
|---|-------------|--------|-------|
| 1 | Working product against current AMS release | PASS | 46/50 scenario scripts PASS against AMS 3.0.3 Enterprise; 0 FAIL |
| 2 | Core features functional end-to-end | PASS | Stream lifecycle, viewer counts, alerting, probes, QoE beacon all validated live |
| 3 | No P0 severity open bugs | PASS | BUG-002 FIXED S23/D-085 (VoD REST poll, live-validated TC-REC-01); BUG-008 FIXED S24/D-086 (Group B from/to via flagHistoryBridge + anomaly_flag_events); no P0-roadmap bugs remain open (BUG-001 is low/no-user-impact) |
| 4 | Integration documentation (AMS-side setup) | PARTIAL | `docs/AMS-INTEGRATION.md` exists; `docs/beacon-sdk.md` authored S19/D-081 (452 lines, DG-07 DONE); `docs/AMS-INTEGRATION.md` §4.5 expanded (DG-04 DONE — webhook downstream impact, workarounds, D-V2-1 future path); §1.1 expanded (DG-11 DONE — implicit RTMP broadcast deletion admonition). Remaining open DGs: DG-01 (HLS CDN viewer count), DG-02 (RTMP -1 FAQ), DG-03 (FPS=0 Known Limitation), DG-05 (standalone CPU/mem blank §3.7), DG-06 (egress estimate), DG-08 (per-app CIDR prereq), DG-09 (lockout strategy), DG-10 (HLS flat URL form), DG-12 (applications/info 405), DG-13 (app reset detection), DG-14 (versionType "Enterprise Edition"), DG-15 (Kafka guide), DG-16 (speed_read_kbps), DG-17 (GeoLite2), DG-18 (RTMP packet loss semantics) |
| 5 | API documentation / OpenAPI spec | PASS | OpenAPI spec exists; 51/52 operations response-body conformant; BUG-004/005/006/007/010 FIXED S20–S22; BUG-008 FIXED S24/D-086 (from/to probed); remaining parameter known-violations: BUG-009 ?tenant ×2 on GET /live (F6 backlog); 2 pinned in conformance registry; conformance registry: 37 probes / 2 known-violations / 47 exempt |
| 6 | Self-hosted deployment guide | PASS | `deploy/` directory contains Docker Compose stack, `Caddyfile.prod`, and environment variable documentation |
| 7 | Support channel defined | NEEDS-OPERATOR-CONTACT | No support SLA or support channel (email / forum / GitHub issues) has been publicly defined for Pulse v0.3.0 |
| 8 | Licensing clearly stated | NEEDS-OPERATOR-CONTACT | Pulse uses a license-key model (PULSE\_LICENSE\_KEY); the public licensing terms (free/pro/enterprise tiers, self-hosted redistribution rights) are not yet published |
| 9 | Marketplace revenue-share terms agreed | NEEDS-OPERATOR-CONTACT | The PRD cites 20–30% revenue share; this figure is **unverified** — it appears only in the PRD as a target and has not been negotiated or confirmed with Ant Media |
| 10 | Listing category, screenshots, and description copy | NEEDS-OPERATOR-CONTACT | `docs/marketplace/listing-draft.md` authored D-089 S27 (DRAFT-INTERNAL: title, tagline, short description, 6 feature bullets, tier/pricing table, demand evidence, trial-key paragraph); `docs/marketplace/screenshot-list.md` authored D-089 S27 (ordered 6-shot plan; PNG export is pending manual step). Listing submission still requires operator contact with Ant Media marketplace team. |
| 11 | Co-marketing / blog post process | NEEDS-OPERATOR-CONTACT | Operator must initiate contact with the Ant Media developer-relations or marketplace team |
| 12 | Semantic versioning and release artifacts | PARTIAL | v0.2.0 and v0.3.0 release tags published (cosign-signed multi-arch Docker images, SBOM + provenance, CI-gated tag pipeline; `ghcr.io/aytekxr/ams-pulse`). Residual gaps: GHCR image registry visibility is private pending operator decision O7; no binary tarball releases (Docker image is the only distribution artifact). Row stays PARTIAL until GHCR is made public. |
| 13 | Security: token authentication on all API routes | PASS | Bearer token (`plt_…`) required on all Pulse API routes; confirmed in TC-H-01/H-02 (S17) |
| 14 | No hard-coded secrets in committed code | PASS | `deploy/.env` is gitignored; secrets are not in committed files |
| 15 | Privacy: viewer IP handling | PASS | Viewer IPs are SHA-256 hashed (`normalize.go:281`); no raw IP stored in ClickHouse |
| 16 | AMS version compatibility disclosure | PASS | `docs/compatibility.md` authored D-089 S27: live-validated AMS 3.0.3 Enterprise (46/50 PASS); mock-profile-only rows for 2.10.0/2.14.x/3.0.2 (real containers 404 on Docker Hub — stated honestly); per-version wire-format differences (v2.10 `speed` vs v2.14+ `bitrate`; `webRTCViewerCount` added 2.14.x; `currentFPS` absent in AMS 3.0.3 REST); AMS 3.x specifics (per-app REST paths, no HMAC webhook signing, no CPU/mem via REST standalone); recommendation: AMS 3.x, others mock-compat only. |
| 17 | Known limitations documented | PASS | `docs/known-limitations.md` authored D-089 S27 (22 items as of D-091 S29), operator-facing tone (what it means for you + workaround/roadmap), priority-sorted, all claims traceable to primary evidence (DG series, TC IDs, AV triage). Supersedes internal-only `docs/assessment/documentation-gaps.md` for operator-facing disclosure. |

**Demand evidence and positioning notes (S25/D-087):**
- **ant-media/Ant-Media-Server#3122** (Prometheus exporter requested 2021, closed 2023 unbuilt; community workaround via `json_exporter` with a moved blog and lost dashboard) — Pulse's `/metrics` endpoint ships this natively, addressing a long-standing unmet community demand.
- **ant-media/Ant-Media-Server#7926** (open 2026-07-06: AMS freezes after ~24 h under high RTMP load; Java alive, OS metrics normal, HLS/API dead) — Pulse's S25 three-rung detection ladder directly addresses this failure class: latency-creep anomaly flag (`ams_api_latency_ms`) → `node_degraded` alert (~15 s) → `node_down` on freeze (BUG-011 fix, D-087).

---

## 4. Missing Opportunities

The following AMS capabilities are not yet consumed by Pulse. Each entry
is verified against `docs/assessment/capability-map.md` before inclusion.

### 4.1 Kafka-sourced CPU / Memory / Disk for Standalone Deployments

**Capability-map reference:** §4 ("Kafka alternative: `ams-instance-stats`
Kafka topic carries CPU/memory fields that REST `system-status` omits.
`PULSE_KAFKA_BROKERS` env var activates the consumer
(`server/internal/collector/kafka/`). Coverage of Kafka path: UNKNOWN.")
and §5 ("PARTIAL — os/jvm shown; resource gauges unavailable without Kafka
or cluster mode").

AMS 3.x does not expose CPU, memory, or disk metrics via the REST
`/system-status` endpoint (AV-06 confirmed). These metrics are available on
the `ams-instance-stats` Kafka topic. Pulse has a Kafka consumer at
`server/internal/collector/kafka/` gated on `PULSE_KAFKA_BROKERS`, but no
broker is deployed on the test VPS (AV-15 BLOCKED, operator Kafka decision
pending).

Consequence: for standalone AMS — which is the most common self-hosted
deployment profile — the Fleet health page shows no resource gauges. Anomaly
detection cannot baseline CPU/mem for standalone nodes. This is the most
impactful gap for a "fleet view" product positioning.

### 4.2 SRT-Specific Protocol-Level Packet Loss

**Capability-map reference:** §4 ("SRT and WHIP ingest use the same
BroadcastDTO so these metrics apply, but SRT-specific packet loss (at-protocol
level before AMS receives it) is not separately instrumented.")

AMS collects SRT socket-level statistics (including ARQ retransmissions and
pre-application packet loss) that differ from the BroadcastDTO
`packetLostRatio` field. Pulse consumes only BroadcastDTO, which reflects
what AMS received after SRT error-correction. The gap is invisible to Pulse
operators using SRT ingest, who may see `packet_loss_pct = 0` even when the
SRT link has meaningful transport-layer loss before AMS's ARQ fixes it.

### 4.3 Object Detection and AI Event Integration

**Capability-map reference:** Not mapped in `capability-map.md`. AMS
Enterprise Edition includes AI-powered features (object detection, face
detection) that emit metadata events. These events are not included in the
capability map and were not validated in this program. This represents an
uninvestigated opportunity; the gap cannot be precisely characterized
without further AMS API exploration.

### 4.4 Scheduled-Stream Pre-Event Alerting

**Capability-map reference:** Not mapped in `capability-map.md`. AMS allows
broadcast items to be scheduled in advance. Pulse does not consume the AMS
schedule endpoint and therefore cannot alert operators when a scheduled
stream has not started within N minutes of its expected start time. This
is a significant monitoring gap for live-event operations (concerts,
webinars, sports) where a missed start is a high-impact incident.

### 4.5 WHIP / WHEP Viewer Counts

**Capability-map reference:** §4 ("SRT and WHIP ingest use the same
BroadcastDTO"). WHIP (WebRTC HTTP Ingest Protocol) publisher counts are
visible as WebRTC publishers in the existing pipeline. WHEP (WebRTC HTTP
Egress Protocol) viewer counts, however, are not separately surfaced in
the BroadcastDTO viewer fields in AMS 3.0.3. Whether AMS 3.0.3 exposes
WHEP viewer counts via a separate endpoint is an open question (see §6).
If accessible, WHEP viewer tracking would complete the protocol coverage
matrix alongside HLS, RTMP, WebRTC, and DASH.

---

## 5. Prioritized Roadmap

Items are ranked by the combination of customer value, implementation
complexity, and marketplace impact.

| Priority | Item | Customer Value | Complexity | Marketplace Impact |
|----------|------|---------------|------------|-------------------|
| ~~**P0**~~ **DONE (S23/D-085)** | ~~**VoD recording\_gb via REST poll fallback**~~ — **FIXED 2026-07-12**: restpoller polls `/{app}/rest/v2/vods/list` every 12th tick; persistent seen-set dedup keyed on the stable AMS `vodId` (`vod_poll_state` meta migration 0003 — the live probe confirmed `vodId` exists, upgrading the design note's HWM to its own safer seen-set option); new `mv_recording_1d` ClickHouse MV (migration 0009) rolls recording\_size into `rollup_usage_1d`; live-validated TC-REC-01 (recording\_gb reconciles within 0.02% of AMS ground truth) | High — was structurally broken; now accounted | **Medium** — landed exactly as the corrected S20/D-082 design predicted (two additive migrations). Design note: `bugs/BUG-002-design-note-vod-rest-poll.md` | High — billing/SLA report use case now credible |
| **P0** | **Unsigned-webhook ingest mode** (D-V2-1; operator decision pending) — accept lifecycle events without HMAC from sources on a `PULSE_WEBHOOK_ALLOW_UNSIGNED_SOURCES` CIDR allowlist (ROADMAP-V2 §2.6 design); operator assumes network-layer trust risk | High — the webhook path is the intended real-time event channel; it is entirely unused in prod today; REST poll covers latency but misses vodReady | Medium — new config flag, webhook handler changes, security documentation | High — enables the full F1 + F6 intended design |
| ~~**P0**~~ **DONE (S20/D-082)** | ~~**Fix BUG-004: parse `from`/`to` in `/api/v1/qoe/ingest` handler**~~ — **FIXED 2026-07-12**: handler now honors `from`/`to`/`app`/`stream`/`node`; contract unchanged. Residual `interval` param carved out as **BUG-005** (same declared-but-ignored class). | Medium — operators using the historical ingest view after a bitrate incident see incorrect averaged data | Low — targeted handler fix at one route; no schema change | Medium — OpenAPI contract violation is a quality signal for marketplace reviewers |
| **P1** | **Standalone CPU / mem / disk via Kafka** (AV-15; requires operator to deploy Kafka broker or configure `PULSE_KAFKA_BROKERS`) | High — the most common deployment profile (standalone AMS on a single VPS) currently shows no resource gauges on the Fleet page | High — requires Kafka broker deployment by operator and validation of the existing `server/internal/collector/kafka/` consumer against `ams-instance-stats` topic schema | High — "fleet health" is a key positioning claim; empty gauges undermine it |
| **P1** | **error\_rate + rebuffer\_ratio anomaly signals** — add `error_rate` and `rebuffer_ratio` (from beacon rollup) to the Welford anomaly evaluator; currently confirmed absent (TC-AN-05 PASS) | Medium — the PRD explicitly lists "errors and rebuffering" as anomaly targets; the absence is a documented gap (F9 PARTIALLY) | High — S25/D-087 SPARSITY GATE: prod `beacon_events` = 2 rows / 1 stream, `realams` = 0 rows; all-zero baselines ⇒ epsilon-floor makes first real rebuffer event an instant false alarm; `rollup_qoe_1h` buckets accumulate within the hour ⇒ non-independent Welford samples; windowing redesign (minute-granularity or tick-deltas) needed before this is safe to enable | High — viewer QoE anomaly detection is a differentiator; completing it makes F9 FULLY |
| **P1** | **SDK integration guide** (DG-07; MVP+1 documentation deliverable) — step-by-step operator guide for `ams-webrtc`, `hls.js`, and `video.js` adapters covering adapter selection, token provisioning, and ingest URL | Medium — the SDK runtime exists but operators cannot self-serve integration without docs | Low — documentation only; no code changes | Medium — marketplace listings expect integration documentation |
| **P1 — BLOCKED: AMS EE licence suspended (2026-07-13)** | **SRT loss validation against live SRT ingest** — `qa/realams/scenarios/TC-I-05-SRT-packet-loss.sh` committed and runnable; DG-18 variant note added to `docs/AMS-INTEGRATION.md` §1.1 (S29/D-091). **BLOCKED:** AMS EE licence suspension gates SRT ingest at SRTAdaptor level. Live evidence 2026-07-13: SRTAdaptor logged "License is suspended. Not accepting the stream" (exit 77; evidence `qa/realams/evidence/S29-TC-I-05-SRT-20260713T215932Z/`). RTMP ingest unaffected. Closure = re-run TC-I-05-SRT post-licence-renewal to record publishType and confirm post-ARQ semantics. DG-18 variance note authored; outstanding observation = first live SRT run. | Medium — SRT ingest operators may misinterpret `packet_loss_pct = 0` when transport loss exists | Low — test-only; possible doc clarification; no code change unless SRT stats API differs | Low — correctness documentation |
| **P1** | **Remote-viewer WebRTC QoE parity** — repeat TC-V-07 / TC-V-08 with a geographically remote WebRTC viewer to confirm ×1000 RTT conversion at non-zero values; same-host loopback returns all-zero AMS stats | Medium — the ×1000 unit conversion is code-verified at `normalize.go:185` but exercised only at 0 values; a flip to ×0.001 would produce RTT values in the µs range silently | Low — test-only; requires access to a second host | Medium — WebRTC QoE is a differentiating metric; correct units are table stakes |
| **P2** | **Scheduled-stream pre-event alerting** — consume AMS schedule endpoint; emit an alert when a scheduled stream has not started within configurable N minutes of its scheduled time | Medium — high-impact for live-event operators (sports, concerts, webinars) | Medium — new AMS API surface to poll; new alert rule type | Medium — live-event monitoring differentiates from generic AMS dashboards |
| **P2** | **GeoLite2 mmdb bundling or setup guide** — geo country analytics are non-functional without the MaxMind database; either bundle under OFL terms or provide a one-command setup step in the deployment guide | Medium — geo analytics is listed as an F2 sub-requirement; empty-country reduces value of the audience analytics module | Low — packaging or documentation change | Low — operators expect geo to work at install time |
| ~~**P2**~~ **DONE (S20/D-082)** | ~~**Fix BUG-003: probe scheduler near-duplicate rows**~~ — **FIXED 2026-07-12**: spawnProbe now returns early on unchanged probe config (filed root-cause hypothesis was wrong — not immediate-on-create goroutine + periodic ticker; actual: 60 s refresh loop unconditionally respawned ALL probes, resetting probe phase on every tick; fix: probeEntry stores domain.ProbeConfig, whole-struct equality check before respawn). | Low | Low | Low |
| **P2** | **RTMP pull viewer count via `/{app}/connections`** | Low — `rtmpViewerCount` inline is 0 for pull viewers in BroadcastDTO; dedicated connections endpoint not polled | Low — additional REST call per stream | Low — edge case for operators using RTMP pull distribution |

---

## 6. Open Questions for the Ant Media Team

These items require input from the Ant Media engineering or product team
to resolve. They are open as of S19 and block certain roadmap or
documentation actions.

### Q1 — Webhook HMAC Signing Plans

**Context:** AMS 3.0.3 cannot HMAC-sign lifecycle hooks (O3 decision,
`decisions.md`; AV-08 confirmed). Pulse's webhook listener is fail-closed
and rejects all unsigned deliveries. The `vodReady` webhook is the only
ingestion path for VoD recording data via webhooks (BUG-002 — since fixed
S23/D-085 by a VoD REST poll fallback, so recording accounting no longer
depends on this answer; the question stands because a signed webhook would
cut recording-visibility latency from ≤60 s to near-real-time). REST polling
covers stream lifecycle within the 10 s latency budget.

**Question:** Does AMS have a roadmap item to add HMAC webhook signing
(a `SharedSecret` field on the outbound hook configuration)? If so, what
is the target release? If not, is there an alternative server-side event
channel (e.g., signed JWT, mTLS) that Pulse could consume for recording
events?

**Impact on roadmap:** The answer determines whether the P0 "unsigned-webhook
ingest mode" item (D-V2-1) is a permanent feature or a temporary workaround.

### Q2 — hlsViewerCount Sliding-Window Semantics and the ~9x Inflation Factor

**Context:** In TC-V-06 (S18), 5 real HLS players produced an AMS
`hlsViewerCount` of 45 (~9x the real viewer count). The count did not drop
below 4 within 90 s after stopping 3 of 5 viewers. The AMS HLS viewer count
is a sliding segment-request window (DG-01). Pulse faithfully mirrors this
value; it does not attempt to session-de-duplicate it.

**Questions:**
- What is the intended semantics of `hlsViewerCount`? Is the 9x factor
  expected behavior for the configured segment duration and playlist depth?
- Is there a session-accurate HLS viewer count available via another AMS
  endpoint (e.g., a CDN integration or session-tracking API)?
- What time window is used for the segment-request expiry? The observed
  behavior (count still 38 at 90 s after stopping 5 viewers) implies the
  window is longer than the session duration.

**Impact:** Pulse documentation must accurately describe the semantics to
operators. If a session-accurate count exists, Pulse should expose it.

### Q3 — WHEP Viewer Count Exposure

**Context:** Pulse tracks WebRTC viewer counts via the `webRTCViewerCount`
inline field in BroadcastDTO. WHEP (WebRTC HTTP Egress Protocol) is a
distinct protocol from the AMS native WebRTC publish/subscribe. In AMS 3.0.3,
WHEP viewer counts are not separately surfaced in the inline BroadcastDTO
fields observed in this validation.

**Question:** Does AMS 3.0.3 (or a recent release) expose WHEP viewer counts
via a REST endpoint? If so, which field or endpoint should Pulse consume?

### Q4 — Analytics Log FPS Field

**Context:** `currentFPS` is absent from the AMS 3.0.3 BroadcastDTO REST
response (confirmed at `client.go:97` comment; AV-04). Pulse stores
`fps = 0` for all REST-polled streams. FPS data is reportedly available via
the AMS analytics log on the Kafka `ams-instance-stats` topic.

**Questions:**
- What is the field name and unit for FPS in the `ams-instance-stats`
  Kafka topic?
- Is `currentFPS` intentionally absent from the REST BroadcastDTO in
  AMS 3.x, or is this a regression from AMS 2.x?
- Is there a plan to restore the FPS field in the REST response?

**Impact:** Without a confirmed Kafka field name, Pulse's Kafka-sourced FPS
implementation cannot be fully specified.

### Q5 — Marketplace Listing Requirements and Revenue-Share Terms

**Context:** The PRD (`docs/prd-report.md` §7) cites a 20–30% marketplace
revenue-share figure. This figure is **unverified** — it appears only in the
PRD as a target number and has not been confirmed with Ant Media. No contact
with the Ant Media marketplace or developer-relations team has been
initiated as of this draft.

**Questions (all require operator to initiate contact):**
- What are the current listing requirements for the Ant Media marketplace
  (categories, screenshot specs, minimum documentation level, support
  requirements)?
- What is the current revenue-share percentage for marketplace listings?
- Is there a co-marketing blog post or developer showcase process that
  Pulse can participate in at listing time?
- What AMS versions must a marketplace product support to be listed?

---

## Appendix A — Bugs Found During This Validation Program

Eleven bugs were found and filed by this program. The methodology (direct
AMS REST cross-check, not UI-only assertions) produced real defects, not
just scenario confirmations. BUG-001/002/003/004/005/006/007/008/010/011 have been fixed; BUG-009 is partially fixed (the `?tenant` pair awaits a multi-tenancy data model — a product decision, not a defect). No open bugs remain.

| ID | Severity | Title | Features Affected | Status |
|----|----------|-------|-------------------|--------|
| BUG-001 | Low | `amsclient.BroadcastStatistics()` is dead code — defined, tested, never called at runtime | F1 (no user impact; inline counts correct) | **FIXED S26/D-088**: dead code deleted (method + DTO + test + fixture); the live pipeline was never affected (inline BroadcastDTO counts) |
| BUG-002 | **High** | `recording_gb` always 0 — VoD REST never polled; vodReady webhook blocked on AMS 3.0.3 (cannot HMAC-sign hooks) | F6 (recording/billing use case was structurally broken) | **FIXED S23/D-085**: VoD REST poll + persistent `vodId` seen-set + `mv_recording_1d` MV (migrations 0003 meta / 0009 ClickHouse); TDD (8 poller tests, MV integration test, 5 mutation proofs RED); live-validated TC-REC-01 3/3 vs real AMS (0.02% reconciliation) |
| BUG-003 | Medium | Probe scheduler unconditionally respawned ALL probes on every 60 s refresh tick, resetting probe phase and producing duplicate result rows every 60 s | F10 (probe result timeseries had N+1/N+2 rows per expected-N window; filed root-cause hypothesis was wrong) | **FIXED S20/D-082** (PR #32); 4 regression tests; prober coverage 72.6%→74.3% |
| BUG-004 | Medium | `GET /api/v1/qoe/ingest` declares `from`/`to` parameters but handler silently ignored them; production dashboard served all-time era-mixed buckets on every page load | F4 (windowed ingest health queries); F8 (OpenAPI contract violation) | **FIXED S20/D-082** (PR #32); 13 TDD subtests; api coverage 76.9%→78.0% |
| BUG-005 | Medium | `GET /api/v1/qoe/ingest` `interval` param declared but ignored; callers receive 60 s buckets regardless of hour/day request | F4/F8 | **FIXED S21/D-083** (PR #33); 5 TDD subtests; absent interval intentionally maps to 0 to preserve 60 s default (F4 "15 s visibility" criterion) |
| BUG-006 | Medium | Pagination params `limit` + `cursor` declared on 8 list endpoints but store-layer methods had no pagination args; all results were unbounded | F5/F8/F10 + admin endpoints | **FIXED S22/D-084** (PR #34); keyset cursors on all 8 store methods; 2 panics caught and fixed (slice OOB, negative limit) |
| BUG-007 | Low–Medium | `cursor` param dropped in `GET /alerts/history` and `GET /probes/{probeId}/results`; callers could not page past page 1 | F5 (alert history); F10 (probe results) | **FIXED S22/D-084** (PR #34); real probes (not exempts) asserting page 1 ≠ page 2 |
| BUG-008 | High | `GET /anomalies` drops all 6 declared filter params; `from`/`to` are architecturally unfixable without a persistent flag-event store | F9 (anomaly detection) | **FIXED S22+S24 (D-084 Group A, D-086 Group B)**: `app`/`stream`/`limit`/`cursor` (Group A) fixed handler-side S22; `from`/`to` (Group B) fixed S24 via `anomaly_flag_events` ClickHouse table (migration 0010) + ADR-0009 + `flagHistoryBridge` wired in serve.go |
| BUG-009 | Medium | `GET /live/overview` + `GET /live/streams`: `tenant` param passed by handler but silently dropped in query layer; `cursor` in LiveStreams was stubbed | F6/F1 | **PARTIALLY FIXED S22/D-084** (PR #34): LiveStreams `cursor` decode + required stability sort added; `tenant` ×2 remain known-violation → `domain.LiveSnapshot` has no tenant assignment (F6 multi-tenancy backlog) |
| BUG-010 | Low | `GET /analytics/audience` reads `?format=csv` but the parameter was not declared in the OpenAPI spec (reverse-direction gap: implementation ahead of contract) | F2/F8 | **FIXED S22/D-084** (PR #34): `format` enum `[json,csv]` + `text/csv` 200 response declared; `gen:api` regenerated; `minSpecParams` 85→86 |
| BUG-011 | **High** | `EvictStaleNodes` implemented but never wired in serve.go — `node_down` structurally unable to fire in production; compounded by failure-streak path refreshing LastSeenAt, keeping frozen nodes perpetually "fresh" | F7 (node up/down alerts; node_down for frozen AMS nodes per ant-media#7926 failure class) | **FIXED S25/D-087**: `wireNodeEviction` goroutine added to serve.go; failure-streak path updated to in-place ConsecAPIErrors update only (no LastSeenAt refresh); RED→GREEN unit tests (TestBUG011\_NodeEviction\_Wired, TestAggregator\_FailureStreak\_LastSeenAtFrozen, TestAggregator\_FailureStreak\_EvictStillWorks); node\_down path is structurally correct and unit-proven; live node-offline scenario not run |

Bug documents: `docs/assessment/bugs/BUG-001-broadcast-statistics-dead-code.md`,
`BUG-002-recording-gb-zero-webhook-blocked.md`,
`BUG-002-design-note-vod-rest-poll.md`,
`BUG-003-probe-scheduler-duplicate-results.md`,
`BUG-004-qoe-ingest-ignores-from-to.md` (BUG-005 documented as residual section therein),
`BUG-006-pagination-dead-params.md`,
`BUG-007-cursor-missing-partial-pagination.md`,
`BUG-008-anomalies-filter-params-silently-dropped.md`,
`BUG-008-triage-s22.md`,
`BUG-009-live-tenant-cursor-dropped-in-query-layer.md`,
`BUG-010-audience-format-param-undeclared.md`,
`BUG-011-evictstalenodes-never-wired.md`.

---

## Appendix B — AMS 3.0.3-Specific Semantics (Not Pulse Bugs)

These are AMS platform behaviors that Pulse correctly mirrors and that must
be disclosed to operators. Documentation gaps are tracked as DG-01 through
DG-18 in `docs/assessment/documentation-gaps.md`.

| Behavior | Evidence | Doc Gap |
|----------|----------|---------|
| hlsViewerCount is a sliding segment-request window (~9x inflation relative to real session count; expiry lag >90 s) | TC-V-06 (S18); peak AMS count = 45 with 5 real viewers; residual count = 38 at 90 s after 3 of 5 viewers stopped | DG-01 |
| currentFPS is absent from AMS 3.x REST BroadcastDTO; Pulse fps = 0 for all REST-polled streams | AV-04; client.go:97 comment | DG-03 |
| AMS 3.0.3 cannot HMAC-sign lifecycle hooks; Pulse webhook path entirely unused in prod | AV-08; O3 decision | DG-04 |
| Standalone AMS exposes no CPU/mem/disk via REST system-status; Pulse emits null (not 0) for absent fields | AV-06; TC-H-06 | DG-05 |
| egress\_gb is a bitrate×watch-time estimate, not a measured delivered-bytes value | AV-09; TC-A-08 | DG-06 |
| RTMP/TCP ingest masks transport-layer packet loss; `packetLostRatio = 0` for RTMP regardless of wire loss | TC-I-05 | DG-18 |
| Implicit RTMP broadcasts (no REST pre-create) are deleted on stop (GET 404), not transitioned to `finished` or `terminated_unexpectedly` | TC-F-02 | DG-11 |
| GET /rest/v2/applications/info returns HTTP 405 on AMS 3.0.3 build 20260504\_1443 | S17 corrections; S17-applications-info.json | — |

---

## Appendix C — Evidence Base and Reproducibility

All scenario evidence directories are local to the validation VPS and are
gitignored. They are cited throughout the PRD validation matrix and this
document by scenario ID and timestamp prefix (e.g.
`S17-TC-WH-02-20260711T120043Z`). Each directory contains:

- `verdict.txt` — PASS / SKIP / FAIL with reasoning
- Timestamped JSON snapshots from the AMS REST endpoint
- Timestamped JSON snapshots from the Pulse API endpoint
- `timeline.txt` (for latency-critical scenarios)
- `checks.txt` (for multi-assertion scenarios)

The harness is at `qa/realams/`. All 50 scenario scripts pass `bash -n`
(syntax clean). The test runner is `make validate-realams-p0` (P0) and
`make validate-realams-p1` (P1).

CI results (mock-AMS) cited throughout this document are reproducible
with `make test` at any commit on the main branch. The AMS trial license
expires 2026-07-12T12:09Z; re-running against the same live AMS instance
after that date requires operator license renewal or replacement.

---

*Document produced at S19 close (2026-07-11) by ORCH-00 authoring agent
(Phase 8, WO-B). See `docs/assessment/session-plan.md` for the full
eight-phase validation program structure and dependency graph.*
