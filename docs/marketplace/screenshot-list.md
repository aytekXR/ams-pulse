> **All six listing shots are AUTOMATED from the LIVE APP** (D-161, S97):
> `node qa/marketplace/capture-live-screenshots.mjs` renders the real React UI
> (route-mocked data, 1920×1080, dark theme) — this is the preferred source, since
> it shows the actual product. `render-screenshots.mjs` (brandkit design mocks)
> remains as a fallback for SS1/SS2/SS4 only; never publish a mock render where it
> diverges from the live app.

---

# Marketplace Screenshot Plan

**Product:** Pulse — Analytics & QoE Monitoring for AMS  
**Prepared:** S27 / D-089 (2026-07-13); automation added S28 / D-090 (2026-07-13)

---

## Automation status

| # | File | Status | Method |
|---|------|--------|--------|
| SS1 | `ss1-dashboard.png` | **AUTOMATED (live app)** | `capture-live-screenshots.mjs`; brandkit fallback available |
| SS2 | `ss2-ingest-health.png` | **AUTOMATED (live app)** | `capture-live-screenshots.mjs` (the old `ss2-stream-detail.png` brandkit fallback was retired in S105 — the live capture is canonical) |
| SS3 | `ss3-alerting.png` | **AUTOMATED (live app)** | `capture-live-screenshots.mjs` (was operator-manual) |
| SS4 | `ss4-analytics.png` | **AUTOMATED (live app)** | `capture-live-screenshots.mjs`; brandkit fallback available |
| SS5 | `ss5-reports.png` | **AUTOMATED (live app)** | `capture-live-screenshots.mjs` (was operator-manual; Business-tier data mocked) |
| SS6 | `ss6-probes.png` | **AUTOMATED (live app)** | `capture-live-screenshots.mjs` (was operator-manual) |

**Primary script:** `qa/marketplace/capture-live-screenshots.mjs` — real React UI at
1920×1080 (dark theme), populated via route-mock data. Also produces the user-guide
set (below).

> **⚠ Verify the output by eye, every time — the capture cannot fail loudly.**
> Route mocks are plain objects with no schema validation, and React renders a missing
> field as `UNKNOWN` / `—` / `0` rather than throwing. In S108 this shipped a flagship
> `ss1-dashboard.png` in which **all 8 streams read UNKNOWN state and health** and the
> BY APPLICATION panel read **0 viewers / 0 publishers** — a monitoring product whose own
> dashboard displayed nothing monitored — because three separate mocks used field names
> `web/src/lib/api/schema.d.ts` does not define (`state` vs `publisher_state`,
> `viewer_count` vs `viewers`, `fired_at` vs `ts`). The script exited successfully every
> time. **After any capture: open each PNG and read every panel, not just the one you
> changed, and diff mock keys against the schema before trusting a "successful" run.**
**Fallback script:** `qa/marketplace/render-screenshots.mjs` (brandkit hi-fi mocks, SS1/SS2/SS4 only).
**Rerun command:** `node qa/marketplace/capture-live-screenshots.mjs` (from repo root)  
**Output directory:** `docs/marketplace/screenshots/` — six listing PNGs and the user-guide
set are committed to the repository (S105/D-172); the capture script is portable (no
hardcoded machine paths). Regenerate at any time with the command above.

**User-guide set (same script, same run):** `ug-qoe.png`, `ug-fleet.png`, `ug-anomalies.png`,
`ug-audit-log.png`, `ug-settings-sources.png`, `ug-settings-license.png`, `ug-login.png`,
`ug-onboarding-step2.png`, plus `ss1-light.png` (a genuine light-theme capture as of
S105/D-172 — the historical byte-identical-to-dark bug is fixed: its real cause was
Playwright's default light `prefers-color-scheme` making the whole "dark" set render
light; both themes are now pinned explicitly and the light shot is asserted at capture).

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

**Historical note (SS3/SS5/SS6 — now automated):**
These screens did not exist as standalone layouts in the dc.html and were originally
operator-manual. As of S97 / D-161 they are produced by `capture-live-screenshots.mjs`
alongside SS1/SS2/SS4. The designer option of extending `brandkit/ui/Pulse App -
Screens.dc.html` with new `data-screen-label` sections remains open if a hi-fi
brandkit variant is later desired, but the live-app capture is the authoritative source.

---

## Ordered screenshot list

Screenshots should be captured in this order for the marketplace listing. Typical
AMS marketplace listings use 4–6 screenshots. The priority order reflects feature
importance and demand evidence.

### Screenshot 1 — Live Operations Dashboard

**Status:** AUTOMATED (live app) — `node qa/marketplace/capture-live-screenshots.mjs`  
**Output:** `docs/marketplace/screenshots/ss1-dashboard.png` (1920×1080; brandkit fallback 1282×802)

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
- Dark and light theme both supported — automation captures in dark theme (explicitly pinned since S105); `ss1-light.png` provides a genuine light-theme variant

**Evidence basis:** F1 PARTIALLY → live dashboard validated TC-WH-02, TC-V-03,
TC-FL-01/02 (S17/S18).

---

### Screenshot 2 — Ingest Health and Bitrate Timeline

**Status:** AUTOMATED (live app) — `node qa/marketplace/capture-live-screenshots.mjs`  
**Output:** `docs/marketplace/screenshots/ss2-ingest-health.png` (1920×1080)

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

**Status:** AUTOMATED (live app) — `node qa/marketplace/capture-live-screenshots.mjs` → `ss3-alerting.png` (1920×1080).
Options:
- Extend the dc.html with a new `data-screen-label="Alerting"` screen section (designer decision)
- Take a live-app screenshot once the alerting React route carries real data

**Caption:** "Alerting on any metric — stream offline, bitrate floor, viewer drop.
Delivers to Slack, email, Telegram, PagerDuty, or webhook in under 201 ms."

**Screen to capture:** The Alerts **History** tab, showing real incident events —
at least one still firing and one resolved.

> The Alerts screen is tabbed (Rules · Channels · History), so rules and history
> **cannot** appear in the same shot; this spec previously asked for both, which is
> why the capture drifted. History is the stronger listing asset: it shows the
> product *catching* something, whereas the Rules tab only shows it configured.

**Key elements to show:**
- Alert history rows with severity, state badge (firing / resolved), timestamp and
  the metric value that crossed the threshold
- At least one row in the `firing` state so the live badge colour is visible

**Evidence basis:** F5 PARTIALLY; TC-H-04/05 (S18); N13 (201 ms detection CI).

---

### Screenshot 4 — Audience Analytics

**Status:** AUTOMATED (live app) — `node qa/marketplace/capture-live-screenshots.mjs`  
**Output:** `docs/marketplace/screenshots/ss4-analytics.png` (1920×1080; brandkit fallback 1282×802)

**Caption:** "Historical audience analytics: total views, unique viewers, watch time
and peak concurrency, over a selectable range. 13-month rollup queries return in
under 150 ms."

**Screen to capture:** The Analytics view on its **Audience** tab — the four
headline tiles (total views, unique viewers, watch time, peak concurrency) above
the audience-over-time chart, with the range selector and the Geo/Device tabs
visible but not selected.

> ⚠ **The caption must describe what the capture actually contains.** This entry
> previously promised "QoE rollups" and a "geo breakdown"; the committed capture
> shows neither — it is the Audience tab alone. A listing caption that describes a
> richer screen than the image is the kind of thing a reviewer notices immediately.
> If you want geo in the shot, change the capture script and re-verify the PNG by
> opening it, not by re-reading this file. Note that the country column renders
> blank without a GeoLite2 mmdb, so a geo capture needs that database present or it
> will look broken.

**Key elements to show:**
- Viewer count over time (line chart)
- QoE summary: startup_p50_ms (250 ms in validation), rebuffer_ratio
- Geo map or country list (blank country acceptable — shows the feature)
- CSV export button

**Evidence basis:** F2 PARTIALLY; TC-A-05/06 (S18); N5 (145 ms rollup CI).

---

### Screenshot 5 — Usage and Billing Reports

**Status:** AUTOMATED (live app) — `node qa/marketplace/capture-live-screenshots.mjs` → `ss5-reports.png` (1920×1080, Business-tier data).
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

**Status:** AUTOMATED (live app) — `node qa/marketplace/capture-live-screenshots.mjs` → `ss6-probes.png` (1920×1080).
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

- [x] SS1 Dashboard — AUTOMATED (`node qa/marketplace/capture-live-screenshots.mjs`)
- [x] SS2 Ingest Health — AUTOMATED (`node qa/marketplace/capture-live-screenshots.mjs`)
- [x] SS3 Alerting — AUTOMATED (`node qa/marketplace/capture-live-screenshots.mjs`)
- [x] SS4 Analytics — AUTOMATED (`node qa/marketplace/capture-live-screenshots.mjs`)
- [x] SS5 Usage Reports — AUTOMATED (`node qa/marketplace/capture-live-screenshots.mjs`)
- [x] SS6 Probes (optional) — AUTOMATED (`node qa/marketplace/capture-live-screenshots.mjs`)

---

*Produced at S27/D-089; automation added S28/D-090. Brand assets verified against
`brandkit/logo/` and `brandkit/ui/` directory listings. Design token source:
`brandkit/design-system/tokens.json`. Font: IBM Plex, self-hosted (OFL), never
from CDN per CLAUDE.md §6.*
