# Pulse MVP — Implementation Log

Self-hosted analytics, QoE monitoring and alerting for Ant Media Server.
Specification: `prd-report.md` §7. This log records, per feature, **what was built,
what went wrong, and how it was resolved** — written by the orchestrator (ORCH-00)
from empirical verification on the current `main`, not from agent self-reports.

> **How this was built.** A multi-agent wave plan (`agents/manifest.yaml`):
> Wave 1 (collector, live dashboard, core alerting, installer), Wave 2 (beacons,
> QoE, reports, fleet, API/Prometheus, Helm), Wave 3-MVP (anomaly detection,
> synthetic probes), then a **validation phase** (mission item 2) that adversarially
> re-verified F1–F10 against the PRD and drove a defect-fix loop. Full chronology in
> `DEVLOG.md`; all rulings in `agents/handoffs/decisions.md` (D-001…D-022).

## Verification status (current `main`)

| Component | Result |
|---|---|
| `server` — `CGO_ENABLED=0 go build/vet ./...` | clean |
| `server` — `go test ./...` (22 packages) | **0 failures** |
| `server` — `go test -tags integration` (reports, query, CH-backed) | pass |
| `web` — `npm run build && lint && test` | **157/157** tests, tsc strict clean |
| `sdk/beacon-js` — `build && test && size` | **65/65** tests, **3.52 KB** gzip (budget 15 KB) |
| Wave-1 / Wave-2 / Wave-3 / Wave-3-Plus gate scripts | pass (PASS_WITH_LIMITATIONS: D-002, D-007.5) |

> **Wave 3-Plus (post-MVP, D-018/D-019, 2026-06-15).** Tech-debt & accuracy closeout:
> probe segment-TTFB + master-playlist variant bitrate (GAP-3-001/003), anomaly
> epsilon floor (GAP-3-004), Kafka lag/parse-errors in `/healthz` (VD-27), **true
> windowed `peak_concurrency`** via new `rollup_concurrency_1d` (VD-38), and test
> coverage (VD-18/19/24/26/31/41). Migrations `0002_concurrency_rollup.sql` +
> `0003_probe_segment_ttfb.sql`. Independently re-verified on HEAD by ORCH-00.

> **W1 CI/CD (session 4, D-020, 2026-06-15).** Always-on GitHub Actions that gate `main`:
> `ci.yml` (7 jobs — contracts, server [go 1.25, CGO=0 build + `-race` tests + CH-service
> `pulse migrate` smoke + integration], web [`npm ci --legacy-peer-deps`, drift guard, HARD
> lint/test], sdk [size gate + HARD lint/test], docker-build, helm, compose), `e2e.yml`
> (compose-up smoke: seed mock-ams → authed `/live/overview` viewers>0) + `deploy/docker-compose.ci.yml`,
> `release.yml` (GHCR on `v*`), and `.github/branch-protection.sh`. Every job reproduced in
> the real CI images locally; e2e re-run by ORCH (viewers=13). Gate CLOSED
> (PASS_WITH_LIMITATIONS — Actions-green-on-push + branch protection are the user's GitHub-side
> step; `gh` not installed on the VPS).

> **Live-dashboard deadlock fix (session 4, D-021, 2026-06-15).** Restoring the demo
> surfaced a real **AB→BA deadlock** between the live aggregator (`a.mu`) and
> `cluster.Discovery` (`d.mu`): both emitted to the fan-out sink while holding their own
> lock and re-entered the other (root-caused from a SIGQUIT goroutine dump — 486 HTTP
> handlers wedged on `CurrentSnapshot`). Fix: collect events under the lock, emit after
> releasing it (`Discovery.poll`, `aggregator.EvictStale`) + regression tests that
> deadlock on the un-fixed source. Demo redeployed → `/healthz` 200, `status:ok`.

> **W2 productionize subset (session 4, D-022, 2026-06-15).** Hardened the public surface
> (no external infra): `deploy/docker-compose.hardened.yml` (Caddy TLS termination +
> ClickHouse auth + pulse off all host ports + secrets-from-env), `deploy/config/Caddyfile`,
> `deploy/.env.example`, `deploy/docker-compose.real-ams.yml`, `docs/runbooks/productionize.md`.
> Adversarially verified against a live `base+hardened` stack: HTTPS 200 (TLSv1.3, Caddy local
> CA), CH auth enforced (wrong-password → Code 516, `default` user removed), migrate exit 0 on
> the authenticated DSN, pulse with zero host ports. Gate CLOSED (PASS_WITH_LIMITATIONS).
> Waived to operator infra: Let's-Encrypt public TLS + real AMS; **`amsclient` real-wire-format
> fixture hardening deferred** to a future session.

**Single unified project** (one repo, one `pulse` binary `serve|migrate|diag` +
one web app + one SDK). No separate codebases to merge.

## Numeric acceptance criteria (PRD / ARCHITECTURE §4) — measured

| Budget | Target | Measured | Where |
|---|---|---|---|
| Stream visible on dashboard | ≤ 10 s | ~1.05 s | wave-1 gate |
| Viewer count accuracy (standalone) | ±2 % | 0 % | wave-1 gate |
| Alert detect → notify | < 30 s | ≤ 15 s fake-clock + ~0.2 s wall-clock (VD-31) | F5 |
| Ingest degradation visible | ≤ 15 s | ~250 µs in-proc | F4 |
| New node discovered | ≤ 2 min | ~24 ms (≤30 s prod) | F7 |
| 13-month rollup query | < 3 s | ~126 ms simple / ~145 ms dimensional (VD-18) | F2 |
| Billing reconciliation | ±1 % | 0.0000 % | F6 (live CH) |
| Statement generation | < 60 s | ~5 ms (seeded month) | F6 |
| SDK bundle | < 15 KB gzip | 3.52 KB | F3 |
| Anomaly false alarms | < 1 / node-week | 0.2594 / node-week | F9 |

---

## Features

### F1 — Real-time live dashboard + collector — **Functional**
**Built:** AMS collectors (REST poller, analytics-log tail, Kafka source) →
`normalize` → live aggregator → WebSocket + `/live/*` API → React dashboard
(virtualized streams table, ≤20 DOM nodes at 500 rows).
**Issues & resolutions:**
- *D-W1-001* node CPU/mem normalized 100× too high (corrupts fleet health/alerts) →
  fixed in the wave-1 fix-loop; regression test pins `cpuUsage=15 → 15.0`.
- *VD-02* the WebSocket broadcast LiveSnapshot, not LiveOverview, so
  `total_publishers`/`protocol_mix`/`apps` went stale after connect → V3b: WS now
  serializes `LiveOverview`; guard test on the wire shape.
- *VD-03* edge/origin viewer double-counting (`IsEdgeStream()` always false) → V3a:
  `EdgeStreamChecker` implemented; aggregator dedups edge counts.
**Known limitations:** dashboard `<2 s @500` verified by virtualization/DOM-row
proxy, not render-time (VD-04, Phase-3); cluster dedup logic untested against a real
multi-node AMS (D-002).

### F2 — Audience analytics (geo / device / protocol) — **Functional**
**Built:** MaxMind-format `.mmdb` geo reader (anonymize-IP zeroes octets *before*
lookup/storage), embedded UA parser, time-series + 13-month rollups, geo & device
breakdown endpoints, CSV export.
**Issues & resolutions:**
- *VD-06* geo/device breakdown handlers were **stubs returning `[]`** → V3a:
  real `GeoBreakdown`/`DeviceBreakdown` queries (`GROUP BY geo_country` /
  `client_device`); integration tests assert non-empty rows.
- *VD-07* `geoResolver`/`uaParser` were built but never passed to the REST poller →
  V3a: wired into `restpoller.Config`.
- *VD-08* beacon events discarded the client IP/UA (never enriched) → V3a:
  `batchToDomain` extracts `X-Forwarded-For`/`RemoteAddr` + UA and enriches.
- *VD-17* the geo test fixture produced an invalid mmdb (test skipped) → V3a: valid
  fixture; `TestGeo_MMDBFixture` now asserts real country lookups.
**Known limitations:** the AMS REST stats API has no per-viewer IP, so REST-path geo
relies on the beacon path (VD-16, architectural); GeoLite2 DB is user-supplied
(D-007.4); the 13-month *dimensional* query budget was measured on a simple
aggregate (VD-18, Phase-3).

### F3 — QoE beacons (SDK + ingest + QoE surface) — **Functional**
**Built:** `@pulse/beacon-js` (3.52 KB gzip, zero deps): one-line init, WebRTC
`getStats` adapter, hls.js adapter, `<video>` adapter, CMCD-aligned events, batching
(`sendBeacon` + keepalive fallback, localStorage spill, backoff), sampling, graceful
no-op. Hardened ingest (`/ingest/beacon`): hashed+constant-time token auth, rate
limit, 64 KB body cap, schema validation, CORS. QoE summary/timeline API.
**This was the most-broken feature in validation — all blockers now fixed:**
- *VD-09 (critical)* the SDK sent `X-Pulse-Token` but the server requires
  `X-Pulse-Ingest-Token` → **every real browser beacon 401'd**. V3a: header
  corrected; a guard test asserts the exact header string so it can't drift.
- *VD-10 (critical)* the default single-port `/ingest/beacon` decoded then **silently
  discarded** events (no `EventSink`) → V3a: `EventSink` wired into the API server;
  `TestVD10_BeaconPOST_PersistsToSink` proves persistence; 64 KB cap enforced.
- *VD-11* `/qoe/summary` was a live-snapshot proxy (`startup_p50_ms` always 0) →
  V3a: real `rollup_qoe_1h` query; correct `bitrate_kbps_p50` field.
- *VD-12* the hls.js adapter never emitted `rebuffer_end` (unbounded open stalls) →
  V3a: emits on `FRAG_BUFFERED` after a stall.
- *VD-13* level-switch events stored `0→0` kbps → V3a: read from `hls.levels[]`.
- *VD-15* beacon ingest wasn't tier-gated → V3b: requires Pro+.
**Known limitations:** player CPU `<1 %` budget not benchmarked (VD-14; ARCHITECTURE
§4 marks "not measurable" for MVP).

### F4 — Ingest / publisher health — **Functional**
**Built:** per-publisher bitrate/fps/keyframe/packet-loss/jitter extraction,
documented health-score formula, drop detection, `/qoe/ingest` API + UI.
**Issues & resolutions:**
- *VD-20 (critical)* `UpdateIngestHealth()` had **zero callers** → `HealthScore`
  was always 0 and the F4 dashboard was dark → V3a: `ComputeHealthScore()` called
  inline in `onIngestStats()`; guard test asserts `HealthScore > 0`.
- *VD-21 (critical)* the API never returned `timeseries`/`drop_events` (UI charts
  blank) → V3a: timeseries query added; both keys populated per the OpenAPI schema.
- *VD-22* the REST poller never emitted `EventIngestStats` (fps/keyframe/loss zero in
  REST-only deployments) → V3a: `NormalizeBroadcast` emits them.
- *VD-23* the `IngestTracker` interface had a mismatched `Snapshot()` type and was
  never wired → V3a: type fixed; `SetIngestTracker` wired in `serve.go`.
**Known limitations:** none outstanding. *Closed in Wave 3-Plus:* Kafka
`Lag()`/`ParseErrors()` now surfaced as a `/healthz` `kafka` component (VD-27 —
`Lag()` was also a dead counter, now reads `r.Stats().Lag` atomically); the ingest
endpoint test-coverage gaps are filled (VD-24 seeded-CH timeseries test; VD-26
IngestPage UI test).

### F5 — Alerting (rules, channels, scheduling) — **Functional**
**Built:** threshold + QoE/ingest/cert/node rule types; channels email, Slack,
Telegram, PagerDuty, generic webhook (HMAC-SHA256); cron maintenance windows;
`enabled` vs `muted` semantics; default rule pack.
**Issues & resolutions:**
- *VD-28* `muted=true` was **dead code** — notifications fired anyway (the default
  pack ships muted) → V3b: `if rule.Muted { return }` guard in fire/resolve; test
  asserts zero deliveries for a muted rule that fires.
- *VD-29* `group_by` was stored but never read — no storm grouping → V3b:
  `applyGroupBy` collapses N streams in a group to one notification.
- *VD-30* `node_down` fired on a CPU>95 proxy, never for genuinely absent nodes →
  V3b: `LastSeenAt` + `EvictStaleNodes` + absence-based firing.
- *VD-32* rebuffer/error alert metrics were `(1−HealthScore)` heuristics → V3b: real
  `rollup_qoe_1h` queries.
- *VD-33* cron ranges (`1-5`) silently truncated to the first value → V3b: range sets.
- *Tier (D-014)*: PagerDuty/webhook channels corrected to **Business** tier.
**Known limitations:** none outstanding. *Closed in Wave 3-Plus:* the `<30 s`
detect→notify budget is now also proven by a **real wall-clock test** (VD-31 —
`TestEvaluator_DetectAndNotify_WallClockBudget` drives the real `Start()` goroutine
+ ticker → notify path end-to-end, measured ~0.2 s), not only the fake-clock construction bound.

### F6 — Usage / billing reports — **Functional**
**Built:** viewer-minutes / egress / recording per app/stream/tenant (egress method
disclosed per row), tenant mapping (glob + meta-tag, precedence, "unassigned"
fallback) **with CRUD**, monthly/range statements (CSV + pure-Go PDF), cron-scheduled
generation, S3 (SigV4) export, `pulse diag --reconcile`.
**Issues & resolutions:**
- *D-W2-002 (major)* `accounting.go` queried non-existent ClickHouse columns and the
  wrong rollup table → `GET /reports/usage` 500'd and reconcile failed on the **live**
  stack (a unit test masked it by bypassing ClickHouse). Wave-2 fix-loop: source from
  `rollup_usage_1d`; added `TestAccountant_CHIntegration` exercising the real path —
  **reconcile drift 0.0000 %**.
- *D-010* the tenant CRUD endpoints were missing from the frozen contract → validation
  V1: approved CR, INT-01 amended the spec, BE-02 implemented routes, FE-01 built the
  UI; live per-tenant reconcile drift 0.0000 %.
- *VD-35* report endpoints were **ungated** (free tier got 200) → V3b: Business-tier
  gate on all report handlers; Free/Pro → 403.
- *VD-36* the UI's 5-field cron presets fell back to monthly (server parser accepted
  only 2–3 fields) → V3b: server parser extended to 5-field cron.
**Known limitations:** egress uses the bitrate×watch-time model until AMS
delivered-byte counters are wired. *Closed in Wave 3-Plus:* `peak_concurrency` is now
a **true windowed maximum** — new `rollup_concurrency_1d` (AggregatingMergeTree) takes
`maxState(viewer_count)` from `server_events` (AMS's authoritative instantaneous
concurrent count) per day, and billing reads `maxMerge` over the range; verified
`TestAccountant_CHIntegration` peak alpha=25/beta=5 from overlapping snapshots, not the
old session-count proxy (VD-38).

### F7 — Multi-node fleet — **Functional**
**Built:** periodic cluster-node discovery, origin/edge roles, per-node load,
node up/down → alert events, aggregate dedup rule.
**Issues & resolutions:**
- *VD-39* `FleetNodes()` hardcoded `role='standalone'` → V3b: real role from cluster
  discovery (`NodeRole()` wired into the query service).
- *VD-40* the node `version` field was always empty → V3a: plumbed
  `ClusterNodeDTO → NodeInfo → FleetNode`.
- *VD-03* edge/origin viewer dedup (shared with F1) → V3a.
**Known limitations:** full multi-node behavior unverified against a real AMS cluster
(D-002); dedup correctness covered by unit tests only.

### F8 — Public API + Prometheus — **Functional**
**Built:** read-only REST API over rollups (generated OpenAPI types end-to-end);
`/metrics` exposition (gauges/counters, bounded cardinality — app/node labels only,
never stream/session); optional metrics token; Grafana starter panels.
**Issues & resolutions:**
- *VD-S1 (security)* the metrics token used non-constant-time `!=` (timing oracle) →
  V3b: `subtle.ConstantTimeCompare`.
- *Tier (D-014)*: API tokens + Prometheus corrected to **Business** tier.
**Known limitations:** `/metrics` does a synchronous SQLite scan (cosmetic, Phase-3).

### F9 — Anomaly detection — **Functional (MVP, D-001)**
**Built:** Welford rolling baselines (mean/stddev/sample-count) per
`(metric, scope, window)` in the meta store; z-score flags computed on read with a
min-sample gate + hysteresis; `GET /anomalies`. Default sensitivity (σ=4,
min-samples=30, hysteresis=10) yields **0.2594 false alarms/node-week** vs the PRD
`<1` target. Enterprise tier.
**Known limitations:** the false-alarm rate is a modeled bound, not a long-run
simulation. *Closed in Wave 3-Plus:* a perfectly-constant metric (σ=0) now CAN flag —
`ComputeFlags` applies an on-read epsilon floor `effStddev = max(stddev, 0.05·|mean|,
1e-9)` (a coefficient-of-variation floor, metric-agnostic), so a real jump from a
constant baseline flags while the stored Welford state and the 0.2594/node-week
false-alarm bound are unchanged (GAP-3-004).

### F10 — Synthetic viewer probes — **Functional (MVP, D-001)**
**Built:** single in-process probe runner — HLS probes (manifest + first-segment
fetch; real TTFB/bitrate/success), honest minimal handling for webrtc/rtmp/dash;
`probe_results` in ClickHouse; `/probes` CRUD + `/probes/{id}/results`; UI with
**clear synthetic-vs-organic labeling** (4 levels). Pro+ tier.
**Known limitations:** single runner, not a distributed probe network (Phase-3).
*Closed in Wave 3-Plus:* a real **first-segment TTFB** is now measured and surfaced as
`segment_ttfb_ms` end-to-end (domain→CH col→API→UI; GAP-3-001), and a probe pointed at
a **master playlist now follows the first variant** to a media segment and reports real
bitrate (one level of indirection; master→master = parse error; GAP-3-003).

---

## Cross-cutting work

**Tier / licensing model (D-014, the biggest validation finding).** The PRD §7.11
defines **four** tiers (Free / Pro / Business / Enterprise) but the implementation
shipped only three — "Business" was missing, so ~half the paid features (F5
PagerDuty/webhook, F6 reports/multi-tenant, F8 API/Prometheus) were mis-gated to
Enterprise and the UI said "requires Business tier" while gating on `enterprise`.
Fixed across the stack: `business` added to the contract enum and `License.tier`;
entitlement matrix re-mapped to §7.11; every gate call site updated (VD-01/35/15);
UI upsell copy/logic corrected; per-tier matrix tests.

**Security hardening (V3b).** VD-S1 constant-time metrics-token compare; VD-S2
removed WebSocket `InsecureSkipVerify` (explicit allowed origins); VD-S3 bearer
middleware enforces token *kind* (ingest tokens can't reach admin/API routes).
Beacon ingest is treated as hostile input throughout (auth, rate-limit, size cap,
strict schema validation, never echoes tokens). Secrets at rest are AES-256-GCM; the
dev key is gitignored.

**Architecture boundaries (verified).** AMS wire formats stay in
`pkg/amsclient` + `internal/collector`; metrics in ClickHouse and config in the meta
store are never crossed; the web UI consumes only generated public-API types.

---

## Validation summary (mission item 2)

An adversarial sweep (14 verifiers) found **41 defects, 11 MVP-blocking** that the
per-wave gates had missed — because those gates tested with workarounds (manually-set
auth headers, unit tests that bypass ClickHouse, tautological assertions). The most
serious were whole **broken flows** behind green tests: F3 beacons 401'd in real
browsers, F2 analytics were stubs, F4 health was always 0, F5 muted/group_by were
dead code, F6 reports were ungated. All P0/P1 + the P2 majors/security/contract
items were fixed in the V3 fix-loop and re-verified (`agents/handoffs/validation/`).

**Process note (D-013, D-017):** the QA agent twice produced "remaining/carried
defect" lists that echoed earlier triage descriptions for defects already fixed
(once via a stale binary, once by declining to re-verify). Both were caught and
empirically disproven by re-running each guard test on HEAD. The feature status in
*this* log is therefore built from ORCH-00's own test runs, not agent self-reports.

---

## Known limitations & Phase-3 backlog (MVP-acceptable, documented)

**Environment waivers (this build machine):**
- **D-002 — no Docker.** Docker Compose, Dockerfile, and the Helm chart are authored
  and lint/`helm template`-validated but not executed; end-to-end runs on a local
  process stack (`pulse` + single-binary ClickHouse + a mock AMS).
- **D-007.5 — no Kafka broker.** The Kafka collector is unit/fake-tested; real AMS
  Kafka E2E is deferred to the version-matrix CI.

**Closed in Wave 3-Plus (D-018/D-019, 2026-06-15):** dimensional 13-month query
measurement (VD-18); Kafka lag in `/healthz` (VD-27); wall-clock alert-latency test
(VD-31); true windowed `peak_concurrency` (VD-38); anomaly epsilon-floor (GAP-3-004);
probe segment-TTFB + master-playlist variant-bitrate (GAP-3-001/003); added test
coverage (VD-19/24/26/41; VD-34 was closed in V3b).

**Still deferred to Phase 3 (not MVP-blocking):** headless render-time budget (VD-04)
and player-CPU benchmark (VD-14) — both need a real browser profiler (Playwright/CDP),
not measurable in jsdom/vitest; long-run false-alarm simulation; real multi-node cluster
verification + edge-dedup E2E and a distributed probe network (both need multiple AMS
nodes, D-002); and the PRD's own Phase-3 platform scope (mobile beacons, SSO, white-label
PDF, air-gapped licensing, hosted option).

---

## Where to look

- Chronology: `DEVLOG.md` · Rulings: `agents/handoffs/decisions.md` (D-001…D-017)
- Validation detail: `agents/handoffs/validation/` (V2 triage + per-area findings + V3 reports)
- Per-wave QA gates: `qa/wave-1|2|3/gate-report.md`
- Architecture & budgets: `docs/ARCHITECTURE.md` §3–4 · Contracts: `contracts/`
