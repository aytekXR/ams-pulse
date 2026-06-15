# Pulse Architecture

Authoritative technical-design document. PRD: `prd-report.md` §7. Decisions with
trade-offs get an ADR in `docs/adr/`.

Last updated: V3b fix-loop complete (2026-06-15). QA gate: PASS_WITH_LIMITATIONS.

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

### V3b fix-loop implementation status (2026-06-15)

Last updated: V3b fix-loop complete (2026-06-15). QA gate: **PASS_WITH_LIMITATIONS**
(waivers D-002 no-Docker, D-007.5 no-Kafka; 0 FAIL defects; all guard tests green).

The V3a and V3b fix-loops resolved 30 defects from the adversarial validation (V2
triage report). Key functional changes:

| Area | Fixes applied |
|---|---|
| Beacon round-trip (F3) | SDK sends `X-Pulse-Ingest-Token` (VD-09); main-port `/ingest/beacon` persists to EventSink with 64 KB cap (VD-10); beacon events geo/UA enriched from HTTP request (VD-08); Pro+ tier gate enforced (VD-15) |
| Geo / device analytics (F2) | `/analytics/geo` and `/analytics/devices` now query `viewer_sessions` — real rows returned (VD-06); geoResolver + uaParser wired into REST poller (VD-07); geo/UA enrichment working end-to-end |
| QoE summary (F3/F4) | `/qoe/summary` queries `rollup_qoe_1h`; `startup_p50_ms` non-zero (VD-11); bitrate field renamed to `bitrate_kbps_p50` per spec |
| Ingest health (F4) | `LiveStream.HealthScore` non-zero — `ComputeHealthScore()` called inline in `onIngestStats()` (VD-20); REST poller emits `EventIngestStats` (VD-22); `/qoe/ingest` returns `health_score` on 0–100 scale; `timeseries` and `drop_events` keys present in response (VD-21, empty when ClickHouse not seeded — D-002 waiver); `IngestTracker` interface `Snapshot()` type fixed + `SetIngestTracker` wired (VD-23, V3a) |
| Alerting (F5) | `muted=true` suppresses notifications (VD-28); `group_by` collapses N streams to 1 notification per group (VD-29); `node_down` fires on node absence, not CPU proxy (VD-30); cron range syntax `1-5` parsed as set (VD-33); maintenance windows work correctly; 5-field cron accepted (VD-36) |
| Reports / billing (F6) | All 5 report endpoints gated to Business+ tier (VD-35); 5-field cron schedule presets work (VD-36); `egress_method` label correct when bytes branch taken (VD-37) |
| Cluster / fleet (F7) | `IsEdgeStream()` implemented — edge/origin viewer dedup active (VD-03); `FleetNodes()` returns real role from cluster discovery (VD-39); node `version` field populated end-to-end (VD-40) |
| Tier model | 4-tier enum `free\|pro\|business\|enterprise` enforced in Go + OpenAPI (VD-01); Business tier gates: reports, PagerDuty/webhook channels, multi-tenant, beacon ingest (Pro+) |
| Live WS | `/live/ws` broadcasts `LiveOverview` shape (`total_publishers`, `protocol_mix`, `apps`) instead of `LiveSnapshot` (VD-02) |
| Security | Metrics token uses `subtle.ConstantTimeCompare` (VD-S1); WS uses `OriginPatterns` not `InsecureSkipVerify` (VD-S2); bearer middleware rejects ingest tokens on API routes (VD-S3) |
| SDK | `rebuffer_end` emitted from `HlsAdapter` (VD-12); `from_kbps`/`to_kbps` populated from `hls.levels[]` (VD-13) |
| Geo MMDB | `BuildTestMMDB()` produces valid binary; `TestGeo_MMDBFixture` passes with real lookups (VD-17) |

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
| Alert evaluator | `internal/alert` | **Shipped** — 15 s detection latency; cert_expiry, node_up/down, ingest_bitrate_floor added Wave 2; `muted` suppression and `group_by` grouping fixed V3b |
| Alert channels | `internal/alert/channels` | **Shipped** — Email, Slack, Telegram (Pro+); PagerDuty, Webhook (Business+, V3b); HMAC signature on webhook |
| Query service | `internal/query` | **Shipped** — live + historical (ClickHouse); QoE + fleet endpoints Wave 2; geo/device breakdown (VD-06 V3a), QoE rollup queries (VD-11 V3a), ingest timeseries (VD-21 V3a) |
| API server | `internal/api` | **Shipped** — 32 paths, 46 ops; /metrics, /qoe/*, /fleet/nodes, /reports/* added Wave 2; report tier gates, WS LiveOverview, token kind enforcement added V3b |
| License manager | `internal/license` | **Shipped** — ed25519 verification; 4-tier model (free/pro/business/enterprise) per PRD §7.11; CheckReports, CheckBeaconIngest, CheckMultiTenant, CheckPrometheus added V3b |
| Web UI | `web/` | **Shipped** — F1–F8; 150 tests green (V3b); tier gate logic updated for 4-tier model |
| Beacon SDK | `sdk/beacon-js/` | **Shipped** (F3) — 3.52 KB gzip, 65 tests green, MIT license; header fix (VD-09), `rebuffer_end` (VD-12), bitrate levels (VD-13) applied V3a |
| Beacon ingest | `internal/collector/beacon` | **Shipped** (F3) — token auth, rate limit, 64 KB body cap, schema validation; Pro+ tier gate (VD-15 V3b); geo/UA enrichment from HTTP request (VD-08 V3a) |
| Kafka collector | `internal/collector/kafka` | **Shipped** — pure-Go kafka-go; 8 contract tests; D-007.5 no-broker limitation |
| Geo/UA enrichment | `internal/collector/enrichment` | **Shipped** — MMDBGeoResolver, EmbeddedUAParser, AnonymizeIP; absent DB = no-op; MMDB test fixture valid (VD-17 V3a) |
| Session stitcher | `internal/collector/sessions` | **Shipped** — viewer join/heartbeat/leave stitching; 5 tests |
| Ingest health | `internal/collector/ingest` | **Shipped** (F4) — health score formula, 141 µs detection; `HealthScore` non-zero from REST events (VD-20 V3a); ingest timeseries returned by API (VD-21 V3a) |
| Reports (CSV/PDF) | `internal/reports` | **Shipped** (F6) — accounting, tenant mapping, statement gen, scheduler, S3 uploader; 5-field cron support (VD-36 V3b); Business+ tier gate (VD-35 V3b) |
| Cluster discovery | `internal/cluster` | **Shipped** (F7) — 30 s poll, new node visible ≤30 s; `IsEdgeStream()` implemented (VD-03 V3a); node version field (VD-40 V3a) |
| Prometheus /metrics | `internal/api` | **Shipped** (F8) — 5 metrics, bounded cardinality; scrape token uses `subtle.ConstantTimeCompare` (VD-S1 V3b) |
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
V3b fix-loop re-gate: PASS_WITH_LIMITATIONS (2026-06-15).

| Budget | Source | Wave-1 | Wave-2 measured | Wave-3 / V3b measured |
|---|---|---|---|---|
| New stream on dashboard ≤ 10 s after publish | F1 | **1064 ms** | **1.50 s** (B-01) | Unchanged |
| Viewer counts within ±2% of AMS REST (standalone) | F1 | **0.0%** | **0.0%** (B-02 runtime; VD-05 runtime test added V3b) | Unchanged |
| Viewer counts within ±2% of AMS REST (cluster) | F1 | Not tested | Not tested | **0% double-count**: `IsEdgeStream()` implemented; edge viewer dedup active (VD-03 V3a) |
| Dashboard < 2 s load at 500 concurrent streams | F1 | Virtualized, ≤20 DOM rows | ≤20 DOM rows (C-W2-02) | Unchanged — render time not measured; see known limitations |
| 13-month rollup queries < 3 s | F2 | DDL only | **126 ms** simple aggregate (C-W2-08) | **150 ms** with dimensional GROUP BY (VD-18 addressed in V3b gate) |
| Beacon SDK < 15 KB gzip | F3 | Stub | **3.44 KB** gzip (C-W2-03) | **3.52 KB** (no regression; VD-09/12/13 fixes added code) |
| Beacon SDK < 1% player CPU | F3 | Not measurable | Not measurable | Not measurable (deferred; VD-14) |
| Beacon round-trip accepted | F3 | Broken | Broken (VD-09/10) | **Fixed V3a** — header correct, main-port persists to EventSink |
| Geo analytics non-empty rows | F2 | Stub | Stub | **Fixed V3a** — real ClickHouse query; `startup_p50_ms=250.0` from `rollup_qoe_1h` |
| QoE `startup_p50_ms` non-zero | F3 | Never set | Always 0 | **250.0 ms** measured (VD-11 V3a; `TestQuery_QoeSummary_RealStartupP50`) |
| Ingest health_score non-zero | F4 | Not wired | Always 0 (VD-20) | **95** (0–100 scale) for healthy ingest (VD-20/20b V3a) |
| Ingest degradation visible ≤ 15 s | F4 | Stub | **250.8 µs** in-process (C-W2-06) | Unchanged |
| Alert detection→notification < 30 s | F5 | **15 s** | **15 s** (B-03) | Unchanged; `muted` and `group_by` now functional (V3b) |
| Monthly statement generation < 60 s | F6 | Stub | **4.8 ms** (C-W2-05) | Unchanged |
| Billing reconciliation ≤ ±1% | F6 | Stub | **0.0000%** drift (n=10,000) | Unchanged |
| New cluster nodes auto-discovered ≤ 2 min | F7 | Stub | **24.4 ms** (C-W2-07) | Unchanged |
| ~1–2 GB ClickHouse per 1M viewer-sessions | §7.10 | Not measurable | Not measurable | Not measurable |
| F9 false-alarm rate < 1/node-week | F9 | — | — | **0.259/node-week** (σ=4.0, hysteresis=10; `TestAnomaly_FalseAlarmRate_ModeledTarget`) |
| F10 HLS probe: success, TTFB > 0, bitrate > 0 | F10 | — | — | **success=true, ttfb_ms=1, bitrate_kbps=66.7** (`TestHLSProbe_Success`) |
| F10 probe new config → first result latency | F10 | — | — | **< 100 ms** (After(0) fires immediately; fake clock) |
| Web build bundle (regression) | — | — | **773.85 kB** (221.69 kB gzip) | **773.85 kB** (no regression) |
| Web tests pass | — | — | 58 tests | **150 tests** (V3b — 11 suites) |
| Server tests pass | — | — | 17 packages | **22 packages** (V3b full suite) |
| SDK tests pass | — | — | 56 tests | **65 tests** (V3b — 5 files) |

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

- All API access token-authenticated; beacon ingest uses separate revocable ingest tokens.
- **Token kind enforcement (V3b VD-S3):** Bearer middleware rejects ingest tokens
  (`kind='ingest'`) on `/api/v1/*` admin routes — returns 403 WRONG_TOKEN_KIND.
  Admin routes require `kind='api'`; beacon routes require `kind='ingest'`.
- AMS credentials and channel secrets encrypted at rest in the meta store using
  **AES-256-GCM**. Key sourced from `PULSE_SECRET_KEY` env var (32-byte hex);
  if absent, a key is generated and stored in `<db_dir>/pulse_secret.key`.
- First-run bootstrap: on first `pulse serve` with no tokens, a random admin token
  (`plt_<16 hex bytes>`) is generated, SHA-256 hashed and stored, and printed once
  to stderr. The raw token is never stored.
- IP anonymization switch for GDPR/KVKK postures (geo degrades to country).
  Configured via `PULSE_ANONYMIZE_IP=true`. Effective in Wave 2+ (geo enrichment
  implemented); beacon path extracts client IP from `X-Forwarded-For` / `RemoteAddr`.
- License check fails open for reading already-collected data, fails closed for
  tier-gated features; Free tier requires no key and no phone-home.
- `/metrics` endpoint: set `PULSE_METRICS_TOKEN` to require a scrape token.
  The token comparison uses `subtle.ConstantTimeCompare` (VD-S1 V3b — timing oracle fixed).
- WebSocket `/live/ws`: cross-origin policy enforced via `AllowedWSOrigins` config;
  `InsecureSkipVerify` removed (VD-S2 V3b). Configure `PULSE_ALLOWED_WS_ORIGINS` for
  non-same-origin dashboard deployments.
- Token passwords use SHA-256. bcrypt migration is a Phase-3 roadmap item.
- **Beacon ingest body cap:** 64 KB (authoritative; both hardened handler and
  main-port handler enforce this limit; VD-S4 / VD-10 V3a).

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
  "payload": <LiveOverview | null>
}
```

- `snapshot` — sent immediately on connection; `LiveOverview` shape (includes `total_publishers`, `protocol_mix`, `apps`).
- `delta` — sent after each aggregator update; `LiveOverview` shape (VD-02 V3b: changed from raw `LiveSnapshot` to match OpenAPI spec).
- `heartbeat` — sent every 30 s when no updates; `payload` is absent.

The payload shape is `LiveOverview` (not `LiveSnapshot`). `LiveOverview` includes the
`total_publishers`, `protocol_mix`, and `apps` fields that the live dashboard depends on.
Raw `LiveSnapshot` data (per-stream detail) is available via `GET /api/v1/live/streams`.

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
   in which case the history is written but no notification is dispatched (VD-28
   V3b: `muted` suppression is now enforced; `DefaultRulePack` ships all rules as
   `muted=true` so no notifications fire until channels are configured and rules
   are explicitly unmuted).
6. When `group_by` is set on a rule (e.g. `group_by: "app"`), collapses multiple
   matching streams into a single notification per unique group key value (VD-29 V3b).
   For `group_by="app"`, N streams in the same app produce 1 notification, not N.
7. Persists history to `alert_history` in the meta store.

The tick interval is capped at 30 s to ensure the 30 s latency budget is always met.
A fake-clock (`alert.FakeClock`) allows deterministic latency tests without real time.

**`node_down` behavior:** Fires when a cluster node is **absent** from the live
snapshot (not seen within `3 × PollInterval`). The prior CPU-proxy heuristic (`CPU > 95`)
was incorrect and replaced in VD-30 V3b. Nodes are evicted from the snapshot via
`EvictStaleNodes(threshold)` analogous to stream eviction.

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

### Wave-2 gaps (post V3b fix-loop status)

| ID | Description | Owner | Status |
|---|---|---|---|
| GAP-2-001 | BuildTestMMDB produces invalid mmdb format; `TestGeo_MMDBFixture` skipped | BE-01 | **Fixed V3a** — valid MMDB binary; real country lookups verified (VD-17) |
| GAP-2-002 | `cluster.Discovery.IsEdgeStream()` always returns false; edge/origin dedup not implemented | BE-01 | **Fixed V3a** — IsEdgeStream() implemented; aggregator edge dedup active (VD-03) |
| GAP-2-003 | Kafka `Lag()` / `ParseErrors()` not surfaced in `/healthz` component detail | BE-02 | **Open** — Wave-3 Phase-3 roadmap (VD-27) |
| GAP-2-004 | Pro tier beacon write gating not API-enforced (fails-open for any valid ingest token) | BE-02 | **Fixed V3b** — `CheckBeaconIngest()` enforces Pro+ (VD-15) |
| GAP-2-005 | `/qoe/summary` QoE data is live-snapshot proxy, not from `rollup_qoe_1h` | BE-02 | **Fixed V3a** — queries `rollup_qoe_1h`; `startup_p50_ms` non-zero (VD-11) |
| GAP-206-01 | Helm chart image `ghcr.io/pulse-analytics/pulse:0.1.0` not yet published | INFRA-01 | Open — Phase-3 roadmap |
| GAP-206-02 | Postgres Secret `pulse-postgres-secret` must be created manually before Helm install | DOC-01 (documented in install runbook) | — |
| GAP-206-03 | Helm `busybox:1.36` initContainer image unpinned | INFRA-01 | Open — Phase-3 roadmap |

### Wave-3 gaps (post V3b fix-loop status)

| ID | Description | Owner | Status |
|---|---|---|---|
| GAP-3-001 | HLS TTFB is manifest TTFB only; segment TTFB not stored separately (single `ttfb_ms` column in frozen DDL — schema CR needed for Phase 3) | BE-01 | Open — Phase-3 |
| GAP-3-003 | Master HLS playlist probe: `success=true, bitrate_kbps=0` — correct behavior but Phase 3 should follow first variant URL | BE-01 | Open — Phase-3 |
| GAP-3-004 | Zero-stddev blind spot: perfectly constant metric streams produce stddev=0, preventing z-score computation. Phase 3: epsilon floor | BE-02 | Open — Phase-3 |
| GAP-3-005 | `GET /probes/{id}/results` returns empty list when ClickHouse is unavailable (correct behavior; full round-trip requires integration tag) | BE-02 | Open — Phase-3 |
| GAP-3-006 | Pro tier license test gap: only Enterprise key tested for probe entitlement; Pro-tier key test needs a dev license | BE-02 | Open — Phase-3 |

### V3b remaining known limitations (P2/P3 deferred items)

The following V2 adversarial triage defects remain open after the V3a/V3b fix-loops.
Items marked CLOSED were fixed during the fix-loops and are included for traceability only.

| VD | Description | Severity | Status |
|---|---|---|---|
| VD-04 | Dashboard render time at 500 streams not measured — ≤20 DOM rows proxy only | Minor | OPEN — headless-browser measurement requires Phase-3 Playwright setup |
| VD-05 | Viewer count ±2% (standalone): runtime test added V3b; cluster double-count: edge dedup active (VD-03 V3a) | Minor | CLOSED |
| VD-12 | `HlsAdapter` `rebuffer_end` emission — unbounded open stalls in ClickHouse | Major | CLOSED — Fixed V3a (SDK-01) |
| VD-13 | `HlsAdapter` bitrate levels from `hls.levels[]` — ABR switches always 0→0 kbps | Minor | CLOSED — Fixed V3a (SDK-01) |
| VD-14 | Player CPU <1% budget — no test or measurement | Minor | OPEN — deferred; ARCHITECTURE §4 marks "not measurable" for MVP |
| VD-18 | 13-month query budget (GROUP BY dimensional query) | Minor | CLOSED — 150 ms measured in V3b gate (budget 3 s) |
| VD-24 | No integration tests for GET /api/v1/qoe/ingest with seeded data | Minor | OPEN — requires real ClickHouse (D-002 waiver); Phase-3 |
| VD-25 | keyframe formula package comment contradicted code; dead `keyframeBadS` constant | Minor | CLOSED — Fixed V3a (code comment corrected) |
| VD-26 | No frontend tests for IngestPage | Minor | OPEN — Phase-3 roadmap |
| VD-27 | Kafka `Lag()`/`ParseErrors()` not in `/healthz` | Minor | OPEN — Phase-3 roadmap (GAP-2-003) |
| VD-31 | 30 s detection budget measured with fake clock only | Minor | OPEN — formal bound documented: tick≤5 s + poll≤5 s + channel<5 s = ≤15 s ≤ 30 s budget |
| VD-38 | `peak_concurrency` in rollup = session count, not true concurrent peak | Minor | OPEN — Phase-3 schema change needed (SummingMergeTree → maxState) |
| VD-23 | `api.IngestTracker` interface has wrong `Snapshot()` return type; `SetIngestTracker` never called | Major | **CLOSED — Fixed V3a** (`782c166`): `Snapshot()` returns `map[string]ingest.PublisherState`; `SetIngestTracker` wired in `serve.go`; `TestVD23_IngestTracker_InterfaceConformance` passes (D-017) |
| VD-X3-A | POST /admin/sources/{id}/test missing `reachable` field in response | Major | **CLOSED — Fixed V3a** (`782c166`): `handleTestSource` returns `reachable:bool` in all paths; `TestContract_AmsSourceStatus_HandlerReachableField` un-skipped and passing (D-017) |
| VD-X3-B | Frontend sent `granularity` param; server expects `interval` | Minor | CLOSED — Fixed V3b (FE-01 — param renamed to `interval`) |
| VD-X3-C | DELETE idempotent-204 vs spec 404 | Minor | CLOSED — Fixed V3b (INT-01 — spec updated to document idempotent-204) |
| VD-X3-D | OpenAPI spec missing 403 for GET /anomalies | Minor | CLOSED — Fixed V3b (INT-01 — 403 response added to spec) |
