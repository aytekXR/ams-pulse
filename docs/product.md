# Pulse — Product Sheet

> One page on WHAT Pulse is (for anyone), a distilled PRD (for builders), and a ready-to-use
> brand-kit design prompt (for designers / design tools).
> Full PRD + market analysis: [`docs/prd-report.md`](prd-report.md) (§7 is the spec; §§1–6 are
> market context). Technical design: [`docs/ARCHITECTURE.md`](ARCHITECTURE.md).

---

## 1. The product

**Pulse — Real-Time Analytics, QoE Monitoring and Alerting for Ant Media Server.**

Pulse is a fully **self-hosted** observability and audience-analytics suite that installs next
to an Ant Media Server (AMS) deployment and answers, out of the box:

> *Who is watching, where, on what device, with what quality — and is anything broken right now?*

**Positioning in three facts:**
1. **Read-only and upgrade-tolerant.** Pulse polls AMS REST v2 (≤5 s) and never modifies AMS —
   it survives AMS upgrades and cannot break a production streaming stack.
2. **Data never leaves the customer.** Single Go binary + ClickHouse + SQLite via Docker
   Compose or Helm on the customer's own infrastructure. No SaaS, no phone-home.
3. **The AMS marketplace gap.** AMS ships no first-party analytics/QoE product; operators
   today wire Grafana/Prometheus by hand or fly blind. Pulse is the turnkey answer
   (opportunity analysis: prd-report.md §§4–6).

**Who buys it (ICPs, PRD §7.5–7.6):** streaming platform operators running self-hosted AMS
(e-learning, events/worship, gaming, broadcast, surveillance), OEMs embedding AMS, and
agencies operating AMS for clients — anyone who must answer "is the stream OK?" for paying
customers without shipping viewer data to a third party.

**The 10 features (all shipped; current release v0.4.0):**

| # | Feature | One-liner | Tier |
|---|---|---|---|
| F1 | Live ops dashboard | streams, viewers, nodes, ≤10 s visibility, WS push | Free+ |
| F2 | Historical analytics | geo/device breakdowns; rollup history capped by tier retention (Pro 90 d, Business+ 13 mo) | Pro+ |
| F3 | Player QoE beacon SDK | 3.52 KB JS SDK (MIT): startup time, rebuffer, bitrate, errors | Pro+ |
| F4 | Ingest health | 0–100 health score per stream, degradation detection | Free+ |
| F5 | Alerting | email/Slack/Telegram/PagerDuty/webhook, mute, grouping, maintenance windows | Free+ (channels tiered) |
| F6 | Usage/billing reports | per-tenant CSV/PDF, S3 export, ±1% reconciliation | Business+ |
| F7 | Cluster fleet view | auto-discovery ≤30 s, origin/edge roles | Free+ |
| F8 | Data API + Prometheus | full REST/WS API + /metrics scrape | Pro+ |
| F9 | Anomaly detection | Welford baselines, σ-deviation alerts, <1 false alarm/node-week | Enterprise |
| F10 | Synthetic probes | HLS probing (TTFB, bitrate); WebRTC/RTMP/DASH reachability | Pro+ |

**Tiers:** Free / Pro / Business / Enterprise — enforced by ed25519-signed license keys
(`docs/licensing.md`; server gates return 403 `LICENSE_REQUIRED`).

**License:** server + web + deploy = PolyForm Noncommercial 1.0.0; beacon SDK = MIT.

---

## 2. Distilled PRD (from prd-report.md §7 — numbers are acceptance criteria, not aspirations)

- **Problem (§7.2):** AMS operators have no product-grade visibility into audience or QoE;
  DIY Grafana stacks measure servers, not viewers, and take weeks to build.
- **UVP (§7.8):** "See every viewer, every stream, every node — in real time, on your own
  infrastructure." Installs in minutes; zero AMS modification; tiered so a solo operator
  starts free and a platform pays for QoE/billing/anomaly depth.
- **Numeric acceptance criteria** (the binding ones; full list `docs/ARCHITECTURE.md` §4):
  stream visible on dashboard ≤10 s after publish; ingest-degradation detection ≤15 s;
  13-month dimensional analytics query ≤3 s; alert detect→notify ≤5 s; beacon SDK ≤15 KB
  gzip; dashboard usable at 500 concurrent streams; install→first-dashboard ≤10 min.
- **Non-goals:** not a CDN, not a player, not a transcoder controller, not multi-CMS
  analytics (AMS-only by design), not SaaS.
- **Business model (§7.13):** free tier as funnel; Pro/Business/Enterprise license keys sold
  by the vendor (minting ceremony: `docs/licensing.md` §3); noncommercial self-hosting free.
- **Post-GA roadmap (ROADMAP-V2 §2/§3):** anomaly rule type in-product, SSO/OIDC, white-label
  PDF, Postgres meta store (HA), native WebRTC/RTMP/DASH probes, mobile beacon SDKs.

---

## 3. Brand-kit design prompt

Paste the block below into a design tool / AI image generator / brand designer brief.
It encodes the product's actual positioning; do not soften the self-hosted/ops angle.

```text
Design a complete brand kit for "Pulse" — a self-hosted real-time analytics, quality-of-
experience (QoE) monitoring and alerting product for Ant Media Server operators. It is a
serious infrastructure/ops tool used by streaming engineers in NOCs and on-call rotations,
not a consumer app.

BRAND PERSONALITY: precise, calm under pressure, trustworthy, technical. Think "the
instrument panel of a live-streaming operation": every pixel earns its place. Confident but
not flashy; closer to Grafana/Datadog/Tailscale in spirit than to consumer video brands.

NAME & MARK: "Pulse". Design a logomark that reads at 16px favicon size. Strong directions
to explore: (a) a single heartbeat/EKG pulse line that doubles as a live-stream bitrate
sparkline; (b) a ring/radar sweep suggesting real-time monitoring; (c) a waveform merging
into a play-triangle. Avoid generic play buttons alone and avoid literal hearts. Wordmark:
lowercase or smallcaps "pulse", geometric or grotesk sans (e.g. Inter/IBM Plex Sans family
feel), tight tracking; must pair with the mark and stand alone.

COLOR SYSTEM (dashboard-first, dark-mode primary):
- Base: near-black blue-gray backgrounds (#0B0F14 territory) with a light theme variant.
- Primary accent: an electric "signal" color for the pulse line and CTAs — explore
  cyan/teal or spectrum-green ("all systems live"); it must pass WCAG AA on the dark base.
- Semantic set (non-negotiable for an ops tool): healthy green, warning amber, critical red,
  neutral slate — each with dark- and light-theme values, distinguishable under color-vision
  deficiency (verify deuteranopia/protanopia).
- Data-viz palette: 6–8 categorical series colors that hold up on both themes.

TYPOGRAPHY: UI sans for dashboards (high x-height, tabular numerals for metrics), monospace
companion for tokens/IDs/logs (JetBrains Mono / IBM Plex Mono feel). Define a numeric
"metric display" style: big tabular numbers with unit suffixes (ms, kbps, %).

DELIVERABLES: logomark + wordmark (light/dark/mono), favicon 16/32px, social/OG card
template, color tokens (hex, light+dark), type scale, one dashboard hero mockup showing a
live-streams table + a QoE chart using the palette, an alert-state illustration set
(ok/warn/critical), and a one-line brand voice guide (technical, direct, no hype).

CONSTRAINTS: no external font CDNs (self-hosted product — fonts must be self-hostable,
prefer OFL-licensed); works embedded in a React dashboard; the beacon SDK is MIT and
white-labelable, so the mark must degrade gracefully to a neutral "powered by Pulse" badge.
```

---

*Maintained by DOC-01; update when tiers, features, or positioning change. Created D-069
(2026-07-09) on operator request.*
