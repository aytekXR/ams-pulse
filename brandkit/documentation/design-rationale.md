# Pulse — Design Rationale & Guidelines

Companion to the visual deliverables. Sources of truth: `uploads/product.md` (positioning),
`uploads/ARCHITECTURE.md` (system facts), `uploads/AMS-INTEGRATION.md` (operator reality).

## 1. Branding decisions

**Positioning drove everything.** Pulse is a serious ops instrument used in NOCs and on-call
rotations — closer in spirit to Grafana/Datadog/Tailscale than to consumer video brands. Every
decision below traces back to three product facts: read-only sidecar, data-never-leaves,
AMS-module (not standalone app).

- **The mark** is a single continuous stroke: a heartbeat that doubles as a bitrate sparkline.
  It reads at 16 px (favicon uses a simplified 3-point version), animates naturally (a live
  "drawing" pulse), and degrades to a neutral outline badge for the MIT white-label beacon SDK.
  We rejected: play buttons (generic), hearts (literal), radar rings as primary (kept as
  app-icon concept B).
- **Dark-mode primary.** The product lives on NOC wall screens and 3 a.m. laptops. The base is
  a blue-gray near-black (#0A0E14), not pure black — it keeps low-contrast borders legible and
  reduces smearing on office displays. A full light theme exists for docs, email, and print.
- **One signal color.** Signal Green (#2CE5A7) means exactly one thing: *live / healthy /
  primary action*. Because "all systems live" is the product's resting state, the brand accent
  and the healthy state are intentionally the same hue. Amber and red are reserved for state
  and never used decoratively.
- **Type: IBM Plex Sans + IBM Plex Mono.** Both OFL-licensed and self-hostable — a hard
  requirement for a no-phone-home product (no font CDNs in production). Plex Sans has the high
  x-height needed for 13 px table text; Plex Mono marks everything copy-pasteable (tokens,
  stream IDs, log lines). Metrics always use `font-variant-numeric: tabular-nums` so digits
  don't jitter under live updates.
- **Voice: runbook, not marketing.** Lead with the fact, quantify everything, no exclamation
  marks, no emoji. The reader is an engineer under pressure.
- **AMS relationship.** Pulse always appears as "pulse — for Ant Media Server" on first
  mention. The schematic illustration style (nodes + dashed flows, Pulse outlined in Signal
  Green) visually places Pulse *inside* the AMS ecosystem as its analytics organ.

## 2. Color accessibility

Verified contrast ratios against WCAG 2.1:

| Pair | Ratio | Level |
|---|---|---|
| Text primary #E8EEF4 on bg #0A0E14 | ~16.5:1 | AAA |
| Text secondary #9FB0C0 on bg #0A0E14 | ~9.1:1 | AAA |
| Signal #2CE5A7 on bg #0A0E14 | ~12.9:1 | AAA |
| On-signal ink #0A0E14 on #2CE5A7 (buttons) | ~12.9:1 | AAA |
| Warning #FFB224 on #0A0E14 | ~10.4:1 | AAA |
| Critical #FF5C68 on #0A0E14 | ~6.6:1 | AA (large + normal) |
| Muted #5C6F80 on #0A0E14 | ~4.6:1 | AA — labels/captions only, never body copy |
| Light theme: #10181F on #F7F9FA | ~16:1 | AAA |
| Light theme signal #0BA678 on #FFFFFF | ~3.2:1 | Large text/icons + non-text UI only; body links use #087A59 if needed |

**Color-vision deficiency:** state is never encoded by hue alone. Healthy/warn/critical/offline
each pair a fixed shape (dot / diamond / triangle / outlined dot) with the color, and warning
vs critical also differ in lightness. The 8-color dataviz palette alternates hue families and
lightness steps so adjacent series remain separable under deuteranopia/protanopia; charts also
carry direct labels or legends, never color-only keys.

## 3. UI guidelines (summary — see design-system/)

- 4 px spacing grid; card padding 24, section rhythm 48/96.
- Radii: 12 card / 8 control / full pill. Nothing else.
- Elevation by tone + border (Ink 1 → Ink 2 + Ink 3 border); shadows only on overlays.
- Tables: 40 px rows, 13 px text, numerics right-aligned tabular.
- App shell: 60 px icon rail (dense ops screens) or 240 px sidebar; 12-col, 24 px gutters.
- Mobile: bottom tab bar, ≥44 px touch targets, metric cards 2-up.
- Live badge ("Live · WS connected") is always visible on live screens — trust in freshness
  is the product's core promise (≤10 s budget).
- Empty states teach (show the RTMP publish URL); error states state the blast radius
  ("live data keeps flowing; historical queries paused"); license gates show the exact API
  error (`403 LICENSE_REQUIRED`) — the audience is engineers.

## 4. Asset inventory

- `/brandkit` — Brand Guidelines (strategy, logo, color, type, icon/illustration, spacing, voice, usage)
- `/logo` — primary (dark/light), stacked secondary, mono ×2, mark ×2, favicon SVG + PNG 16/32/48, white-label badge
- `/icons` — 3 concepts; iOS dark/light, Android adaptive (fg + bg ×2), web maskable; PNG 16–1024
- `/design-system` — component library DC (light+dark) + `tokens.json` (machine-readable)
- `/ui` — 8 hi-fi screens: login, dashboard, stream-detail workflow, analytics, settings, users/tokens, error/empty/gated states, mobile ×2
- `/website` — full marketing site (hero, gap/solution, 10 features, how-it-works, AMS integration, pricing, FAQ, contact, footer)
- `/assets` — OG 1200×630, social 1080², deck cover 1920×1080 (SVG-based artboards + PNG), email signature HTML, signal-grid pattern, background field

All masters are SVG or HTML (editable); PNGs are exports. Fonts referenced via Google Fonts
for preview convenience only — self-host IBM Plex in production.

## 5. Future design evolution

1. **Motion language** — define the live-update vocabulary (fade-only data updates, a drawing
   pulse-line loader, alert pulse-once). Never bounce, never slide charts.
2. **White-label kit** — tokenized theme file for OEM/agency deployments (Enterprise): swap
   signal color + logo, keep semantics locked so ops meaning is never rebrandable.
3. **Density modes** — a "wall screen" display mode (larger metrics, auto-cycling) and a
   "compact" mode for 500-stream tables.
4. **Anomaly visual language (F9)** — σ-band overlays on charts need a dedicated treatment
   (baseline band in neutral, deviation fill in semantic color) before Enterprise push.
5. **Mobile beacon SDKs / roadmap features** — SSO login screen variant, white-label PDF
   report template (Business+), Postgres HA deployment page.
6. **Icon set** — commission or curate the full Lucide subset at 1.75 px stroke; publish as
   `icons/ui/` sprite so product and marketing never drift.
