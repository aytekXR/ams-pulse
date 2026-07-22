<!--
  DRAFT — INTERNAL. External use gated on operator review of
  docs/assessment/final-assessment.md (D-081).
-->

# Marketplace demo video — storyboard & script

**Target:** 2:30–3:00 min, 1920×1080, dark theme. Mirrors and deepens the live demo
already shown to Ant Media ("the live dashboard looked good"). Recording is an
**operator step** (screen recording over the seeded demo stack); an experimental
Playwright `recordVideo` rough cut is possible from the capture script's mock setup but
a human take with narration is expected to look better.

**Setup before recording:** demo stack with populated screens — either the compose
override + mock-AMS (`docker compose -f deploy/docker-compose.yml -f
deploy/docker-compose.override.yml up -d`, then seed via
`curl -X POST http://127.0.0.1:9090/control/bulk_publish -d '{"count":8,"prefix":"demo-","viewers_each":120}'`)
or the production instance with real streams. Enterprise license key active so every
page renders (no tier-gate cards on camera). Hide any real credentials/domains if
recording against production.

| # | Scene (duration) | Screen / action | Narration (spoken) |
|---|---|---|---|
| 1 | Cold open (0:00–0:15) | Live dashboard, populated: stat cards, protocol donut, streams table with the green "Live" WS indicator | "This is Pulse — self-hosted analytics, monitoring and alerting for Ant Media Server. It installs next to AMS in about fifteen minutes, reads the REST API, and never modifies your server." |
| 2 | Install cred (0:15–0:30) | Quick cut: terminal running the one-command quickstart, then the 4-step onboarding wizard (Add source → Verify green check) | "One command installs the stack. The wizard connects your first AMS instance — URL, credentials, test connection, done." |
| 3 | Live ops (0:30–0:50) | Back on the dashboard: hover a stream row, open stream detail; point at viewers/publishers/CPU cards | "Every stream, viewer and node, live within ten seconds. Edge and origin viewers are deduplicated, so the numbers are real." |
| 4 | Alerting (0:50–1:15) | Alerts page: rules list; stop a mock publisher (or toggle the demo control) → alert fires; show the Slack/email notification arriving | "Alert rules watch streams, nodes and viewer experience. Here a stream drops — Pulse pages Slack in seconds. Email, Telegram, PagerDuty and webhooks are built in, with maintenance windows and storm grouping." |
| 5 | Viewer QoE (1:15–1:40) | QoE page with beacon data: startup p50/p95, rebuffer ratio, bitrate timeline | "This is the part server-side tools can't see: a 3.5 KB open-source beacon in your player reports real startup times, rebuffering and bitrate — per stream, per geography, per device." |
| 6 | History & reports (1:40–2:05) | Analytics page (13-month range, geo/device tabs) → Reports page (usage table, schedule with PDF format) | "Thirteen months of audience history, sub-second queries. Usage and billing reports reconcile viewer-minutes and egress to within one percent — scheduled as CSV or PDF, exported to S3." |
| 7 | Fleet & probes (2:05–2:25) | Fleet page (node cards) → Probes page (HLS probe result timeline) → Anomalies page (one row, sigma badge) | "Cluster nodes are discovered automatically. Synthetic probes test your streams from the outside — HLS, DASH, WebRTC, RTMP — and statistical baselines flag anomalies before users complain." |
| 8 | Close (2:25–2:50) | Overview diagram (docs/overview.md architecture figure) → logo card with URL | "Self-hosted, read-only against AMS, your data stays yours. Pulse — analytics and QoE monitoring for Ant Media Server. Free tier available; install it today." |

**Don'ts:** no real customer stream names on camera; no tokens/credentials visible
(blur the Settings token screens if shown); don't show the staging AMS panel or any
Ant Media confidential material; keep version strings visible (credibility).

**Deliverables:** `pulse-demo.mp4` (H.264, ≤100 MB), a 30-second cutdown for social
(scenes 1+4+5+8), and a poster frame (scene 1 at 0:05).
