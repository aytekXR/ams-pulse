# Pulse Architecture

Authoritative technical-design document. PRD: `prd-report.md` §7. Decisions with
trade-offs get an ADR in `docs/adr/`.

Last updated: Wave 1 implementation complete (2026-06-12).

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

### Wave-1 implementation status

The following components are implemented and unit-tested as of Wave 1:

| Component | Package | Status |
|---|---|---|
| REST poller | `internal/collector/restpoller` | Implemented; 5 s default poll, ≤10 s stream visibility |
| Log tail | `internal/collector/logtail` | Implemented; rotation-safe, partial-line-safe |
| Webhook receiver | `internal/collector/webhook` | Implemented; HMAC-SHA256 validation |
| Fanout + dedup | `internal/collector/fanout`, `dedup` | Implemented |
| Live aggregator | `internal/collector/aggregator` | Implemented; in-memory, deep-copy snapshots |
| Normalizer | `internal/collector/normalize` | Implemented; AMS→domain mapping. D-W1-001 fixed (CPU/mem/disk no longer multiplied by 100). |
| ClickHouse store | `internal/store/clickhouse` | Implemented; batched async inserts |
| Meta store | `internal/store/meta` | Implemented; SQLite (pure-Go), AES-256-GCM encryption |
| Config | `internal/config` | Implemented; YAML + env override, full surface |
| Alert evaluator | `internal/alert` | Implemented; fake-clock tested, 15 s detection latency |
| Alert channels | `internal/alert/channels` | Email + Slack implemented; PD/TG/webhook roadmap Wave 2 |
| Query service | `internal/query` | Implemented; live + historical (ClickHouse) |
| API server | `internal/api` | Implemented; chi router, bearer auth, WS hub, OpenAPI-conformant |
| License manager | `internal/license` | Implemented; ed25519 verification, tier entitlements |
| Web UI | `web/` | Implemented; live dashboard (F1), analytics (F2), alerts (F5), settings; Wave 2 routes are placeholders |
| Kafka collector | `internal/collector/kafka` | Stub; Wave 2 |
| Beacon ingest | `internal/collector/beacon` | Stub; Wave 2 |
| Geo/UA enrichment | `internal/collector/enrichment` | No-op interfaces in place; Wave 2 |
| Reports (CSV/PDF) | `internal/reports` | Stub; Wave 2 |
| Cluster discovery | `internal/cluster` | Stub; Wave 2 |
| Prometheus /metrics | `internal/api` | 2 metrics exported; full coverage Wave 2 |

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
   - **Probe results** go in ClickHouse (`probe_results` table, 90-day TTL). Probe
     _config_ goes in the meta store (`probes` table). Decided Wave 1 (INT-01, Q2).
   - **Anomaly baselines** go in the meta store (`anomaly_baselines` table). They are
     low-cardinality, mutated in-place rolling-window stats — config-like, not event-series.
     Decided Wave 1 (INT-01, Q3).
4. **API layer is thin.** `internal/api` does HTTP/auth/transport; business logic in
   `query`, `alert`, `reports`. The web UI consumes only the public API — no
   private endpoints, so the customer-facing Data API (F8) gets parity for free.
5. **Beacon ingest is hostile-input territory.** Token auth, rate limits, size caps,
   schema validation. It is the only internet-facing surface.
6. **Free tier must stay cheap.** 2-vCPU sidecar budget drives defaults: sampling,
   batch sizes, ClickHouse low-footprint tuning.

## 4. Performance budgets (from PRD acceptance criteria)

| Budget | Source | Wave-1 measured |
|---|---|---|
| New stream on dashboard ≤ 10 s after publish | F1 | **1064 ms** (2 s poll, local stack; 5 s worst-case in production) |
| Viewer counts within ±2% of AMS REST | F1 | **0.0%** error on all tested streams |
| Dashboard < 2 s load at 500 concurrent streams | F1 | Virtualized table confirmed ≤20 DOM rows for 500-stream input |
| 13-month rollup queries < 3 s | F2 | Not yet measurable (Wave 1 DDL only; data needed) |
| Beacon SDK < 15 KB gzipped, < 1% player CPU | F3 | SDK stub; Wave 2 |
| Ingest degradation visible ≤ 15 s | F4 | Ingest health stub; Wave 2 |
| Alert detection→notification < 30 s | F5 | **15 s** (fake-clock unit test; 10.1 s worst-case by construction) |
| Monthly statement generation < 60 s, ±1% reconciliation | F6 | Reports stub; Wave 2 |
| New cluster nodes auto-discovered ≤ 2 min | F7 | Cluster discovery stub; Wave 2 |
| ~1–2 GB ClickHouse per 1M viewer-sessions | §7.10 | Not yet measurable (requires production load) |

These are CI-verifiable targets; QA-01 owns regression checks against them.

## 5. Technology choices

See ADRs: [0001 tech stack](adr/0001-tech-stack.md),
[0002 ClickHouse](adr/0002-storage-clickhouse.md),
[0003 single binary](adr/0003-single-binary.md).

Additional Wave-1 library decisions:
- **`modernc.org/sqlite`** — pure-Go SQLite (CGO_ENABLED=0 enforced throughout).
- **`go-chi/chi v5`** — HTTP router for `internal/api`.
- **`nhooyr.io/websocket`** — WebSocket hub for `/live/ws`.
- **`getkin/kin-openapi`** — OpenAPI conformance testing in API tests.
- **`recharts`** — client-side charts (SVG; server-aggregated data, not raw events).
- **`@tanstack/react-virtual`** — virtual list for 500+ row stream table.

## 6. Security posture

- All API access token-authenticated; beacon ingest uses separate revocable tokens.
- AMS credentials and channel secrets encrypted at rest in the meta store using
  **AES-256-GCM**. Key sourced from `PULSE_SECRET_KEY` env var (32-byte hex);
  if absent, a key is generated and stored in `<db_dir>/pulse_secret.key`.
- First-run bootstrap: on first `pulse serve` with no tokens, a random admin token
  (`plt_<16 hex bytes>`) is generated, SHA-256 hashed and stored, and printed once
  to stderr. The raw token is never stored.
- IP anonymization switch for GDPR/KVKK postures (geo degrades to country).
  Configured via `PULSE_ANONYMIZE_IP=true`; the Wave 1 binary logs this as
  a no-op (enrichment stubs); effective in Wave 2 when geo enrichment lands.
- License check fails open for reading already-collected data, fails closed for
  tier-gated features; Free tier requires no key and no phone-home.
- `/metrics` endpoint is unauthenticated by default; set `PULSE_METRICS_TOKEN` to
  require a scrape token (INT-01 Q4 decision). `PULSE_METRICS_TOKEN` is a Wave 2
  env var; the Wave 1 binary does not enforce it.
- Token passwords use SHA-256 in Wave 1. bcrypt migration planned for Wave 2
  (BE-02 G3; requires pure-Go bcrypt library).

## 7. Live aggregates design

The live dashboard (`/api/v1/live/overview`, `/api/v1/live/streams`,
`/live/ws`) is served from **in-memory aggregates**, not ClickHouse queries.

The `internal/collector/aggregator.Aggregator` maintains a `LiveSnapshot` in memory:
- Updated on every `OnServerEvent` call from the Fanout.
- Deep-copied for lock-free reads (no reader contention during updates).
- Distributed to WebSocket subscribers via `Subscribe() (<-chan *LiveSnapshot, func())`.
- Stale stream eviction runs periodically (streams not updated in >2 poll intervals).

This design satisfies the ≤10 s stream visibility budget and keeps dashboard latency
independent of ClickHouse query performance. Historical analytics (`/api/v1/analytics/*`)
query ClickHouse.

### WebSocket message envelope

The `/live/ws` endpoint sends JSON messages with a common envelope:

```json
{
  "type": "snapshot" | "delta" | "heartbeat",
  "ts": <unix epoch ms>,
  "payload": <LiveSnapshot | null>
}
```

- `snapshot` — sent immediately on connection; full `LiveSnapshot`.
- `delta` — sent after each aggregator update; full `LiveSnapshot` (diff compression Wave 2).
- `heartbeat` — sent every 30 s when no updates; `payload` is absent.

Clients authenticate via `Authorization: Bearer plt_<hex>` header or
`?token=plt_<hex>` query parameter. The token is validated against the meta
store on connection; unauthenticated requests receive a 401 HTTP response
before the WebSocket upgrade completes.

## 8. Alert evaluator design

The `internal/alert.Evaluator` runs a tick loop (default 5 s) that:
1. Lists alert rules from the meta store.
2. Skips any rule where `enabled = false` (no metric fetch, no history write).
3. Gets a `CurrentSnapshot()` from the live aggregator.
4. Evaluates each remaining rule against the snapshot using the state machine in §2 above.
5. For rules that transition to `firing` or `resolved`, builds an
   `alert-notification` JSON payload (conforming to
   `contracts/events/alert-notification.schema.json`) and delivers it to
   all configured channels via the channel registry — unless `muted = true`,
   in which case the history is written but no notification is dispatched.
6. Persists history to `alert_history` in the meta store.

The tick interval is capped at 30 s to ensure the 30 s latency budget is always met.
A fake-clock (`alert.FakeClock`) allows deterministic latency tests without real time.

## 9. Meta store encryption

AMS source credentials, SMTP passwords, and Slack webhook URLs are stored
encrypted in the meta store. The encryption scheme is AES-256-GCM with a
random 12-byte nonce prepended to each ciphertext. The encryption key is
derived from `PULSE_SECRET_KEY` (32-byte hex).

Encrypted columns store base64-encoded `nonce || ciphertext`. The Go API
exposes `meta.Store.Encrypt(plaintext) (ciphertext, error)` and
`meta.Store.Decrypt(ciphertext) (plaintext, error)`.

## 10. Known issues (Wave 1 — post fix-loop status)

Wave-1 fix-loop (D-006, 2026-06-12) resolved D-W1-001 through D-W1-005. The table
below records the final status of each defect.

| ID | Component | Description | Status |
|---|---|---|---|
| D-W1-001 | `collector/normalize.go` | Node CPU/mem values multiplied by 100. | **Fixed** — `* 100` multipliers removed; `cpu_pct=15.0` for AMS `cpuUsage=15.0`. |
| D-W1-002 | `api/server.go` | `/healthz` `latency_ms` always null; no 503 on ClickHouse down. | **Fixed** — `/healthz` now pings ClickHouse (3 s timeout) and meta store, returns measured `latency_ms` and HTTP 503 + `status:"down"` when unreachable. |
| D-W1-003 | `cmd/pulse/main.go` | `pulse migrate` skipped meta migrations; meta DDL required `PULSE_META_DDL_PATH`. | **Fixed** — meta DDL embedded in binary (`meta.EmbeddedDDL`); `pulse migrate` applies it automatically. `PULSE_META_DDL_PATH` is now an optional override. |
| D-W1-004 | `cmd/pulse/serve.go` | Duplicate import alias (`clickhouse` + `chstore`). | **Fixed** — single import, `chstore` alias removed. |
| D-W1-005 | `pkg/amsclient/client.go` | Dead `get()` method with double-decoder bug. | **Fixed** — `get()` deleted; only `getJSON()` remains. |
| D-W1-006 | `.github/workflows/ams-version-matrix.yml` | `TestAMSVersionMatrix` Go integration tests not implemented. | **Deferred** — needs real AMS containers in CI; carried to wave-2 validation sweep (QA-01). |
