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

| Feature | PRD ref | Wave 1 status | Wave 2 status |
|---|---|---|---|
| Live ops dashboard | F1 | **Shipped** — streams, viewers, nodes, WS push | — |
| Historical analytics | F2 | **Shipped** — audience, geo, device tabs (ClickHouse DDL + API + UI) | Full rollup queries |
| Core alerting (email + Slack) | F5 | **Shipped** — rules, channels, history, test-fire | PD/TG/webhook channels, maintenance windows |
| Licensing + tier enforcement | — | **Shipped** — Free tier, ed25519 key verification | Pro/Business tier gating |
| API (REST + WebSocket) | F8 | **Shipped** — 32 paths, 46 ops, OpenAPI-conformant | Full export, Prometheus /metrics |
| Onboarding wizard | §7.12 | **Shipped** — 4-step first-run flow | — |
| QoE beacon SDK | F3 | Roadmap Wave 2 | TypeScript, < 15 KB gzip |
| Ingest health monitoring | F4 | Roadmap Wave 2 | — |
| Usage / billing reports | F6 | Roadmap Wave 2 | CSV + PDF exports |
| Cluster fleet view | F7 | Roadmap Wave 2 | Auto-discovery ≤ 2 min |
| Data API + Prometheus | F8 | Partial (REST only) | Full export + /metrics |
| Anomaly detection | F9 | Roadmap Wave 3-MVP | — |
| Synthetic probes | F10 | Roadmap Wave 3-MVP | — |

---

## System overview

```
AMS REST v2 / analytics log / Kafka / webhooks ─┐
Player beacons (JS SDK, HTTPS) ─────────────────┤
                                                ▼
                                   ┌─────────────────────┐
                                   │  Pulse Collector    │  single Go binary, stateless
                                   └──────────┬──────────┘
                                              ▼
                              ┌───────────────────────────────┐
                              │ ClickHouse (events + rollups)  │  90-day raw / 13-month rollups
                              │ SQLite / Postgres (config)     │  rules, users, tokens, schedules
                              └──────────┬────────────────────┘
                         ┌──────────────┼───────────────────┐
                         ▼              ▼                    ▼
                   Query API     Alert Evaluator      Report Generator
                         │              │                    │
                         ▼              ▼                    ▼
                   Web UI (React)  Slack/Email/…       CSV/PDF exports
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
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Component diagram, boundaries, performance budgets, Wave-1 implementation status |
| [docs/runbooks/install.md](docs/runbooks/install.md) | 15-minute install guide: Docker Compose path + QA-verified local binary path |
| [docs/runbooks/alerting.md](docs/runbooks/alerting.md) | Alert rule semantics, channel setup, test-fire, cooldown |
| [docs/adr/0001-tech-stack.md](docs/adr/0001-tech-stack.md) | ADR: Go + React + ClickHouse stack decision |
| [docs/adr/0002-storage-clickhouse.md](docs/adr/0002-storage-clickhouse.md) | ADR: two-store split (ClickHouse + SQLite) |
| [docs/adr/0003-single-binary.md](docs/adr/0003-single-binary.md) | ADR: single binary with role flags |
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
cd web && npm run test                 # 21 component tests
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
- **Wave 2:** QoE beacon SDK (F3), ingest health (F4), usage/billing reports (F6), cluster fleet view (F7), full data API + Prometheus (F8), Helm.
- **Wave 3-MVP:** Anomaly detection (F9), synthetic probes (F10). Statistical baselines, single probe runner.
- **Post-MVP:** Mobile beacons, SSO, white-label, air-gapped licensing.
