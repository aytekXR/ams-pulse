# Pulse Architecture

Authoritative technical-design document. PRD: `prd-report.md` §7. Decisions with
trade-offs get an ADR in `docs/adr/`.

Last updated: Wave 3-MVP implementation complete (2026-06-14).

## 1. System context

Pulse is a **read-only sidecar** to Ant Media Server. It never modifies AMS, needs no
inbound access to AMS beyond the existing REST port, and stores all data on the
customer's infrastructure. That "data never leaves" property is the product's core
differentiator (PRD §7.1) — any design that ships customer data to us (telemetry,
crash reporting) must be opt-in and documented.

## 2. Components

```
                ┌──────────────────────────── pulse binary ───────────────────────────────────────┐
AMS REST ──────►│ collector/restpoller ─┐                                                         │
AMS log ───────►│ collector/logtail ────┤                                                         │
AMS Kafka ─────►│ collector/kafka ──────┼─► normalize ─► store/clickhouse (events)                │
AMS webhooks ──►│ collector/webhook ────┤        │                                                │
Player beacons ►│ collector/beacon ─────┤        ├─► sessions.Stitcher ─► viewer_sessions (CH)   │
  (:8091 ingest)│                       │        ├─► ingest.HealthTracker (health score, F4)      │
                │ cluster/discovery ────┘        ├─► alert/evaluator ─► channels ──────────────────► Slack/Email/PD/TG/webhook
                │  (fleet nodes, F7)             └─► live aggregates ─► api (WS push)             │
                │ prober.Runner (F10) ──────────────────────────────────────────────────────────► probe_results (CH, 90-day TTL)
                │  (synthetic probes; 60 s refresh; 4-worker pool; HLS full / others minimal)     │
                │ anomaly.Detector tick (F9) ───────────────────────────────────────────────────► anomaly_baselines (meta)
                │  (Welford baselines; σ=4.0; minSamples=30; hysteresis=10; 0.259 FA/node-week)   │
                │ query ◄── store reads ◄─────────────────────────────────────────────────────────│
                │ api: REST (/api/v1) · WS (/live/ws) · /metrics · /healthz · static UI           │
                │ reports/scheduler ─► CSV/PDF exports (F6) · S3 upload                           │
                │ license ─► tier entitlements                                                     │
                └──────────────────────────── meta store (SQLite/Postgres) ────────────────────────┘
```

One Go binary (`server/cmd/pulse`), role-splittable via `--role` for large installs.
Default deployment is all-in-one + ClickHouse via Docker Compose.

### Wave-3 implementation status (2026-06-14)

Last updated: Wave 3-MVP complete (2026-06-14). QA gate: **PASS_WITH_LIMITATIONS**
(two D-002 waivers; no FAIL defects; see `qa/wave-3/gate-report.md`).

| Component | Package | Status |
|---|---|---|
| Probe runner | `internal/prober` | **Shipped** (F10 MVP) — HLS full; webrtc/rtmp/dash minimal-honest; 4-worker pool; 60 s config refresh |
| Probe results store | `internal/store/clickhouse` | **Shipped** (F10) — `InsertProbeResult` + `QueryProbeResults`; 90-day TTL |
| Probe CRUD + API | `internal/api` | **Shipped** (F10) — `POST/GET/PUT/DELETE /probes`; `GET /probes/{id}/results`; Pro+ tier gate |
| Anomaly detector | `internal/anomaly` | **Shipped** (F9 MVP) — Welford online baselines; σ=4.0; 0.259 FA/node-week; `GET /anomalies`; Enterprise-only |
| Web UI — anomalies | `web/src/features/anomalies` | **Shipped** (F9) — flag table; sigma selector; severity badges; Enterprise gate |
| Web UI — probes | `web/src/features/probes` | **Shipped** (F10) — CRUD form; results panel with TTFB+bitrate charts; 4-level synthetic labeling; Pro+ gate |

Minimal-but-working scope (D-001):
- F9: 3 metrics (viewers, cpu_pct, mem_pct); 1-hour rolling window; on-read flag computation.
- F10: HLS probes fully implemented; webrtc/rtmp/dash are reachability-only stubs (`error_code=not_probed`).

Phase-3 deltas (not in MVP):
- Mobile beacons (F3 extension), SSO, white-label PDF, distributed probe network, multi-node edge dedup.
- F9: multi-window baselines (24h, 7d), additional metrics, flag persistence table.
- F10: native RTMP client, WHIP/WHEP WebRTC probing, DASH manifest parsing.

### Wave-2 implementation status

Last updated: Wave 2 implementation complete (2026-06-14).

| Component | Package | Status |
|---|---|---|
| REST poller | `internal/collector/restpoller` | **Shipped** — 5 s default poll, ≤10 s stream visibility |
| Log tail | `internal/collector/logtail` | **Shipped** — rotation-safe, partial-line-safe |
| Webhook receiver | `internal/collector/webhook` | **Shipped** — HMAC-SHA256 validation |
| Fanout + dedup | `internal/collector/fanout`, `dedup` | **Shipped** |
| Live aggregator | `internal/collector/aggregator` | **Shipped** — in-memory, deep-copy snapshots; wave-2 health fields added |
| Normalizer | `internal/collector/normalize` | **Shipped** — AMS→domain mapping; D-W1-001 fixed |
| ClickHouse store | `internal/store/clickhouse` | **Shipped** — batched async inserts; viewer_sessions + rollup_qoe_1h added Wave 2 |
| Meta store | `internal/store/meta` | **Shipped** — SQLite (pure-Go), AES-256-GCM; tenant + schedule CRUD added Wave 2 |
| Alert evaluator | `internal/alert` | **Shipped** — 15 s detection latency; cert_expiry, node_up/down, ingest_bitrate_floor added Wave 2 |
| Alert channels | `internal/alert/channels` | **Shipped** — Email, Slack, Telegram, PagerDuty, Webhook (Wave 2); HMAC signature on webhook |
| Query service | `internal/query` | **Shipped** — live + historical (ClickHouse); QoE + fleet endpoints Wave 2 |
| API server | `internal/api` | **Shipped** — 32 paths, 46 ops; /metrics, /qoe/*, /fleet/nodes, /reports/* added Wave 2 |
| License manager | `internal/license` | **Shipped** — ed25519 verification; pro/enterprise tier gating Wave 2 |
| Web UI | `web/` | **Shipped** — F1 live dashboard, F2 analytics, F3 QoE, F4 ingest health, F5 alerts, F6 reports, F7 fleet, F8 /metrics; 58 tests green |
| Beacon SDK | `sdk/beacon-js/` | **Shipped** (F3) — 3.44 KB gzip, 56 tests green, MIT license |
| Beacon ingest | `internal/collector/beacon` | **Shipped** (F3) — token auth, rate limit, body cap, schema validation |
| Kafka collector | `internal/collector/kafka` | **Shipped** — pure-Go kafka-go; 8 contract tests; D-007.5 no-broker limitation |
| Geo/UA enrichment | `internal/collector/enrichment` | **Shipped** — MMDBGeoResolver, EmbeddedUAParser, AnonymizeIP; absent DB = no-op |
| Session stitcher | `internal/collector/sessions` | **Shipped** — viewer join/heartbeat/leave stitching; 5 tests |
| Ingest health | `internal/collector/ingest` | **Shipped** (F4) — health score formula, 141 µs detection |
| Reports (CSV/PDF) | `internal/reports` | **Shipped** (F6) — accounting, tenant mapping, statement gen, scheduler, S3 uploader |
| Cluster discovery | `internal/cluster` | **Shipped** (F7) — 30 s poll, new node visible ≤30 s |
| Prometheus /metrics | `internal/api` | **Shipped** (F8) — 5 metrics, bounded cardinality; scrape token gate |
| Helm chart | `deploy/helm/pulse/` | **Shipped** (authored-unexecuted per D-002) — lint passes, 3 template variants |

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

All wave-2 budgets are QA-verified (QA-01 WO-207 gate report, 2026-06-14).
Wave-3 budgets QA-verified in WO-304 gate report (2026-06-14).

| Budget | Source | Wave-1 | Wave-2 measured | Wave-3 measured |
|---|---|---|---|---|
| New stream on dashboard ≤ 10 s after publish | F1 | **1064 ms** | **1.50 s** (B-01) | Unchanged |
| Viewer counts within ±2% of AMS REST | F1 | **0.0%** | **0.0%** (B-02) | Unchanged |
| Dashboard < 2 s load at 500 concurrent streams | F1 | Virtualized, ≤20 DOM rows | ≤20 DOM rows (C-W2-02) | Unchanged |
| 13-month rollup queries < 3 s | F2 | DDL only | **126 ms** (C-W2-08) | Unchanged |
| Beacon SDK < 15 KB gzip, < 1% player CPU | F3 | Stub | **3.44 KB** gzip (C-W2-03) | **3.44 KB** (no change) |
| Ingest degradation visible ≤ 15 s | F4 | Stub | **250.8 µs** in-process (C-W2-06) | Unchanged |
| Alert detection→notification < 30 s | F5 | **15 s** | **15 s** (B-03) | Unchanged |
| Monthly statement generation < 60 s | F6 | Stub | **4.8 ms** (C-W2-05) | Unchanged |
| Billing reconciliation ≤ ±1% | F6 | Stub | **0.0000%** drift (n=10,000) | Unchanged |
| New cluster nodes auto-discovered ≤ 2 min | F7 | Stub | **24.4 ms** (C-W2-07) | Unchanged |
| ~1–2 GB ClickHouse per 1M viewer-sessions | §7.10 | Not measurable | Not measurable | Not measurable |
| F9 false-alarm rate < 1/node-week | F9 | — | — | **0.259/node-week** (σ=4.0, hysteresis=10; `TestAnomaly_FalseAlarmRate_ModeledTarget`) |
| F10 HLS probe: success, TTFB > 0, bitrate > 0 | F10 | — | — | **success=true, ttfb_ms=1, bitrate_kbps=66.7** (`TestHLSProbe_Success`) |
| F10 probe new config → first result latency | F10 | — | — | **< 100 ms** (After(0) fires immediately; fake clock) |
| Web build bundle (regression) | — | — | **773.85 kB** (221.69 kB gzip) | **773.85 kB** (no regression) |
| Web tests pass | — | — | 58 tests | **109 tests** (51 new wave-3) |

These are CI-verifiable targets; QA-01 owns regression checks against them.
See `qa/budgets/run-budget-tests.sh` for the budget regression suite.

## 5. Technology choices

See ADRs: [0001 tech stack](adr/0001-tech-stack.md),
[0002 ClickHouse](adr/0002-storage-clickhouse.md),
[0003 single binary](adr/0003-single-binary.md),
[0007 anomaly detection algorithm](adr/0007-anomaly-detection-welford.md),
[0008 probe protocol coverage](adr/0008-probe-protocol-coverage.md).

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
- `delta` — sent after each aggregator update; full `LiveSnapshot` (diff compression: Phase-3 roadmap).
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

## 10. Ingest health score formula (F4)

The health score is computed per publisher from live ingest stats. It is the
authoritative formula used by BE-02's `/api/v1/qoe/ingest` response and the
FE ingest health dashboard.

```
score = 0.35*S_bitrate + 0.25*S_fps + 0.20*S_keyframe + 0.12*S_loss + 0.08*S_jitter

S_bitrate  = clamp(bitrate_kbps / target_kbps, 0, 1)       [target default: 2000]
S_fps      = clamp(fps / target_fps, 0, 1)                  [target default: 30]
S_keyframe = 1.0                         if keyframe_s <= 2.0
             clamp(2.0 / keyframe_s, 0,1) if keyframe_s >  2.0
S_loss     = clamp(1.0 - loss_pct/10.0, 0, 1)
S_jitter   = clamp(1.0 - jitter_ms/100.0, 0, 1)

Weight sum: 0.35+0.25+0.20+0.12+0.08 = 1.0 (verified by unit test)

Classification:
  score >= 0.80 -> Good
  score >= 0.50 -> Warning
  score <  0.50 -> Critical
  absent > sourceGoneTimeout (default 15 s) -> Offline
```

Authoritative Go implementation: `server/internal/collector/ingest/health.go:ComputeHealthScore`.

**Drop detection thresholds:**
- Bitrate floor breach: `S_bitrate < 0.5` (< 50% of target bitrate)
- FPS collapse: `fps < 5.0`
- Source gone: no `ingest_stats` event for > `SourceGoneTimeout` (default 15 s)

**Budget:** In-process detection is sub-millisecond (measured 141–250 µs).
Production worst-case with 5 s REST poll: ≤ 10 s (2 poll cycles) — well within
the 15 s F4 budget.

Configurable via: `PULSE_INGEST_TARGET_BITRATE_KBPS` (default 2000),
`PULSE_INGEST_TARGET_FPS` (default 30).

---

## 11. Known issues

### Wave-1 defects (post fix-loop status, D-006, 2026-06-12)

| ID | Component | Description | Status |
|---|---|---|---|
| D-W1-001 | `collector/normalize.go` | Node CPU/mem values multiplied by 100. | **Fixed** — `* 100` multipliers removed. |
| D-W1-002 | `api/server.go` | `/healthz` `latency_ms` always null; no 503 on ClickHouse down. | **Fixed** — `/healthz` pings CH + meta store, returns 503 on failure. |
| D-W1-003 | `cmd/pulse/main.go` | `pulse migrate` skipped meta migrations; meta DDL required external file. | **Fixed** — meta DDL embedded in binary; applied automatically. |
| D-W1-004 | `cmd/pulse/serve.go` | Duplicate import alias. | **Fixed** |
| D-W1-005 | `pkg/amsclient/client.go` | Dead `get()` method with double-decoder bug. | **Fixed** — `get()` deleted. |
| D-W1-006 | `.github/workflows/ams-version-matrix.yml` | `TestAMSVersionMatrix` Go integration tests not implemented. | **Fixed** (Wave 2, QA-01) — 3 mock profiles; real-container assertions documented for CI. |

### Wave-2 defects (post QA gate, WO-207, 2026-06-14)

| ID | Component | Description | Status |
|---|---|---|---|
| D-W2-001 | `qa/wave-1/run-gate.sh` | Alert rule POST missing `name` field — wave-1 gate script exits nonzero. | Open — QA-01 fix pending |
| D-W2-002 | `internal/reports/accounting.go` | Wrong ClickHouse column names (`watch_s_state`, `peak_viewers_state`, `bucket_ts`) — live billing broken; unit test passes. | Open — BE-02 fix pending. Fix: rename to `watch_time_s`, `peak_concurrency`, `bucket`. |
| D-W2-003 | `qa/wave-1/run-gate.sh` | Same as D-W2-001 (filed separately as regression). | Open — QA-01 fix pending |

### Wave-2 gaps (non-blocking)

| ID | Description | Owner | Wave |
|---|---|---|---|
| GAP-2-001 | BuildTestMMDB produces invalid mmdb format; `TestGeo_MMDBFixture` skipped | BE-01 | 3 |
| GAP-2-002 | `cluster.Discovery.IsEdgeStream()` always returns false; edge/origin dedup not implemented | BE-01 | 3 |
| GAP-2-003 | Kafka `Lag()` / `ParseErrors()` not surfaced in `/healthz` component detail | BE-02 | 3 |
| GAP-2-004 | Pro tier beacon write gating not API-enforced (fails-open for any valid ingest token) | BE-02 | 3 |
| GAP-2-005 | `/qoe/summary` QoE data is live-snapshot proxy, not from `rollup_qoe_1h` | BE-02 | 3 |
| GAP-206-01 | Helm chart image `ghcr.io/pulse-analytics/pulse:0.1.0` not yet published | INFRA-01 | 3 |
| GAP-206-02 | Postgres Secret `pulse-postgres-secret` must be created manually before Helm install | DOC-01 (documented in install runbook) | — |
| GAP-206-03 | Helm `busybox:1.36` initContainer image unpinned | INFRA-01 | 3 |

### Wave-3 gaps (non-blocking, from gate report WO-304)

| ID | Description | Owner | Phase |
|---|---|---|---|
| GAP-3-001 | HLS TTFB is manifest TTFB only; segment TTFB not stored separately (single `ttfb_ms` column in frozen DDL — schema CR needed for Phase 3) | BE-01 | 3 |
| GAP-3-003 | Master HLS playlist probe: `success=true, bitrate_kbps=0` — correct behavior but Phase 3 should follow first variant URL | BE-01 | 3 |
| GAP-3-004 | Zero-stddev blind spot: perfectly constant metric streams produce stddev=0, preventing z-score computation. Phase 3: epsilon floor | BE-02 | 3 |
| GAP-3-005 | `GET /probes/{id}/results` returns empty list when ClickHouse is unavailable (correct behavior; full round-trip requires integration tag) | BE-02 | 3 |
| GAP-3-006 | Pro tier license test gap: only Enterprise key tested for probe entitlement; Pro-tier key test needs a dev license | BE-02 | 3 |
