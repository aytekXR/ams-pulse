# Answers to Ant Media's open items

Reply to the "Still needed before this can be built" table in Ant Media's page draft
(*ams-pulse-page-draft*). All five items are answered below.

---

## 1. Logo / brand mark — ready

Product brand mark for **Pulse** (the product's own mark; it is not a company logo).
Folder: `assets/branding/`.

| Use | File |
|---|---|
| On light backgrounds (website body, footer credit) | `png/pulse-logo-for-light-backgrounds-trimmed-960w.png` · vector: `svg-outlined/pulse-logo-for-light-backgrounds-trimmed.svg` |
| On dark backgrounds (hero) | `png/pulse-logo-for-dark-backgrounds-trimmed-960w.png` · vector: `svg-outlined/pulse-logo-for-dark-backgrounds-trimmed.svg` |
| Single colour | `pulse-logo-mono-black…` / `pulse-logo-mono-white…` |
| Square mark / app icon | `png/pulse-mark-512w.png`, `png/pulse-mark-1024w.png` |
| Small credit badge | `png/powered-by-pulse-badge-600w.png` |

All PNGs have transparent backgrounds. Use the `svg-outlined/` files for the web: the wordmark
is converted to vector outlines, so it renders identically without the IBM Plex Sans font.
Every PNG comes in several sizes (wordmarks 480–1920 px wide, marks 64–1024 px); the `-trimmed`
variants have no extra padding.

## 2. Dashboard & alert screenshots — ready

All captured on 2026-10-07 from the real Pulse application (v0.5.0) (web UI, API, collector, alert
engine and notification delivery all running). The Ant Media Server was Pulse's AMS
simulator, and the viewers were synthetic players, so every stream name and number is demo
data. No customer system was used.

| For | File |
|---|---|
| **Hero (annotated, "Alert triggered")** | `assets/hero/pulse-hero-alert-1920x1080.png` (also 3840×2160) |
| Hero, no annotation | `assets/hero/pulse-hero-dashboard-1920x1080.png` |
| Social / Open Graph card | `assets/hero/pulse-social-1200x630.png` (also 2400×1260) |
| Install step 1 — installation output | `assets/walkthrough/step-1-install-output.png` |
| Install step 4 — sign-in, then first-run dashboard | `assets/walkthrough/step-4a-sign-in.png`, `step-4b-dashboard-first-run.png` |
| Dashboard | `assets/screenshots/01-live-dashboard.png` |
| An alert in action | `assets/screenshots/02-alert-history.png` (fired → resolved), `03-alert-email-delivered.png`, `08-ingest-health-bitrate-drop.png` |
| Alert configuration | `assets/screenshots/05-alert-rules.png`, `06-alert-rule-editor.png`, `07-alert-channels.png` |

Captions for every image: [`asset-inventory.md`](asset-inventory.md).

On the hero callout: your draft suggested "Alert Triggered — Viewer QoE Degraded". We used
the **ingest-bitrate** alert instead, because it fired for real in the capture session and its
value is reliable. The QoE rebuffer-ratio rule exists, but this audit found that its input
metric is understated, and that needs fixing first. We would rather not headline a number we
know is wrong.

## 3. Developer credit — ready

**Developed by Aytekin Erdogan** — an individual developer, not a company
(beyondkaira.com). There is no company logo; the product's own brand mark (§1) is the logo to
use.

## 4. Pricing / purchase link — ready

- **Pricing: free.** Every feature, with no license key, no node or retention limits, and
  commercial use included. This is the launch policy for at least the first year. Pulse is
  licensed under the PolyForm Shield License 1.0.0 — free for any use, including commercial;
  the one restriction is building a competing product. The beacon SDKs are MIT-licensed.
- **Purchase link: none.** There is nothing to buy, so please do not add a purchase or
  checkout button. Point the page's main button at the install page instead:
  **Install Pulse — free** → `https://aytekxr.github.io/ams-pulse/get/`.

## 5. Dashboard URL / port — ready

For the "Open the dashboard" step:

> Open **`http://<your-server>:8090`** and sign in with the admin token the installer printed
> (it starts with `plt_`). The live dashboard opens immediately and shows your AMS streams
> within seconds; the green **Live** badge means updates are streaming in.

Details you may want for the page:

- **8090** is the default port; the installer accepts `PULSE_HOST_PORT=<port>` to use another.
- It serves **plain HTTP**. For anything beyond a private network, put a TLS reverse proxy in
  front and use `https://pulse.your-domain` (a reference nginx configuration is included in the
  repository).
- The quickstart publishes the port on all network interfaces, so restrict it with a firewall
  or a TLS proxy on public servers.
- Additional ports appear only if the operator enables them: 8091 (dedicated player-beacon
  endpoint) and 8092 (signed webhook receiver).

---

## Corrections to the draft

Four statements in the current draft would be inaccurate or would not work as written. The
corrected wording is already in [`marketplace-copy.md`](marketplace-copy.md):

1. **Install step 1 fails as written** — the command needs `--password` as well;
   `curl … | bash` cannot prompt for it and the installer stops with an error.
2. **"No config changes" / "no credentials shared"** — Pulse needs an AMS login, and each AMS
   application's REST IP filter must allow Pulse's IP (AMS defaults to `127.0.0.1` only).
   Accurate version: *no plugin, no downtime — your AMS credentials stay on your own server*.
3. **Helm step** — the chart is experimental (not yet validated on a cluster) and the one-line
   command omits the required secret and schema steps. We suggest dropping it from the page.
4. **"Viewer IPs SHA-256 hashed"** — Pulse does not store viewer IPs at all (stronger than
   hashing). The full list of smaller wording changes is in [`draft-review.md`](draft-review.md).

## Items you are checking — our understanding so far

We have not received these from you. What the repository records is listed for reference
only:

| Item | Status on our side |
|---|---|
| Marketplace submission requirements | No published checklist found; we follow your draft's five items. |
| Review timeline | Unknown — waiting for you. |
| Load-testing expectations | Unknown thresholds/format. We have a packaged load-test harness ready to run once we know the format you need. |
| Commission terms | Pulse is free, so nothing is sold through the listing for now. We would still like your terms in writing for later. |
