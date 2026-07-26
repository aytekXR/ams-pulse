# Pulse — Marketplace Listing Copy

> **Submission copy.** This file contains the exact text to paste into the Ant Media Marketplace
> listing form. It has no internal identifiers, no internal notes, and no negotiating context.
> Internal working notes, source-code cross-references, and decision history are in
> [`listing-draft.md`](listing-draft.md).

---

## Listing title

**Pulse — Analytics & QoE Monitoring for AMS**

Character count: 43 (limit 60).

---

## Tagline

Self-hosted streaming analytics, alerting, and viewer QoE for Ant Media Server operators.

---

## Short description (≤250 characters)

Pulse installs next to AMS and delivers real-time stream dashboards, QoE beacon analytics,
fleet health monitoring, alerting (email, Slack, Telegram, PagerDuty, webhook), scheduled
PDF/CSV usage reports, and anomaly detection — self-hosted.

Character count: 240 (limit 250).

**Compatibility:** Validated live on AMS 3.0.3 Enterprise (current release); best-effort
compatibility with AMS 2.10+ via mock wire-format profile tests.

---

## Category (proposed)

**Analytics and Monitoring**

*Proposed pending Ant Media confirmation (assumption A10 — verify at the developer meeting).*

---

## Long-form description

Pulse is a fully self-hosted observability and audience-analytics platform for Ant Media Server
operators. It installs as a sidecar — a single Docker Compose stack or Helm chart placed
alongside an existing AMS deployment — and begins polling the AMS REST v2 API within seconds.
Pulse never modifies AMS configuration, never proxies media traffic, and requires no AMS plugin
or code change. Because Pulse is read-only, it survives AMS upgrades without adjustment and
cannot interfere with a live streaming stack. The beacon SDK is MIT-licensed and may be embedded
in any player, including commercial products, without restriction; only the Pulse server and web
UI require a commercial subscription for production use.

Out of the box Pulse answers the questions AMS does not: who is watching, on what device, with
what quality, and is anything broken right now. The live operations dashboard surfaces stream
health, viewer counts by protocol (HLS, WebRTC, RTMP, DASH), ingest bitrate, and fleet node
status — all updated within ten seconds of a publish event, with no page refresh required. A
player-side QoE beacon SDK (3.52 KB gzip, MIT-licensed) captures startup latency, rebuffer
ratio, bitrate switches, and error rates directly from viewers' browsers without routing data
through any third-party server.

Pulse is built for AMS operators who need product-grade visibility — streaming platforms,
e-learning providers, event broadcasters, OEMs, and managed-service agencies. Data stays on the
operator's own infrastructure: Pulse stores metrics in a self-hosted ClickHouse instance and
exposes them through a fully documented REST and WebSocket API plus a Prometheus /metrics
endpoint. There is no SaaS component, no phone-home, and no vendor lock-in. License keys are
self-contained signed tokens verified entirely offline — no connection to any Pulse or vendor
server is required at activation or runtime — making Pulse suitable for air-gapped and
restricted-network deployments.

What sets Pulse apart from a DIY Grafana/Prometheus stack is purpose-built AMS integration and
time-to-value. A standard Grafana stack measures servers; Pulse measures viewers. The five alert
channel types (email, Slack, Telegram, PagerDuty, webhook) fire within 201 ms of threshold
breach in lab validation. The three-rung node health ladder — API latency anomaly, node
degraded, node down — detects AMS freeze conditions within 15 seconds without requiring
OS-level metric access. Business-tier billing reports reconcile viewer-minutes to within one
percent accuracy (confirmed in CI against 10,000 synthetic events); egress figures are a
directional estimate derived from AMS REST counters and should be cross-checked with CDN logs
for billing-grade invoicing.

Pulse ships all ten analytics features in v0.4.1, validated against a live AMS 3.0.3 Enterprise
deployment. Install takes under 15 minutes from a Docker Compose quickstart. A 14-day Pro trial
(no credit card) is available on request; the deployment gracefully reverts to the Free tier on
expiry with no data loss.

---

## Feature bullets

1. **Live ops dashboard** — new stream visible within 4 seconds of publish (measured in live AMS
   3.0.3 validation); viewer counts per protocol (HLS, WebRTC, RTMP, DASH); fleet node health;
   auto-discovers all AMS applications; cluster fleet view with wire shape verified against AMS
   3.0.3 source — live multi-node validation pending.

2. **Player QoE beacon SDK** — 3.52 KB gzip (15 KB budget); MIT-licensed; adapters for AMS
   WebRTC, hls.js, and video.js; reports startup time, rebuffering, errors, bitrate switches,
   and watch time from the viewer's browser. (Pro+)

3. **Alerting on any metric — 5 channel types** — stream offline, ingest bitrate floor, viewer
   count drop, node-degraded, node-down; 201 ms detection-to-notification (measured in CI and
   lab validation). Channels by tier: email (Free+); email + Slack + Telegram (Pro+); all 5
   channels including PagerDuty and webhook (Business+). Demand signal:
   ant-media/Ant-Media-Server#7926 — AMS freeze under high RTMP load; Pulse's three-rung
   detection ladder directly addresses this failure class.

4. **Synthetic probes + full observability API** — active connectivity probes over HLS, WebRTC,
   RTMP, and DASH verify stream reachability from Pulse's own vantage (Pro+); 13-month (396-day)
   retention window for long-horizon trend and rollup analysis (Business+); Prometheus /metrics
   endpoint in standard exposition format (Business+); spec-first OpenAPI 3.1 API (42 paths, 59
   operations) with response-body conformance enforced in CI. Demand signal:
   ant-media/Ant-Media-Server#3122 — Prometheus exporter requested 2021, closed 2023 without
   implementation; Pulse ships this natively.

5. **Usage and billing reports** — viewer-minutes, peak concurrency, VoD recording storage;
   viewer-minutes reconciled to ±1% (confirmed in CI against 10,000 synthetic events); egress is
   a directional estimate — use CDN logs for invoicing; on-demand CSV export and scheduled PDF
   and CSV delivery. (Business+)

6. **Anomaly detection** — Welford statistical baseline on viewer counts and bitrate; fewer than
   0.26 false alarms per node per week at default sensitivity (measured in CI/lab validation).
   (Enterprise)

---

## Tier and pricing table

| Tier | Price | Max Nodes | Retention | Alert Channels | Notes |
|------|-------|-----------|-----------|----------------|-------|
| **Free** | $0/month | 1 | 7 days | Email only | **Noncommercial use only** (PolyForm Noncommercial 1.0.0); commercial deployments start at Pro |
| **Pro** | $99/month | 10 | 90 days | Email, Slack, Telegram | QoE beacon SDK; synthetic probes; data API |
| **Business** | $299/month | 50 | 396 days (13 months) | All 5 channels | Usage reports; Prometheus /metrics; scheduled PDF/CSV; multi-tenant |
| **Enterprise** | from $799/month | Unlimited | Unlimited | All 5 channels | Anomaly detection; white-label PDF; SSO/OIDC; air-gapped licensing |

Monthly pricing. Annual subscriptions billed at 10× the monthly rate (two months free). Standard
rates from year two — see the Founding Operators campaign below.

### Founding Operators — launch campaign

Available to any paid-tier deployment activated during the first 6 months after the marketplace
listing goes live, or the first 100 paid activations — whichever comes first.

| Tier | First 12 months | Standard price after |
|------|-----------------|----------------------|
| Pro | **$9/month** (~91% off) | $99/month |
| Business | **$29/month** (~90% off) | $299/month |
| Enterprise | **90-day free pilot, then 25% off year one** | from $799/month |

Campaign price is locked at signup. At the 12-month renewal the subscription auto-reverts to
standard pricing with 30 days' advance email notice. Founding Operators keep a permanent 10%
loyalty discount on standard pricing at every renewal thereafter. Free stays free.

### 14-day Pro trial

**14-day Pro trial — no credit card.** Request a trial key from the marketplace listing or by
emailing **support@beyondkaira.com**; the key arrives by email (typically within 1 business day)
and activates in Settings → License. On expiry the deployment gracefully reverts to Free — no
data loss. A trial that converts within the Founding Operators launch window qualifies for the
campaign price.

---

## What's included per tier

### Free
- Live operations dashboard (stream list, viewer counts, fleet node health)
- Stream start/stop alerting (email)
- 7-day data retention
- Docker Compose single-node install
- Community support (GitHub Issues)
- **Noncommercial use only** — PolyForm Noncommercial 1.0.0; any commercial AMS deployment
  requires a Pro, Business, or Enterprise subscription

### Pro ($99/month)
- Everything in Free, plus:
- Player QoE beacon SDK integration (AMS WebRTC, hls.js, video.js adapters)
- Historical QoE analytics (startup p50, rebuffer ratio, error rate)
- Synthetic viewer probes (HLS, WebRTC, RTMP, DASH)
- Slack and Telegram alert channels
- On-demand CSV data export
- 90-day data retention
- Up to 10 AMS nodes

### Business ($299/month)
- Everything in Pro, plus:
- Usage and billing reports (viewer-minutes, egress estimate, VoD recording storage)
- On-demand CSV export and scheduled PDF and CSV delivery (via report schedules API)
- Multi-tenant billing (stream-name pattern or metadata tag)
- Prometheus /metrics endpoint
- PagerDuty and webhook alert channels (all 5 channel types)
- 396-day (13-month) data retention — enables long-horizon trend analysis and rollups
- Priority email support (1-business-day response target)
- Up to 50 AMS nodes

### Enterprise (from $799/month)
- Everything in Business, plus:
- Anomaly detection (Welford baselines on viewers, bitrate, CPU/memory)
- White-label PDF reports
- SSO / OIDC
- Unlimited nodes and retention
- Air-gapped licensing — supported today: activate via `PULSE_LICENSE_FILE`; no phone-home
  required at activation or runtime
- SLA and onboarding support (4-business-hour response target, Mon–Fri 09:00–18:00 UTC)

---

## Install summary

Pulse installs in under 15 minutes via:

- **Docker Compose** — `install.sh` (health-gated, no-TTY safe) sets up the full stack with a
  single command; database migrations are baked into the Docker image (no bind mount required).
- **Helm** — chart available in-repo; install from a local chart path.
- **Binary** — build from source with Go 1.22+.

After installing:

1. Copy the admin token printed to stderr on first boot.
2. Open `http://localhost:8090` and log in.
3. Streams from your AMS instance appear within 10 seconds of publish.
4. To activate your Pro or Business license: paste your license key in **Settings → License**
   and click Activate. Features unlock immediately — no restart required.

To start your 14-day Pro trial, email **support@beyondkaira.com** or request a trial key from
the marketplace listing; the key arrives by email within 1 business day. For support, contact
support@beyondkaira.com.

---

## Support

| Tier | Channel | Response target | Hours |
|------|---------|-----------------|-------|
| Free | GitHub Issues | Community | — |
| Pro | support@beyondkaira.com | 2 business days | Mon–Fri 09:00–18:00 UTC |
| Business | support@beyondkaira.com | 1 business day | Mon–Fri 09:00–18:00 UTC |
| Enterprise | support@beyondkaira.com | 4 business hours | Mon–Fri 09:00–18:00 UTC |

Full support policy: docs/support.md · Licensing details: docs/licensing-public.md

---

## Screenshots

All shots at 1920×1080, dark theme. Regenerate: `node qa/marketplace/capture-live-screenshots.mjs`.

| # | File | Subject |
|---|------|---------|
| SS1 | `ss1-dashboard.png` | Live ops dashboard — streams, viewer counts by protocol, fleet node health |
| SS2 | `ss2-ingest-health.png` | Ingest health detail — bitrate timeline, health score, protocol breakdown |
| SS3 | `ss3-alerting.png` | Alerting rules with incident history — active rule and firing badge |
| SS4 | `ss4-analytics.png` | Audience analytics — viewer count chart, QoE rollups, geo breakdown |
| SS5 | `ss5-reports.png` | Usage reports — viewer-minutes, egress estimate, VoD storage (Business tier) |
| SS6 | `ss6-probes.png` | Synthetic viewer probes — HLS, WebRTC, RTMP, DASH probe results |

A light-theme variant of SS1 is also available (`ss1-light.png`).
