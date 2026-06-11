# Pulse

**Real-Time Analytics, QoE Monitoring and Alerting for Ant Media Server.**

Pulse is a fully self-hosted observability and audience analytics suite that installs
next to Ant Media Server (AMS) and answers, out of the box: *who is watching, where,
on what, with what quality — and is anything broken right now?*

> Status: **skeleton / pre-build**. Structure is final; business logic is implemented
> incrementally by the agent workflow described in [`agents/README.md`](agents/README.md).
> Product requirements live in [`prd-report.md`](prd-report.md) (Section 7 is the PRD).

## System overview

```
AMS REST v2 / analytics log / Kafka / webhooks ─┐
Player beacons (JS SDK, HTTPS) ─────────────────┤
                                                ▼
                                   ┌─────────────────────┐
                                   │  Pulse Collector    │  single Go binary, stateless
                                   └──────────┬──────────┘
                                              ▼
                                   ┌─────────────────────┐
                                   │  ClickHouse         │  events + rollups (90d raw / 13mo rollups)
                                   │  SQLite/Postgres    │  config, users, alert rules, schedules
                                   └──────────┬──────────┘
                              ┌───────────────┼────────────────┐
                              ▼               ▼                ▼
                        Query API       Alert Evaluator   Report Generator
                              │               │                │
                              ▼               ▼                ▼
                        Web UI (React)  Slack/Email/PD/…   CSV/PDF exports
```

Integration with AMS is **read-only and upgrade-tolerant**: Pulse never modifies AMS.

## Repository layout

| Path | Contents |
|---|---|
| `server/` | Go monorepo: collector, query API, alert evaluator, report generator (one binary, subcommands) |
| `web/` | React + TypeScript dashboard (Vite) |
| `sdk/beacon-js/` | Player QoE beacon SDK (TypeScript, <15 KB gzipped, MIT — to be open-sourced) |
| `contracts/` | Source of truth: OpenAPI spec, event JSON schemas, DB migrations. All agents code against these. |
| `deploy/` | Docker Compose bundle, Dockerfiles, example config, Helm (Phase 2) |
| `agents/` | Multi-agent development workflow: agent definitions, manifest, handoff protocol |
| `docs/` | Architecture, ADRs, runbooks |
| `.github/` | CI workflows incl. AMS version-matrix tests |

## Quick start (target state — not functional yet)

```sh
cd deploy && docker compose up -d
# UI on http://localhost:8090 — point it at your AMS REST endpoint
```

## Development

```sh
make help     # all targets
make build    # build server binary + web UI + beacon SDK
make test     # run all test suites
make lint     # lint everything
```

## Roadmap (from PRD §7.14)

- **Phase 1 / MVP (wk 1–10):** Collector, live ops dashboard (F1), basic historical analytics (F2), core alerting (F5), Docker Compose installer, licensing.
- **Phase 2 (wk 11–18):** QoE beacon SDK (F3), ingest health (F4), full rollups (F2), usage/billing reports (F6), cluster fleet view (F7), data API + Prometheus (F8), Helm.
- **Phase 3 (wk 19–30):** Mobile beacons, anomaly detection (F9), synthetic probes (F10), SSO, white-label, air-gapped licensing.
