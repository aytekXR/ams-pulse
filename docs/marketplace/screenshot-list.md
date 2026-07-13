<!--
  DRAFT — INTERNAL. External use gated on operator review of
  docs/assessment/final-assessment.md (D-081).
-->

> **DRAFT — INTERNAL. External use gated on operator review of
> `docs/assessment/final-assessment.md` (D-081).**
>
> SS1, SS2, and SS4 are now automated via `qa/marketplace/render-screenshots.mjs`.
> SS3, SS5, and SS6 remain operator-manual (no standalone screens in brandkit dc.html).

---

# Marketplace Screenshot Plan

**Product:** Pulse — Analytics & QoE Monitoring for AMS  
**Prepared:** S27 / D-089 (2026-07-13); automation added S28 / D-090 (2026-07-13)

---

## Automation status

| # | File | Status | Method |
|---|------|--------|--------|
| SS1 | `ss1-dashboard.png` | **AUTOMATED** | `node qa/marketplace/render-screenshots.mjs` |
| SS2 | `ss2-stream-detail.png` | **AUTOMATED** | `node qa/marketplace/render-screenshots.mjs` |
| SS3 | `ss3-alerting.png` | **OPERATOR-MANUAL** | No standalone Alerting screen in brandkit dc.html; options: extend dc.html with a new screen section, or take live-app captures once the alerting React route carries real data |
| SS4 | `ss4-analytics.png` | **AUTOMATED** | `node qa/marketplace/render-screenshots.mjs` |
| SS5 | `ss5-billing.png` | **OPERATOR-MANUAL** | No standalone Billing/Usage screen in brandkit dc.html; same options as SS3 |
| SS6 | `ss6-probes.png` | **OPERATOR-MANUAL** | No standalone Probes screen in brandkit dc.html; same options as SS3 |

**Script:** `qa/marketplace/render-screenshots.mjs`  
**Rerun command:** `node qa/marketplace/render-screenshots.mjs` (from repo root)  
**Output directory:** `docs/marketplace/screenshots/` (gitignored — PNGs are reproducible artifacts, not committed)

---

## Logo assets (for listing header)

Verified paths in `brandkit/logo/`:

| Asset | Path | Use |
|-------|------|-----|
| Primary logo (light background) | `brandkit/logo/pulse-logo-primary-light.svg` | Listing header, light theme |
| Primary logo (dark background) | `brandkit/logo/pulse-logo-primary-dark.svg` | Listing header, dark theme |
| Monochrome black | `brandkit/logo/pulse-logo-mono-black.svg` | Print, documents |
| Monochrome white | `brandkit/logo/pulse-logo-mono-white.svg` | Dark-background docs |
| Stacked secondary | `brandkit/logo/pulse-logo-secondary-stacked.svg` | Compact use (square layouts) |
| Mark only (light) | `brandkit/logo/pulse-mark-light.svg` | Favicon / icon contexts |
| Mark only (dark) | `brandkit/logo/pulse-mark.svg` | Favicon / icon contexts |
| Favicon SVG | `brandkit/logo/favicon.svg` | Browser tab |
| PNG marks (256px) | `brandkit/logo/png/pulse-mark-256.png`, `brandkit/logo/png/pulse-mark-light-256.png` | Raster icon contexts |
| PNG favicons | `brandkit/logo/png/favicon-16.png`, `brandkit/logo/png/favicon-32.png`, `brandkit/logo/png/favicon-48.png` | Browser tab, bookmarks |
| Powered-by badge | `brandkit/logo/powered-by-pulse-badge.svg` | Co-marketing, partner pages |

---

## Screen source

The Pulse UI hi-fi screens are designed in:

```
brandkit/ui/Pulse App - Screens.dc.html
```

This is a design-canvas HTML file (the `.dc.html` extension). It contains 8
named screens: Login, Dashboard, Stream Detail, Analytics, Settings, Users and
Tokens, Error and Empty States, Mobile — each a `data-screen-label` div with a
1280×800 inner content div.

**Automated capture** (`qa/marketplace/render-screenshots.mjs`):
- Copies the dc.html and support.js to a temp render directory
- Replaces the Google Fonts CDN `<link>` tags with inline `@font-face` CSS using
  woff2 files from `web/node_modules/@fontsource/` (self-hosted, OFL)
- Pre-stubs `window.React`/`window.ReactDOM` in the support.js copy so the
  dc-runtime boots offline without CDN (screens are static HTML, no React needed)
- Launches Chromium headless, aborts all non-`file://` requests (zero CDN reliance)
- Element-screenshots each matched screen at 1440×900 viewport

**Operator-manual capture** (SS3/SS5/SS6):
- These screens do not exist as standalone layouts in the dc.html
- Options: (a) extend `brandkit/ui/Pulse App - Screens.dc.html` with new screen
  sections (designer decision), or (b) take live-app screenshots once the
  corresponding React routes (alerting, billing, probes) carry real data

---

## Ordered screenshot list

Screenshots should be captured in this order for the marketplace listing. Typical
AMS marketplace listings use 4–6 screenshots. The priority order reflects feature
importance and demand evidence.

### Screenshot 1 — Live Operations Dashboard

**Status:** AUTOMATED — `node qa/marketplace/render-screenshots.mjs`  
**Output:** `docs/marketplace/screenshots/ss1-dashboard.png` (1282×802px)

**Caption:** "Real-time stream overview — viewer counts, active publishers, and node
health at a glance. New streams appear within 4 seconds of publish on AMS 3.0.3."

**Screen to capture:** The main dashboard view showing the live stream grid with
viewer count badges, protocol indicators (HLS, WebRTC, RTMP, DASH), and the fleet
node health panel. The stream list should show at least one active stream with
non-zero bitrate and viewer count.

**Key elements to show:**
- Stream cards with `hlsViewerCount`, `webRTCViewerCount`, health score badge
- Fleet node card (OS, version, status=up)
- Timestamp or "last updated" indicator
- Dark and light theme both supported — capture in the default theme (light)

**Evidence basis:** F1 PARTIALLY → live dashboard validated TC-WH-02, TC-V-03,
TC-FL-01/02 (S17/S18).

---

### Screenshot 2 — Ingest Health and Bitrate Timeline

**Status:** AUTOMATED — `node qa/marketplace/render-screenshots.mjs`  
**Output:** `docs/marketplace/screenshots/ss2-stream-detail.png` (1282×802px)

**Caption:** "Per-publisher ingest health: bitrate, health score, packet loss, and
drop events. Ingest degradation visible within 15 seconds."

**Screen to capture:** The ingest health detail view for a single stream, showing
the bitrate timeseries chart, health score gauge (0–100), and protocol breakdown.
Ideally showing a non-trivial bitrate (~2 Mbps) and health score above 80.

**Key elements to show:**
- Bitrate_kbps chart over time
- Health score gauge (0–100 scale, green above 80)
- Protocol label (RTMP/WebRTC/SRT)
- `from`/`to` time range selector (confirms BUG-004/005 FIXED — handlers now
  honor time range parameters)

**Evidence basis:** F4 PARTIALLY; TC-I-01/02/06 (S17); BUG-004 FIXED S20/D-082.

---

### Screenshot 3 — Alerting — Active Rules and Incident History

**Status:** OPERATOR-MANUAL — no standalone Alerting screen in `brandkit/ui/Pulse App - Screens.dc.html`.
Options:
- Extend the dc.html with a new `data-screen-label="Alerting"` screen section (designer decision)
- Take a live-app screenshot once the alerting React route carries real data

**Caption:** "Alerting on any metric — stream offline, bitrate floor, viewer drop.
Delivers to Slack, email, Telegram, or PagerDuty in under 201 ms."

**Screen to capture:** The alert rules list with at least one active rule (e.g.,
"stream offline" or "ingest bitrate floor"), plus the incident history panel showing
a recent alert event. The Slack/email channel configuration UI would be a bonus
if visible.

**Key elements to show:**
- Alert rule card with threshold and channel assignment
- Alert history list with timestamp, stream ID, and status (fired / resolved)
- Maintenance window indicator (if visible in the screen)

**Evidence basis:** F5 PARTIALLY; TC-H-04/05 (S18); N13 (201 ms detection CI).

---

### Screenshot 4 — Audience Analytics and QoE Rollups

**Status:** AUTOMATED — `node qa/marketplace/render-screenshots.mjs`  
**Output:** `docs/marketplace/screenshots/ss4-analytics.png` (1282×802px)

**Caption:** "Historical audience analytics with viewer QoE: startup time, rebuffer
ratio, watch time, and geo breakdown. 13-month rollup queries return in under 150 ms."

**Screen to capture:** The audience analytics view showing the historical viewer
count chart, QoE summary tile (startup_p50_ms, rebuffer_ratio), and geo analytics
(country breakdown — note: country column blank without GeoLite2 mmdb, but chart
structure is visible). Date range selector visible.

**Key elements to show:**
- Viewer count over time (line chart)
- QoE summary: startup_p50_ms (250 ms in validation), rebuffer_ratio
- Geo map or country list (blank country acceptable — shows the feature)
- CSV export button

**Evidence basis:** F2 PARTIALLY; TC-A-05/06 (S18); N5 (145 ms rollup CI).

---

### Screenshot 5 — Usage and Billing Reports

**Status:** OPERATOR-MANUAL — no standalone Billing/Usage screen in `brandkit/ui/Pulse App - Screens.dc.html`.
Options:
- Extend the dc.html with a new `data-screen-label="Billing"` screen section (designer decision)
- Take a live-app screenshot once the billing React route carries real data

**Caption:** "Usage reports with billing-grade accuracy: viewer-minutes, egress
estimate, VoD recording storage. ±1% reconciliation confirmed against real AMS."

**Screen to capture:** The billing / usage report view showing the monthly
viewer-minutes chart, recording_gb total, egress_gb estimate, and tenant breakdown
(if multi-tenant configured). The CSV export or report schedule panel would
strengthen the screenshot.

**Key elements to show:**
- Viewer-minutes total for the period
- recording_gb (non-zero since BUG-002 FIXED S23/D-085)
- egress_gb (with "estimate" label to be honest about method)
- Per-tenant or per-stream breakdown

**Evidence basis:** F6 PARTIALLY; TC-A-09, TC-REC-01 (S18/S23); BUG-002 FIXED
S23/D-085 (0.02% reconciliation live-validated).

---

### Screenshot 6 — Synthetic Viewer Probes (optional / bonus)

**Status:** OPERATOR-MANUAL — no standalone Probes screen in `brandkit/ui/Pulse App - Screens.dc.html`.
Options:
- Extend the dc.html with a new `data-screen-label="Probes"` screen section (designer decision)
- Take a live-app screenshot once the probes React route carries real data

**Caption:** "Synthetic viewer probes — HLS, WebRTC, RTMP, and DASH probes run
continuously alongside organic viewers. Detect outages from outside your network."

**Screen to capture:** The probes management view showing a configured HLS probe
with recent result history (success=true, ttfb_ms, bitrate_kbps visible). WebRTC
and RTMP probe cards alongside the HLS card strengthen the "all four protocols"
claim.

**Key elements to show:**
- Probe cards with protocol type badge
- Result timeseries: success/failure, ttfb_ms, bitrate_kbps
- "Synthetic" vs "organic" labeling
- Probe interval and last-run timestamp

**Evidence basis:** F10 FULLY; TC-P-01/03/04 (S17); BUG-003 FIXED S20/D-082.

---

## PNG export checklist

- [x] SS1 Dashboard — AUTOMATED (`node qa/marketplace/render-screenshots.mjs`)
- [x] SS2 Stream Detail — AUTOMATED (`node qa/marketplace/render-screenshots.mjs`)
- [ ] SS3 Alerting — OPERATOR-MANUAL (no dc.html screen; extend dc.html or use live-app capture)
- [x] SS4 Analytics — AUTOMATED (`node qa/marketplace/render-screenshots.mjs`)
- [ ] SS5 Billing/Usage — OPERATOR-MANUAL (no dc.html screen; extend dc.html or use live-app capture)
- [ ] SS6 Probes (optional) — OPERATOR-MANUAL (no dc.html screen; extend dc.html or use live-app capture)

---

*Produced at S27/D-089; automation added S28/D-090. Brand assets verified against
`brandkit/logo/` and `brandkit/ui/` directory listings. Design token source:
`brandkit/design-system/tokens.json`. Font: IBM Plex, self-hosted (OFL), never
from CDN per CLAUDE.md §6.*
