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
**Prepared:** S27 / D-089 (2026-07-13); last revised S96 / D-160 (2026-07-22)  
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

> Pulse installs next to AMS and delivers real-time stream dashboards, QoE beacon analytics,
> fleet health monitoring, alerting (email, Slack, Telegram, PagerDuty, webhook), scheduled
> PDF/CSV usage reports, and anomaly detection — self-hosted.

Character count: 240 (limit 250 — within budget).

<!-- INTERNAL POSITIONING NOTE — omit from all public copy.
AMS Management-panel-reborn (G-27, public-repo evidence 2026-07-22, commit c4a0235,
src/lib/api/client.ts MGMT_PREFIX='/rest/v2') ships live server-side analytics charts:
per-stream bitrate/viewer/speed history, per-app viewer sparklines, system-resource
and GPU trends, WebRTC client stats. It does NOT include: alerting or notification
channels, player-side QoE beacon, long-horizon retention / rollups, scheduled billing
reports, synthetic probes, or anomaly detection. Pulse's competitive positioning has
shifted from "AMS has no analytics at all" to "the new AMS panel charts live server
metrics; Pulse adds alerting (5 channels), viewer-side QoE beacon (3.52 KB), 13-month
historical rollups, scheduled PDF/CSV billing reports, synthetic probes (4 protocols),
and anomaly detection." Source: docs/compatibility.md §G-27 Public-repo evidence
subsection (2026-07-22). Confirm remaining unknowns at the developer meeting. -->

---

## 4. Feature bullets (5–6)

1. **Live ops dashboard** — new stream visible ≤4 s; viewer counts per protocol (HLS,
   WebRTC, RTMP, DASH); fleet node health; auto-discovers all AMS apps and cluster nodes.
   (Evidence: TC-WH-02 S17 — 4 s publish→Pulse; 46/50 scenario scripts PASS on AMS 3.0.3
   Enterprise live deployment.)

2. **Player QoE beacon SDK** — 3.52 KB gzip (budget 15 KB); MIT-licensed; adapters for
   AMS WebRTC, hls.js, and video.js; reports startup time, rebuffering, errors, bitrate
   switches, and watch time from the viewer's browser. Integration guide:
   `docs/beacon-sdk.md` (485 lines as of 2026-07-22, authored S19/D-081).

3. **Alerting on any metric — 5 channel types** — stream offline, ingest bitrate floor,
   viewer count drop, node-degraded rung, node-down freeze detection; 201 ms
   detection-to-notification wall-clock. Channels by tier: **email** (Free+),
   **email + Slack + Telegram** (Pro+), **email + Slack + Telegram + PagerDuty + webhook**
   (Business+, all 5); all 5 channels available on Enterprise.
   (Evidence: TestEvaluator\_DetectAndNotify\_WallClockBudget CI; TC-H-04/05 S18 live.)
   **Demand signal:** ant-media/Ant-Media-Server#7926 (open 2026-07-06: AMS freeze under high
   RTMP load). Pulse's S25 three-rung ladder directly addresses this failure class
   (`final-assessment.md` §3 demand note).

4. **Synthetic probes + full observability API** — active connectivity probes over HLS,
   WebRTC, RTMP, and DASH verify stream reachability from Pulse's own vantage (Pro+);
   13-month (396-day) retention window enables long-horizon trend and rollup analysis;
   Prometheus `/metrics` endpoint in standard exposition format; 42-path OpenAPI 3.1 spec
   (51/52 operations response-body conformant — **historical v0.3-era CI measurement;
   pending re-run against the current 59-operation spec**); scrape token uses constant-time compare.
   No additional Grafana pipeline needed.
   **Demand signal:** ant-media/Ant-Media-Server#3122 (closed 2023 without implementation —
   Prometheus exporter was a long-standing unmet community request; Pulse ships this natively).

5. **Usage and billing reports** — viewer-minutes, peak concurrency (true windowed max),
   VoD recording storage (REST poll, BUG-002 FIXED S23/D-085), egress estimate; on-demand
   CSV export and **scheduled PDF and CSV delivery** (via `/reports/schedules`);
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

The entitlements below are drawn from `server/internal/license/license.go:90–150`
(the authoritative runtime implementation). Where the PRD §7.11 and license.go diverge,
the code governs and the divergence is flagged.

| Tier | Price (PROPOSED) | Max Nodes | Max Streams | Retention | Alert Channels | Data API | White-label | Notes |
|------|-----------------|-----------|-------------|-----------|----------------|----------|-------------|-------|
| **Free** | $0/month | 1 | Unlimited | 7 days | Email only (1 channel) | No | No | `freeTierEntitlements` in `server/internal/license/license.go` (§7.11 reference in comment) |
| **Pro** | $99/month | 10 | Unlimited | 90 days | Email, Slack, Telegram (3 channels) | Yes | No | `proTierEntitlements` in license.go; **NOTE (PRD divergence):** PRD §7.11 says "1 to 2 nodes" but license.go enforces MaxNodes=10 — code governs; operator should reconcile before publishing. **NOTE (tier-order inversion):** Pro (MaxNodes=10) exceeds Business (MaxNodes=5) — a higher tier carries a lower node limit; this reversal is almost certainly unintentional and must be resolved before publishing |
| **Business** | $299/month | 5 | Unlimited | 396 days (13 months) | Email, Slack, Telegram, PagerDuty, Webhook (5 channels) | Yes | No | `businessTierEntitlements` in license.go; includes billing reports, multi-tenant, Prometheus, scheduled PDF/CSV |
| **Enterprise** | from $799/month (PROPOSED) | Unlimited | Unlimited | Unlimited | All 5 channels (email, Slack, Telegram, PagerDuty, webhook) | Yes | Yes | `enterpriseTierEntitlements` in license.go; includes anomaly detection (F9), SSO, white-label PDF |

**Feature gates cross-check** (license.go:90–150 vs README.md feature table):

| Feature | Minimum tier | Code gate |
|---------|-------------|-----------|
| QoE beacon events (ingest) | Pro+ | `CheckBeaconIngest()` in `license.go:405` (Pro/Business/Enterprise); beacon round-trip confirmed TC-A-05/06 |
| CSV export / usage reports | Business+ | `CheckReports()` in `license.go:394` (Business/Enterprise); gated in both export handler (`handleReportExport`) and report scheduler |
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
- On-demand CSV data export
- 90-day data retention
- Up to 10 AMS nodes (code: license.go:124; reconcile with PRD "1 to 2 nodes" AND the Pro>Business node-limit inversion)

### Business ($299/month — PROPOSED)
- Everything in Pro, plus:
- Usage and billing reports (viewer-minutes, egress estimate, VoD recording storage)
- On-demand CSV export **and scheduled PDF and CSV delivery** (via report schedules API)
- Multi-tenant billing (stream-name pattern or metadata tag)
- Prometheus `/metrics` endpoint
- PagerDuty and webhook alert channels (all 5 channel types)
- 396-day (13-month) data retention — enables long-horizon trend analysis and rollups
- Priority email support
- Up to 5 AMS nodes (code: license.go:135; see tier-order inversion note in §5)

### Enterprise (from $799/month — PROPOSED)
- Everything in Business, plus:
- Anomaly detection (F9: Welford baselines on viewers, bitrate, CPU/mem)
- White-label PDF reports
- SSO / OIDC (shipped Wave 3, D-070/D-074)
- Unlimited nodes and retention
- Air-gapped licensing (roadmap)
- SLA and onboarding support

---

## 7. Marketplace screenshots

Screenshots are produced by `qa/marketplace/capture-live-screenshots.mjs` against a
route-mocked live-app build (Vite preview + Playwright; self-contained, starts and stops
its own server). Output goes to `docs/marketplace/screenshots/` (gitignored).
**The PNGs are not committed to the repository** — run the capture script to produce them
before submission.

| File | Subject | Capture method |
|------|---------|----------------|
| `ss1-dashboard.png` | Live ops dashboard — ~8 streams, viewer counts per protocol, fleet node health | Automated (route-mocked live app) |
| `ss2-ingest-health.png` | Ingest health detail panel open | Automated (route-mocked live app) |
| `ss3-alerting.png` | Alerting rules tab with 3–4 rules and a History firing badge | **Automated via live-app capture** |
| `ss4-analytics.png` | Analytics audience tab — line charts and totals | Automated (route-mocked live app) |
| `ss5-reports.png` | Usage reports tab, Business tier, populated usage table | **Automated via live-app capture** |
| `ss6-probes.png` | Probes page with 3 active probes, Pro+ tier | **Automated via live-app capture** |

SS3 (alerting), SS5 (usage reports), and SS6 (probes) are now captured through the
live-app route-mock harness introduced in the capture script; these are not static
hand-crafted screenshots. To regenerate all six:

```sh
node qa/marketplace/capture-live-screenshots.mjs
```

(Requires `web/dist/` to exist — run `cd web && npm run build` first, or let the script
detect the missing dist and build it. The script uses Playwright Chromium; run
`npx playwright install chromium` in `web/` if browsers are not yet installed.)

---

## 8. Trial-key onboarding paragraph

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

## 9. Support and licensing rows (NEEDS-OPERATOR-CONTACT)

| Row | Status | Action required |
|-----|--------|-----------------|
| Support channel / SLA | NEEDS-OPERATOR-CONTACT | Define and publish a support URL (email, GitHub Issues, or hosted forum). Checklist row 7 in `docs/assessment/final-assessment.md` §3. |
| Public licensing terms | NEEDS-OPERATOR-CONTACT | Publish human-readable licensing terms (PolyForm NC for self-hosted; commercial license for vendor use). Checklist row 8. |
| Revenue-share terms | UNVERIFIED | Negotiate with Ant Media marketplace team. PRD target: 20–30%. Checklist row 9. |
| Listing submission | NEEDS-OPERATOR-CONTACT | Initiate contact with Ant Media developer-relations or marketplace team. Checklist row 10. |
| AMS version support requirement | NEEDS-OPERATOR-CONTACT | Ask Ant Media what minimum AMS version a marketplace product must support. Q5 in `docs/assessment/final-assessment.md` §6. |

---

## 10. Demand evidence citations

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

*Produced at S27/D-089; revised S96/D-160 (2026-07-22). Evidence sources:
`docs/assessment/final-assessment.md` §3, `docs/prd-report.md` §7.11,
`server/internal/license/license.go:90–150` (entitlements), `contracts/openapi/pulse-api.yaml`
(42 paths / 59 operations / 73 schemas), `docs/compatibility.md` §G-27 (new-panel competitive scope),
`qa/marketplace/capture-live-screenshots.mjs` (screenshot file names),
`docs/assessment/prd-validation-matrix.md`.*
