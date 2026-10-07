# Asset inventory and captions

Every image in `assets/`, with a caption you can publish and where it came from. All UI images
are real captures of the Pulse application (no mocked API, no redrawn UI), retaken on
2026-10-07 from a v0.5.0 build (every feature free — the sidebar reads FREE). Data is demo data: Pulse's AMS simulator stands in for Ant Media Server, and the
viewers are synthetic players. Annotations on hero images are labelled overlays whose text
reproduces values Pulse recorded.

**Suggested credit line under any screenshot:** *Pulse dashboard, demo environment.*

## Hero and social

| File | Size | Caption | Notes |
|---|---|---|---|
| `hero/pulse-hero-alert-1920x1080.png` (+ `-3840x2160`) | 1920×1080 | *An encoder on `studio-b` drops below 1,500 kbps: Pulse fires the rule and the on-call e-mail arrives 0.6 s later.* | Annotated: framed live dashboard, highlighted stream row, alert card containing the real delivered e-mail. Matches the draft's "annotated screenshot" hero. |
| `hero/pulse-hero-dashboard-1920x1080.png` | 1920×1080 | *Live operations dashboard for Ant Media Server.* | Same frame, no annotation, all streams healthy. |
| `hero/pulse-social-1200x630.png` (+ `-2400x1260`) | 1200×630 | — | Open Graph / LinkedIn / X card with product name and tagline. |

## Install walkthrough (draft steps 1 and 4)

| File | Size | Caption | Notes |
|---|---|---|---|
| `walkthrough/step-1-install-output.png` | 1920×1436 | *One command installs Pulse and ClickHouse, checks the AMS connection and prints a one-time admin token.* | Verbatim output of the published installer with the released v0.5.0 image (2026-10-07), run from `/opt/pulse`; admin token and password masked. The `--ams-url` shown is the demo environment's simulator address. |
| `walkthrough/step-4a-sign-in.png` | 1920×1080 | *Open `http://<your-server>:8090` and sign in with the admin token.* | Released v0.5.0 image, first visit — no "Session expired" message (F1). |
| `walkthrough/step-4b-dashboard-first-run.png` | 1920×1080 | *Streams from your Ant Media Server appear within seconds of the first poll.* | Released v0.5.0 image installed by the published installer; first sign-in through the form — the sidebar's FREE label appears at once (D14 fixed). |

## Screenshots (1920×1080, dark theme)

| # | File | Caption |
|---|---|---|
| 01 | `screenshots/01-live-dashboard.png` | *Live dashboard: concurrent viewers, publishers, node CPU and RAM, protocol mix, per-application and per-stream health — pushed live.* |
| 02 | `screenshots/02-alert-history.png` | *Alert history: an ingest-bitrate warning and a stream-offline alert, each fired and resolved.* |
| 03 | `screenshots/03-alert-email-delivered.png` | *The alert e-mail as delivered (shown in a local test inbox).* |
| 04 | `screenshots/04-alert-inbox-fired-and-resolved.png` | *Firing and resolved notifications for the same incident.* |
| 05 | `screenshots/05-alert-rules.png` | *Alert rules — the four shipped defaults (muted until you add a channel) and four custom rules.* |
| 06 | `screenshots/06-alert-rule-editor.png` | *Editing a threshold rule: metric, operator, window, severity, cooldown, scope.* |
| 07 | `screenshots/07-alert-channels.png` | *Notification channels with a one-click test.* |
| 08 | `screenshots/08-ingest-health-bitrate-drop.png` | *Ingest health: the moment an encoder's bitrate fell from about 3.4 Mbps to under 1,000 kbps.* |
| 09 | `screenshots/09-live-dashboard-during-incident.png` | *Dashboard during the incident (`studio-b` at ~0.9 Mbps; one stream offline).* |
| 10 | `screenshots/10-anomaly-detection.png` | *Anomaly detection learns a baseline for each metric first — until it has enough samples it says so rather than guessing.* (Empty state: baselines need a few hours of live traffic.) |
| 11 | `screenshots/11-synthetic-probes.png` | *Synthetic DASH and RTMP probes checking a stream from Pulse's vantage point.* |
| 12 | `screenshots/12-fleet.png` | *Fleet view of the AMS node with CPU and memory.* |
| 13 | `screenshots/13-sign-in.png` | *Token sign-in.* |
| 14 | `screenshots/14-onboarding-wizard.png` | *Optional setup wizard.* |

Caveats for specific images:

- **09** shows `904.002 Kbps` (an unrounded display value — D9) and the stream's health still
  "GOOD" (lenient score — D10). Prefer **01** or the hero unless the incident is the point.
- **11** shows the simulator's host name (`mock-ams`) in the probe URLs.
- **12** shows a single simulated node (a real cluster would list each node).
- **02** identifies rules by ID rather than name (D7).

Recommended set for the marketplace page: hero → 01 → 02 → 03 → 05 → 08, plus the walkthrough
images for the install steps.

**Deliberately not included:** the Analytics, QoE and Usage-report pages. Their numbers are
affected by defects D1–D4 (submission notes §4). The capture script can produce them once the
fixes ship (`node qa/marketplace/capture-real-stack.mjs --only analytics,qoe,reports`).

## Brand mark (`assets/branding/`)

Product logo for **Pulse**, from the brand source of truth `brandkit/logo/`. It is not a
company logo.

| Folder | Contents |
|---|---|
| `svg/` | The brandkit SVGs as shipped (live text in IBM Plex Sans). |
| `svg-outlined/` | Same artwork with the wordmark converted to outlines (font-independent) — use these on the web. `-trimmed` variants have no extra canvas. |
| `png/` | Transparent PNGs: wordmark (for dark / light backgrounds, mono black / white) at 480, 960, 1920 px wide plus trimmed variants; stacked logo 320–1280 px; square mark and light mark 64–1024 px; favicon 16–180 px; "powered by pulse" badge 300 / 600 px. |

Colours: signal `#2CE5A7` on `#0A0E14` (dark); `#0BA678` mark on white (light). Font: IBM Plex
Sans SemiBold (SIL Open Font License). Outlining was checked against a live-text render: ≤ 0.06%
of pixels differ for the wordmarks.

## Diagram

| File | Size | Caption |
|---|---|---|
| `diagrams/pulse-architecture.svg` / `.png` / `@2x.png` | 1600×900 | *How Pulse attaches to Ant Media Server: read-only REST polling, player beacons, ClickHouse and a local meta store, alerts out to your team.* |

## Reproducing the images

```sh
bash qa/marketplace/demo-stack/up.sh
node qa/marketplace/demo-stack/seed-demo.mjs setup && node qa/marketplace/demo-stack/seed-demo.mjs history
node qa/marketplace/demo-stack/seed-demo.mjs live &          # keep running
node qa/marketplace/capture-real-stack.mjs                    # screenshots
node qa/marketplace/demo-stack/seed-demo.mjs degrade          # incident, then capture alerts
node qa/marketplace/tools/compose-hero.mjs <spec.json>         # hero images
node qa/marketplace/tools/render-terminal.mjs <log> <png> "<command>" 0
node qa/marketplace/tools/build-brand-assets.mjs              # logo exports
bash qa/marketplace/demo-stack/down.sh
```
