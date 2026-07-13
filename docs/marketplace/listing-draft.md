<!--
  DRAFT — INTERNAL. External use gated on operator review of
  docs/assessment/final-assessment.md (D-081).
-->

> **DRAFT — INTERNAL. External use gated on operator review of
> `docs/assessment/final-assessment.md` (D-081).**
>
> This document has not been reviewed by the Ant Media marketplace team.
> Rows marked NEEDS-OPERATOR are blocked on operator action before the
> listing can be submitted. Pricing rows are PROPOSED (from `docs/prd-report.md`
> §7.11) and have not been confirmed with Ant Media. Revenue-share rows are
> UNVERIFIED (PRD target only; not yet negotiated).

---

# Ant Media Marketplace — Listing Draft

**Product:** Pulse: Analytics & QoE Monitoring for AMS  
**Prepared:** S27 / D-089 (2026-07-13)  
**Contact for submission:** NEEDS-OPERATOR (see §7 below)

---

## 1. Listing title

> **Pulse — Analytics & QoE Monitoring for AMS**

Character count: 43 (limit 60 — within budget).

---

## 2. Tagline

> Self-hosted streaming analytics, alerting, and viewer QoE for Ant Media Server operators.

---

## 3. Short description (≤250 characters)

> Pulse installs next to AMS and delivers real-time stream dashboards, viewer QoE analytics,
> fleet health monitoring, alerting (Slack/email/PagerDuty), usage reports, and anomaly
> detection — all on your own infrastructure.

Character count: 202 (limit 250 — within budget).

---

## 4. Feature bullets (5–6)

1. **Live ops dashboard** — new stream visible ≤4 s; viewer counts per protocol (HLS,
   WebRTC, RTMP, DASH); fleet node health; auto-discovers all AMS apps and cluster nodes.
   (Evidence: TC-WH-02 S17 — 4 s publish→Pulse; 46/50 scenario scripts PASS on AMS 3.0.3
   Enterprise live deployment.)

2. **Player QoE beacon SDK** — 3.52 KB gzip (budget 15 KB); MIT-licensed; adapters for
   AMS WebRTC, hls.js, and video.js; reports startup time, rebuffering, errors, bitrate
   switches, and watch time from the viewer's browser. Integration guide:
   `docs/beacon-sdk.md` (452 lines, authored S19/D-081).

3. **Alerting on any metric** — stream offline, ingest bitrate floor, viewer count drop,
   node-degraded rung, node-down freeze detection; channels: email (Free+), Slack/Telegram
   (Pro+), PagerDuty/webhook (Business+); 201 ms detection-to-notification wall-clock.
   (Evidence: TestEvaluator\_DetectAndNotify\_WallClockBudget CI; TC-H-04/05 S18 live.)
   **Demand signal:** ant-media/Ant-Media-Server#7926 (open 2026-07-06: AMS freeze under high
   RTMP load). Pulse's S25 three-rung ladder directly addresses this failure class
   (`final-assessment.md` §3 demand note).

4. **Prometheus endpoint + full REST API** — `/metrics` in Prometheus exposition format;
   32-path OpenAPI 3.1 spec; 51/52 operations response-body conformant; scrape token uses
   constant-time compare. No additional Grafana pipeline needed.
   **Demand signal:** ant-media/Ant-Media-Server#3122 (closed 2023 without implementation —
   Prometheus exporter was a long-standing unmet community request; Pulse ships this natively).

5. **Usage and billing reports** — viewer-minutes, peak concurrency (true windowed max),
   VoD recording storage (REST poll, BUG-002 FIXED S23/D-085), egress estimate; CSV/PDF;
   ±1% reconciliation (0.0000% drift at n=10,000 in CI). Business+ tier.

6. **Anomaly detection** — Welford statistical baseline on viewer counts and bitrate;
   0.259 false alarms/node-week at default σ=4.0 (target <1); epsilon-floor prevents
   constant-zero baseline false silencing. Enterprise tier.

---

## 5. Tier and pricing table

> **PROPOSED** — prices from `docs/prd-report.md` §7.11. Not yet published or confirmed
> with the Ant Media marketplace team (checklist row 8 NEEDS-OPERATOR-CONTACT).
> **Revenue-share** (20–30%) is UNVERIFIED — appears only in the PRD as a target figure
> and has not been negotiated with Ant Media (checklist row 9, final-assessment.md §6 Q5).

The entitlements below are drawn from `server/internal/license/license.go:90–129`
(the authoritative runtime implementation). Where the PRD §7.11 and license.go diverge,
the code governs and the divergence is flagged.

| Tier | Price (PROPOSED) | Max Nodes | Retention | Alert Channels | Data API | White-label | Notes |
|------|-----------------|-----------|-----------|----------------|----------|-------------|-------|
| **Free** | $0/month | 1 | 7 days | Email only | No | No | `freeTierEntitlements` in `server/internal/license/license.go` (§7.11 reference in comment) |
| **Pro** | $99/month | 10 | 90 days | Email, Slack, Telegram | Yes | No | `proTierEntitlements` in license.go; **NOTE:** PRD §7.11 says "1 to 2 nodes" but license.go enforces MaxNodes=10 — code governs; operator should reconcile before publishing |
| **Business** | $299/month | 5 | 13 months | Email, Slack, Telegram, PagerDuty, Webhook | Yes | No | `businessTierEntitlements` in license.go; includes billing reports, multi-tenant, Prometheus |
| **Enterprise** | from $799/month (PROPOSED) | Unlimited | Unlimited | All channels | Yes | Yes | `enterpriseTierEntitlements` in license.go; includes anomaly detection (F9), SSO, white-label PDF |

**Feature gates cross-check** (license.go:90–129 vs README.md feature table):

| Feature | Minimum tier | Code gate |
|---------|-------------|-----------|
| QoE beacon events (ingest) | Pro+ | `DataAPI: true` in `proTierEntitlements`; beacon round-trip confirmed TC-A-05/06 |
| CSV export / usage reports | Business+ | `businessTierEntitlements.DataAPI = true`; billing reports gated in report generator |
| Anomaly detection (F9) | Enterprise | `enterpriseTierEntitlements`; anomaly evaluator checks Enterprise tier flag |
| White-label PDF reports | Enterprise | `WhiteLabel: true` only in `enterpriseTierEntitlements` (license.go) |
| PagerDuty / Webhook channels | Business+ | `businessTierEntitlements.Channels` includes "pagerduty" and "webhook" |

---

## 6. What's included per tier

### Free
- Live operations dashboard (stream list, viewer counts, fleet node health)
- Stream start/stop alerting (email)
- 7-day data retention
- Docker Compose single-node install
- Community support (GitHub Issues)

### Pro ($99/month — PROPOSED)
- Everything in Free, plus:
- Player QoE beacon SDK integration (AMS WebRTC, hls.js, video.js adapters)
- Historical QoE analytics (startup p50, rebuffer ratio, error rate)
- Synthetic viewer probes (HLS, WebRTC, RTMP, DASH — `docs/runbooks/probes.md`)
- Slack and Telegram alert channels
- CSV data export
- 90-day data retention
- Up to 10 AMS nodes (code: license.go:102; reconcile with PRD "1 to 2 nodes")

### Business ($299/month — PROPOSED)
- Everything in Pro, plus:
- Usage and billing reports (viewer-minutes, egress estimate, VoD recording storage)
- Multi-tenant billing (stream-name pattern or metadata tag)
- Prometheus `/metrics` endpoint
- PagerDuty and webhook alert channels
- 13-month data retention
- Priority email support
- Up to 5 AMS nodes (code: license.go:112)

### Enterprise (from $799/month — PROPOSED)
- Everything in Business, plus:
- Anomaly detection (F9: Welford baselines on viewers, bitrate, CPU/mem)
- White-label PDF reports
- SSO / OIDC (shipped Wave 3, D-070/D-074)
- Unlimited nodes and retention
- Air-gapped licensing (roadmap)
- SLA and onboarding support

---

## 7. Trial-key onboarding paragraph

> **ASSUMED — OPERATOR-DECISION-PENDING.** A 14-day Pro trial key is assumed as the
> standard onboarding path. Whether Pulse ships with a built-in trial or requires
> a manual trial-key issuance is an operator decision.

---

Pulse installs in under 15 minutes. After installing via Docker Compose:

1. Copy the admin token printed to stderr on first boot.
2. Open `http://localhost:8090` and log in.
3. Streams from your AMS instance appear within 10 seconds of publish (confirmed 4 s
   in live validation, TC-WH-02).
4. To activate your Pro or Business license: paste your license key in
   **Settings → License** and click Activate. Features unlock immediately — no restart required.

To request a 14-day Pro trial key (OPERATOR-DECISION-PENDING: trial key issuance
mechanism not yet decided), contact: [support channel — NEEDS-OPERATOR].

---

## 8. Support and licensing rows (NEEDS-OPERATOR-CONTACT)

| Row | Status | Action required |
|-----|--------|-----------------|
| Support channel / SLA | NEEDS-OPERATOR-CONTACT | Define and publish a support URL (email, GitHub Issues, or hosted forum). Checklist row 7 in `docs/assessment/final-assessment.md` §3. |
| Public licensing terms | NEEDS-OPERATOR-CONTACT | Publish human-readable licensing terms (PolyForm NC for self-hosted; commercial license for vendor use). Checklist row 8. |
| Revenue-share terms | UNVERIFIED | Negotiate with Ant Media marketplace team. PRD target: 20–30%. Checklist row 9. |
| Listing submission | NEEDS-OPERATOR-CONTACT | Initiate contact with Ant Media developer-relations or marketplace team. Checklist row 10. |
| AMS version support requirement | NEEDS-OPERATOR-CONTACT | Ask Ant Media what minimum AMS version a marketplace product must support. Q5 in `docs/assessment/final-assessment.md` §6. |

---

## 9. Demand evidence citations

These are included in the listing copy (bullet 3 and 4 above) and are on the public
GitHub issue tracker; they do not require operator clearance to cite:

- **ant-media/Ant-Media-Server#3122** (Prometheus exporter requested 2021, closed 2023
  without implementation; community workaround via `json_exporter` with a moved blog and
  lost dashboard). Pulse's `/metrics` endpoint ships this natively.
  Source: `docs/assessment/final-assessment.md` §3 demand note (added S25/D-087).

- **ant-media/Ant-Media-Server#7926** (open 2026-07-06: AMS freezes after ~24 h under
  high RTMP load; Java alive, OS metrics normal, HLS/API dead). Pulse's S25 three-rung
  detection ladder — latency-creep anomaly flag (`ams_api_latency_ms`) → `node_degraded`
  alert (~15 s) → `node_down` on freeze — directly addresses this failure class (BUG-011
  FIXED S25/D-087).
  Source: `docs/assessment/final-assessment.md` §3 demand note (added S25/D-087).

---

*Produced at S27/D-089. Evidence sources: `docs/assessment/final-assessment.md` §3,
`docs/prd-report.md` §7.11, `server/internal/license/license.go:90–129`,
`docs/assessment/prd-validation-matrix.md`.*
