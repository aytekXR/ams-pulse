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

| Feature | PRD ref | Status | Notes |
|---|---|---|---|
| Live ops dashboard | F1 | **Shipped** | Streams, viewers, nodes, WS push; ≤10 s stream visibility |
| Historical analytics | F2 | **Shipped** | 13-month rollup queries: 126 ms measured (budget 3 s) |
| QoE beacon SDK | F3 | **Shipped** | TypeScript, 3.44 KB gzip (budget 15 KB); 56 tests; MIT |
| Ingest health monitoring | F4 | **Shipped** | Health score formula; 250 µs detection (budget 15 s) |
| Core alerting | F5 | **Shipped** | Email, Slack, Telegram, PagerDuty, Webhook; maintenance windows; default rule pack |
| Usage / billing reports | F6 | **Shipped** | CSV + PDF; tenant mapping; S3 export; ±1% reconciliation |
| Cluster fleet view | F7 | **Shipped** | Auto-discovery ≤ 30 s (budget 2 min); origin/edge roles |
| Data API + Prometheus | F8 | **Shipped** | 5 bounded metrics; scrape token gate; Grafana starter panels |
| Helm install path | §7.10 | **Shipped** (authored) | Lint and template verified; cluster deploy deferred D-002 |
| Licensing + tier enforcement | — | **Shipped** | Free/Pro/Enterprise; ed25519 verification; 403 on gated features |
| API (REST + WebSocket) | — | **Shipped** | 32 paths, 46 ops, OpenAPI-conformant |
| Onboarding wizard | §7.12 | **Shipped** | 4-step first-run flow |
| Anomaly detection | F9 | Roadmap Wave 3 | Statistical baselines |
| Synthetic probes | F10 | Roadmap Wave 3 | Single probe runner |

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
                                   │  └ cluster discovery (F7)        │
                                   └──────────┬──────────────────────┘
                                              ▼
                              ┌───────────────────────────────────┐
                              │ ClickHouse (events + rollups)      │  90-day raw / 13-month rollups
                              │   viewer_sessions, beacon_events   │
                              │   rollup_audience_1d/1h, qoe_1h    │
                              │ SQLite / Postgres (meta store)     │  rules, users, tokens, schedules,
                              │   tenants, report_schedules        │  alert channels, cluster_nodes
                              └──────────┬────────────────────────┘
                         ┌──────────────┼───────────────┬──────────┐
                         ▼              ▼                ▼          ▼
                   Query API    Alert Evaluator   Report Scheduler  /metrics
                   /qoe/ingest  (F4/F5/cert)     + S3 uploader     Prometheus
                         │              │                │
                         ▼              ▼                ▼
                   Web UI (React)  Slack/Email/    CSV/PDF exports
                   F1 F2 F3 F4     Telegram/PD/
                   F5 F6 F7 F8     Webhook
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
cd web && npm run test                 # 58 component tests (Wave 2)
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
- **Wave 3-MVP:** Anomaly detection (F9), synthetic probes (F10). Statistical baselines, single probe runner. Fix D-W2-002, edge/origin dedup, full QoE ClickHouse queries.
- **Post-MVP:** Mobile beacons, SSO, white-label, air-gapped licensing.
