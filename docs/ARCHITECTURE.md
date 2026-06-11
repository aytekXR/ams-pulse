# Pulse Architecture

Authoritative technical-design document. PRD: `prd-report.md` §7. Decisions with
trade-offs get an ADR in `docs/adr/`.

## 1. System context

Pulse is a **read-only sidecar** to Ant Media Server. It never modifies AMS, needs no
inbound access to AMS beyond the existing REST port, and stores all data on the
customer's infrastructure. That "data never leaves" property is the product's core
differentiator (PRD §7.1) — any design that ships customer data to us (telemetry,
crash reporting) must be opt-in and documented.

## 2. Components

```
                ┌──────────────────────────── pulse binary ───────────────────────────┐
AMS REST ──────►│ collector/restpoller ─┐                                             │
AMS log ───────►│ collector/logtail ────┤                                             │
AMS Kafka ─────►│ collector/kafka ──────┼─► normalize ─► store/clickhouse (events)    │
AMS webhooks ──►│ collector/webhook ────┤        │                                    │
Player beacons ►│ collector/beacon ─────┘        ├─► alert/evaluator ─► channels ────────► Slack/Email/PD/TG/webhook
                │                                └─► live aggregates ─► api (WS push) │
                │ query ◄── store reads ◄────────────────────────────────────────────│
                │ api: REST (/api/v1) · WS (/live/ws) · /metrics · /healthz · static UI │
                │ reports ─► CSV/PDF exports          license ─► tier entitlements   │
                └─────────────────────────────── meta store (SQLite/Postgres) ────────┘
```

One Go binary (`server/cmd/pulse`), role-splittable via `--role` for large installs.
Default deployment is all-in-one + ClickHouse via Docker Compose.

## 3. Key boundaries (the rules agents must not break)

1. **Contracts first.** All shapes in `contracts/` (OpenAPI, event schemas, DDL).
   Implementation follows contracts, never the other way around.
2. **AMS isolation.** Only `server/pkg/amsclient` and `server/internal/collector/*`
   parse AMS wire formats. Everything downstream consumes normalized `domain` types.
   This is what makes the Phase 3 Wowza/Red5/Flussonic expansion a collector swap,
   not a rewrite.
3. **Two stores, strict split.** ClickHouse = events and rollups (high volume,
   append-only). Meta store (SQLite/Postgres) = config and small relational state.
   Metrics never go in the meta store; config never goes in ClickHouse.
4. **API layer is thin.** `internal/api` does HTTP/auth/transport; business logic in
   `query`, `alert`, `reports`. The web UI consumes only the public API — no
   private endpoints, so the customer-facing Data API (F8) gets parity for free.
5. **Beacon ingest is hostile-input territory.** Token auth, rate limits, size caps,
   schema validation. It is the only internet-facing surface.
6. **Free tier must stay cheap.** 2-vCPU sidecar budget drives defaults: sampling,
   batch sizes, ClickHouse low-footprint tuning.

## 4. Performance budgets (from PRD acceptance criteria)

| Budget | Source |
|---|---|
| New stream on dashboard ≤ 10 s after publish | F1 |
| Viewer counts within ±2% of AMS REST | F1 |
| Dashboard < 2 s load at 500 concurrent streams | F1 |
| 13-month rollup queries < 3 s | F2 |
| Beacon SDK < 15 KB gzipped, < 1% player CPU | F3 |
| Ingest degradation visible ≤ 15 s | F4 |
| Alert detection→notification < 30 s | F5 |
| Monthly statement generation < 60 s, ±1% reconciliation | F6 |
| New cluster nodes auto-discovered ≤ 2 min | F7 |
| ~1–2 GB ClickHouse per 1M viewer-sessions | §7.10 |

These are CI-verifiable targets; QA-01 owns regression checks against them.

## 5. Technology choices

See ADRs: [0001 tech stack](adr/0001-tech-stack.md),
[0002 ClickHouse](adr/0002-storage-clickhouse.md),
[0003 single binary](adr/0003-single-binary.md).

## 6. Security posture

- All API access token-authenticated; beacon ingest uses separate revocable tokens.
- AMS credentials and channel secrets encrypted at rest in the meta store.
- IP anonymization switch for GDPR/KVKK postures (geo degrades to country).
- License check fails open for reading already-collected data, fails closed for
  tier-gated features; Free tier requires no key and no phone-home.
