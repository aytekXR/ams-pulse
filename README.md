# Pulse

**Real-Time Analytics, QoE Monitoring and Alerting for Ant Media Server.**

Pulse is a fully self-hosted observability and audience analytics suite that installs
next to Ant Media Server (AMS) and answers, out of the box: *who is watching, where,
on what, with what quality — and is anything broken right now?*

Integration with AMS is **read-only and upgrade-tolerant**: Pulse never modifies AMS.
Customer data never leaves the customer's infrastructure.

---

## Quick start

**Docker Compose (supported production path):**

```sh
git clone https://github.com/your-org/pulse.git && cd pulse
cp deploy/config/pulse.example.yaml deploy/config/pulse.yaml
# edit deploy/config/pulse.yaml — set your AMS rest_url
export PULSE_AMS_AUTH_TOKEN=your_ams_token
export PULSE_SECRET_KEY=$(openssl rand -hex 32)
make up
# UI on http://localhost:8090
```

> The Docker Compose path was authored and validated by analysis; Docker was not
> available on the build machine (D-002). The local binary path is QA-verified
> (< 2 min to live dashboard). See [docs/runbooks/install.md](docs/runbooks/install.md).

**Local binary (QA-verified):**

```sh
# 1. Get ClickHouse binary and start it
cd /tmp && curl -fsSL https://clickhouse.com/ | sh
/tmp/clickhouse server &

# 2. Build Pulse
cd server && CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/

# 3. Apply migrations and start
PULSE_CLICKHOUSE_DSN=clickhouse://localhost:9000/pulse \
PULSE_META_DSN=/tmp/pulse.db \
/tmp/pulse migrate
PULSE_AMS_URL=http://your-ams:5080 \
PULSE_AMS_AUTH_TOKEN=your_ams_token \
PULSE_META_DSN=/tmp/pulse.db \
PULSE_SECRET_KEY=$(openssl rand -hex 32) \
/tmp/pulse serve

# Copy the admin token printed to stderr, then open http://localhost:8090
```

---

## Feature status

Last updated: Wave-3-Plus (2026-06-15). QA gate: PASS_WITH_LIMITATIONS.

| Feature | PRD ref | Status | Notes |
|---|---|---|---|
| Live ops dashboard | F1 | **Shipped** | Streams, viewers, nodes; WS push broadcasts `LiveOverview` shape; ≤10 s stream visibility; edge/origin viewer dedup active |
| Historical analytics | F2 | **Shipped** | Geo + device breakdown: real rows from `viewer_sessions`; 13-month rollup: 150 ms measured (budget 3 s) |
| QoE beacon SDK | F3 | **Shipped** | TypeScript, 3.52 KB gzip (budget 15 KB); 65 tests; MIT; `rebuffer_end` from HlsAdapter; bitrate from `hls.levels[]` |
| QoE beacon round-trip | F3 | **Shipped** | SDK sends `X-Pulse-Ingest-Token`; main-port `/ingest/beacon` persists to EventSink (64 KB cap); Pro+ tier required; beacon events geo/UA enriched |
| QoE summary (`/qoe/summary`) | F3 | **Shipped** | Queries `rollup_qoe_1h`; `startup_p50_ms` non-zero (250 ms measured); `bitrate_kbps_p50` field |
| Ingest health monitoring | F4 | **Shipped** | Health score formula; `health_score` 0–100 scale; ingest timeseries + drop_events in API; 250 µs detection (budget 15 s) |
| Core alerting | F5 | **Shipped** | Email (Free+), Slack/Telegram (Pro+), PagerDuty/Webhook (Business+); `muted=true` suppresses notifications; `group_by` collapses storm alerts; `node_down` fires on node absence; maintenance windows with range cron syntax |
| Usage / billing reports | F6 | **Shipped** | Business+ tier required; CSV + PDF; tenant mapping; S3 export; ±1% reconciliation; 5-field cron schedules work; `peak_concurrency` sourced from true windowed max (`rollup_concurrency_1d`) |
| Cluster fleet view | F7 | **Shipped** | Auto-discovery ≤ 30 s (budget 2 min); real origin/edge roles; node version field populated |
| Data API + Prometheus | F8 | **Shipped** | 5 bounded metrics; scrape token uses constant-time compare; Grafana starter panels |
| Helm install path | §7.10 | **Shipped** (authored) | Lint and template verified; cluster deploy deferred D-002 |
| Licensing + tier enforcement | — | **Shipped** | 4-tier: Free/Pro/Business/Enterprise (PRD §7.11); ed25519 verification; 403 on gated features; token kind enforcement |
| API (REST + WebSocket) | — | **Shipped** | 32 paths, 46 ops, OpenAPI-conformant; WS origin enforcement; idempotent DELETE documented |
| Onboarding wizard | §7.12 | **Shipped** | 4-step first-run flow |
| Anomaly detection | F9 | **Shipped** (Wave 3-MVP + Wave-3-Plus, Enterprise) | Welford baselines; σ=4.0; 0.259 false alarms/node-week (target <1); minSamples=30 warmup; hysteresis cooldown; epsilon floor — constant-baseline deviations now flagged |
| Synthetic probes | F10 | **Shipped** (Wave 3-MVP + Wave-3-Plus, Pro+) | HLS full — media and master playlists; `ttfb_ms` + `segment_ttfb_ms` stored separately; bitrate >0 for master playlists; webrtc/rtmp/dash reachability-only stubs (Phase-3 roadmap); 60 s config refresh; 4-worker pool; 90-day result TTL |

### Known limitations (Phase-3 / deferred)

- Dashboard render time at 500 streams: virtualization confirmed (≤20 DOM rows), wall-clock not measured — Phase-3 Playwright benchmark (VD-04).
- `rebuffer_ratio` / `error_rate` alert rules: computed from live ingest health heuristic proxy; full ClickHouse `rollup_qoe_1h` query is Phase-3.
- Player CPU <1% budget: not measurable in jsdom/vitest; Phase-3 real-browser profiler needed (VD-14).
- AMS Kafka / log-tail source: no broker available in CI (D-007.5 waiver); REST poller path fully functional and QA-verified. Kafka `lag` + `parse_errors` are surfaced in `/healthz` (Wave-3-Plus).
- Docker Compose not tested on build machine (D-002 waiver); local binary path is QA-verified.

---

## System overview

```
AMS REST v2 / analytics log / Kafka / webhooks ────┐
Player beacons (JS SDK :8091) ─────────────────────┤
Cluster fleet discovery ───────────────────────────┤
                                                    ▼
                                   ┌─────────────────────────────────┐
                                   │  Pulse Collector + Ingest       │
                                   │  ├ restpoller / logtail / kafka  │  single Go binary
                                   │  ├ beacon ingest (:8091)         │
                                   │  ├ session stitcher (F3/F4)      │
                                   │  ├ ingest health tracker (F4)    │
                                   │  ├ cluster discovery (F7)        │
                                   │  ├ probe runner (F10)            │  synthetic HLS checks
                                   │  └ anomaly detector tick (F9)    │  Welford baselines, 60 s
                                   └──────────┬──────────────────────┘
                                              ▼
                              ┌───────────────────────────────────┐
                              │ ClickHouse (events + rollups)      │  90-day raw / 13-month rollups
                              │   viewer_sessions, beacon_events   │
                              │   rollup_audience_1d/1h, qoe_1h    │
                              │   probe_results (90-day TTL, F10)  │
                              │ SQLite / Postgres (meta store)     │  rules, users, tokens, schedules,
                              │   tenants, report_schedules        │  alert channels, cluster_nodes
                              │   probes config (F10)              │
                              │   anomaly_baselines (F9)           │
                              └──────────┬────────────────────────┘
                         ┌──────────────┼───────────────┬──────────┐
                         ▼              ▼                ▼          ▼
                   Query API    Alert Evaluator   Report Scheduler  /metrics
                   /qoe/ingest  (F4/F5/cert)     + S3 uploader     Prometheus
                   /anomalies   /probes + results
                         │              │                │
                         ▼              ▼                ▼
                   Web UI (React)  Slack/Email/    CSV/PDF exports
                   F1 F2 F3 F4     Telegram/PD/
                   F5 F6 F7 F8     Webhook
                   F9 F10
```

---

## Repository layout

| Path | Contents |
|---|---|
| `server/` | Go monorepo: collector, query API, alert evaluator, report generator (one binary, subcommands) |
| `web/` | React + TypeScript dashboard (Vite) |
| `sdk/beacon-js/` | Player QoE beacon SDK (TypeScript, < 15 KB gzipped, MIT — Wave 2) |
| `contracts/` | Source of truth: OpenAPI spec, event JSON schemas, DB migrations |
| `deploy/` | Docker Compose bundle, Dockerfiles, example config, Helm (Wave 2) |
| `agents/` | Multi-agent development workflow: agent definitions, manifest, handoff protocol |
| `docs/` | Architecture, ADRs, runbooks |
| `qa/` | Gate scripts, mock AMS server, budget regression suite |
| `.github/` | CI workflows including AMS version-matrix tests |

---

## Documentation

| Document | Description |
|---|---|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Component diagram, boundaries, performance budgets, Wave-2 implementation status, ingest health score formula |
| [docs/runbooks/install.md](docs/runbooks/install.md) | Install guide: Docker Compose + QA-verified local binary + Helm (Kubernetes) |
| [docs/runbooks/alerting.md](docs/runbooks/alerting.md) | Alert rule semantics, channel setup (Email/Slack/Telegram/PD/Webhook), maintenance windows, HMAC verification |
| [docs/runbooks/reports.md](docs/runbooks/reports.md) | Usage reports: tenant mapping, egress estimation, schedule setup, S3 export, reconciliation |
| [docs/guides/beacon-sdk.md](docs/guides/beacon-sdk.md) | Beacon SDK integration: AMS WebRTC, hls.js, video.js, native video; sampling; privacy |
| [docs/guides/prometheus.md](docs/guides/prometheus.md) | Prometheus scrape config, metric reference, Grafana starter panels |
| [docs/guides/anomaly-detection.md](docs/guides/anomaly-detection.md) | F9 anomaly detection: Welford statistical model, sensitivity calibration, false-alarm math, tuning min_sigma, API usage (Enterprise) |
| [docs/runbooks/probes.md](docs/runbooks/probes.md) | F10 synthetic probes: creating probes, HLS/protocol coverage, result interpretation, synthetic vs organic labeling (Pro+) |
| [docs/adr/0001-tech-stack.md](docs/adr/0001-tech-stack.md) | ADR: Go + React + ClickHouse stack decision |
| [docs/adr/0002-storage-clickhouse.md](docs/adr/0002-storage-clickhouse.md) | ADR: two-store split (ClickHouse + SQLite) |
| [docs/adr/0003-single-binary.md](docs/adr/0003-single-binary.md) | ADR: single binary with role flags |
| [sdk/beacon-js/README.md](sdk/beacon-js/README.md) | Beacon SDK API reference and player integration guide |
| [deploy/helm/pulse/README.md](deploy/helm/pulse/README.md) | Helm chart values table, secrets setup, HA deployment, resource sizing |
| [contracts/README.md](contracts/README.md) | Contract surface, codegen commands, CI validation |
| [contracts/openapi/pulse-api.yaml](contracts/openapi/pulse-api.yaml) | Full OpenAPI 3.1 spec (32 paths, 46 operations, 66 schemas) |
| [agents/README.md](agents/README.md) | Multi-agent build workflow |

---

## Development

```sh
make help      # all targets
make build     # build server binary + web UI
make test      # run all test suites
make lint      # lint everything
```

**Server (Go):**
```sh
cd server && CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go test ./...
```

**Web (React):**
```sh
cd web && npm install && npm run dev   # proxies to pulse serve on :8090
cd web && npm run build                # production build
cd web && npm run test                 # 157 component tests (Wave-3-Plus — 12 suites)
```

**API types (auto-generated from OpenAPI spec):**
```sh
cd web && npm run generate:api
```

**Contract validation:**
```sh
npx @redocly/cli lint contracts/openapi/pulse-api.yaml   # 0 errors/warnings
sqlite3 :memory: < contracts/db/meta/0001_init.sql        # meta DDL
```

---

## Roadmap (from PRD §7.14)

- **Wave 1 / MVP (complete):** Collector, live ops dashboard (F1), historical analytics (F2), core alerting (F5), Docker Compose installer, licensing.
- **Wave 2 (complete):** QoE beacon SDK (F3, 3.44 KB gzip), ingest health (F4, 250 µs detection), usage/billing reports (F6, ±1% reconciliation), cluster fleet view (F7, ≤30 s discovery), full data API + Prometheus (F8), Telegram/PD/webhook channels, Helm chart.
- **Wave 3-MVP (complete):** Anomaly detection (F9, Enterprise — Welford baselines, 0.259 false alarms/node-week), synthetic probes (F10, Pro+ — HLS full coverage, webrtc/rtmp/dash minimal-honest stubs).
- **V3a/V3b fix-loop (complete, 2026-06-15):** Beacon round-trip end-to-end (SDK header, main-port sink, Pro+ gate, geo enrichment); geo/device analytics; QoE rollup queries; ingest health non-zero; alerting muted/group_by/node_down; 4-tier license model (Business tier); report tier gates; 5-field cron; security hardening (CT compare, WS origin, token kind). See `docs/ARCHITECTURE.md` for full defect list.
- **Wave-3-Plus (complete, 2026-06-15):** True windowed peak concurrency in billing (`rollup_concurrency_1d`, maxState/maxMerge; VD-38); alert detect→notify wall-clock test passes at 201 ms (VD-31); 13-month dimensional GROUP BY query at 145 ms (VD-18/C9b); HLS probe segment TTFB (`segment_ttfb_ms`) and master-playlist variant-following for real bitrate; anomaly epsilon floor — constant-baseline deviations now flagged; Kafka lag + parse_errors in `/healthz`.
- **Post-MVP (Phase 3):** Mobile beacons, SSO, white-label PDF, air-gapped licensing, distributed probe network, native RTMP/WebRTC/DASH probing, multi-window anomaly baselines, headless render-time benchmarks.
