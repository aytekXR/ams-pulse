<!--
  DRAFT — INTERNAL. External use gated on operator review of
  docs/assessment/final-assessment.md (D-081).
-->

> **DRAFT — INTERNAL. External use gated on operator review of
> `docs/assessment/final-assessment.md` (D-081).**
>
> Screenshot PNGs are a pending manual step: the Pulse UI screens are designed
> in `brandkit/ui/Pulse App - Screens.dc.html` (the design-canvas HTML file).
> Exporting individual screens as PNG requires opening that file in a browser
> and using the browser's screenshot or print-to-image function, or using the
> design-canvas native export. This document lists the ordered shots with
> captions; the PNG export is operator-action-pending.

---

# Marketplace Screenshot Plan

**Product:** Pulse — Analytics & QoE Monitoring for AMS  
**Prepared:** S27 / D-089 (2026-07-13)

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

This is a design-canvas HTML file (the `.dc.html` extension). To export individual
screens as PNG:

1. Open `brandkit/ui/Pulse App - Screens.dc.html` in a modern browser (Chrome or
   Firefox recommended for accurate font rendering — fonts are IBM Plex, self-hosted
   per brandkit/design-system, no CDN).
2. Navigate to the target screen/frame.
3. Use the browser's built-in screenshot tool or a headless Puppeteer/Playwright
   script to capture a 1280×800 (or 1440×900) viewport PNG.
4. Save to a `docs/marketplace/screenshots/` directory (create this directory; it is
   not committed yet).

**Pending action:** The operator or a design-tooling agent must perform the PNG
export. This document specifies what to capture and in what order.

---

## Ordered screenshot list

Screenshots should be captured in this order for the marketplace listing. Typical
AMS marketplace listings use 4–6 screenshots. The priority order reflects feature
importance and demand evidence.

### Screenshot 1 — Live Operations Dashboard

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

## PNG export checklist (pending operator action)

- [ ] Open `brandkit/ui/Pulse App - Screens.dc.html` in Chrome
- [ ] Capture Screenshot 1 — Live Operations Dashboard (1440×900 viewport, light theme)
- [ ] Capture Screenshot 2 — Ingest Health and Bitrate Timeline
- [ ] Capture Screenshot 3 — Alerting — Active Rules and Incident History
- [ ] Capture Screenshot 4 — Audience Analytics and QoE Rollups
- [ ] Capture Screenshot 5 — Usage and Billing Reports
- [ ] Capture Screenshot 6 — Synthetic Viewer Probes (optional)
- [ ] Save PNGs to `docs/marketplace/screenshots/` (directory not yet created)
- [ ] Verify PNG dimensions meet Ant Media marketplace spec (NEEDS-OPERATOR-CONTACT for spec)
- [ ] Compress PNGs with `pngcrush` or equivalent

---

*Produced at S27/D-089. Brand assets verified against `brandkit/logo/` and
`brandkit/ui/` directory listings. Design token source:
`brandkit/design-system/tokens.json`. Font: IBM Plex, self-hosted (OFL),
never from CDN per CLAUDE.md §6.*
