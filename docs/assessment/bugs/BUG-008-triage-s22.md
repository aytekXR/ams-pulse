# BUG-008 Triage — S22 Assessment

**Assessor:** BUG-008 ASSESSOR (SESSION-22 WO-C)
**Date:** 2026-07-12
**Source bug:** docs/assessment/bugs/BUG-008-anomalies-filter-params-silently-dropped.md
**Decision:** split-S23 — partial fix in S22 (4 of 6 params), architectural redesign deferred

---

## 1. Architecture recap

`GET /anomalies` calls `anomalyDetector.ComputeFlags(ctx, sigmaThreshold)`. The concrete
implementation (`server/internal/anomaly/anomaly.go:348`) is **point-in-time only**:

1. `d.live.CurrentSnapshot()` — single live snapshot; no history, no time series.
2. `d.store.ListAnomalyBaselines(ctx)` — loads ALL rows from `anomaly_baselines` (no WHERE
   clause; no entity or time argument accepted).
3. Iterates baselines, computes z-scores against the current snapshot, emits `[]AnomalyFlag`.
4. Flags are **ephemeral** — computed fresh on each request and never persisted anywhere.

The handler (`wave3.go:27-74`) post-filters on `metric` but reads none of the remaining six
declared params (`from`, `to`, `app`, `stream`, `limit`, `cursor`).

The bridge (`serve.go:63-88`) wraps `*anomaly.Detector` and passes the call through unchanged.
No entity or time information crosses the interface at any layer.

---

## 2. Per-param effort classification

### Group A — handler-only (no service layer change required): `app`, `stream`, `limit`, `cursor`

**`app` and `stream`**

`AnomalyBaselineRow.Scope` is a JSON string encoding `{node_id, app, stream_id}`.
`parseScopeJSON` (anomaly.go:485) already parses it into `domain.AlertScope`.
`AnomalyFlagAPI.Scope` already carries `App` and `StreamID` fields.

After `ComputeFlags` returns, the handler already has a `for _, f := range flags` loop
(wave3.go:61-64) that post-filters on `metric`. Extending that loop to also check
`f.Scope.App` and `f.Scope.StreamID` against `?app` and `?stream` query params is a
handler-only change — identical pattern, no signature change, no new store method.
Infrastructure is already present.

Effort: **S** (< 1 h, handler loop extension only).

**`limit` and `cursor`**

Flags are a `[]AnomalyFlagAPI` slice in memory after `ComputeFlags` returns. Deterministic
ordering (by `TS` then `ID`) followed by a slice-window is pure handler logic. Cursor is
encoded as a plain decimal integer offset over the sorted in-memory slice — the simplest
honest choice for an ephemeral point-in-time list (no keyset cursor needed when the backing
data has no stable storage key). See wave3.go `handleAnomalies` comment for the design note.

Effort: **S** (< 2 h, handler-only, no new store methods).

### Group B — architectural gap (new persistent store required): `from` and `to`

A time-range query (`?from=T1&to=T2`) implies "return flags that were detected between T1 and
T2". The current detector has no flag history: every call generates a fresh point-in-time
snapshot; emitted flags are never written to disk. There is no table to query.

An honest implementation requires:

1. **New schema**: A `anomaly_flag_events` table (ClickHouse recommended for time-series
   retention; meta/SQLite possible for low-volume but non-standard per ARCHITECTURE §3.3 which
   reserves ClickHouse for metrics and meta for config). Schema: `(id, metric, scope, observed,
   expected, sigma, detected_at_ms)`.
2. **Write path**: Each tick that calls `UpdateBaselines` (or a new background writer) must
   persist any emitted flags to `anomaly_flag_events`. This is a non-trivial change: the write
   path must handle deduplication (hysteresis ticks already suppress re-fires in memory, but a
   persistent store needs its own dedup key), retention policy, and ClickHouse batching.
3. **Read path**: `ComputeFlags` (or a new `QueryFlagHistory`) must accept `from`/`to` and
   query `anomaly_flag_events` by `detected_at_ms` range.
4. **Migration**: Schema migration in the meta or ClickHouse setup path.

This is a cross-cutting change touching anomaly.go, server.go (interface), serve.go (bridge),
the store layer, and a new migration. It is not achievable within the S22 handler-scope budget
without creating an unreviewed architectural shortcut.

**Why no 501 guard in S22 (ORCH DECISION)**

The triage doc originally recommended returning HTTP 501 Not Implemented for `?from`/`?to` as
a "cheap honesty" option. ORCH rejected this for S22:

- A 501 response on a declared param is a **behavior change** (currently the handler silently
  ignores `?from`/`?to` and returns a full unfiltered snapshot; a 501 changes the status code
  that existing callers observe).
- Before shipping a behavior change on a production endpoint, a **UI-caller audit** is required
  to confirm no caller currently sends `?from`/`?to` and treats a non-error response as
  authoritative for decisions.
- **Audit result (S22)**: `web/src/api/client.ts` declares `from?` and `to?` in the
  `anomaliesApi.list` parameter type, but `web/src/features/anomalies/AnomaliesPage.tsx` calls
  `anomaliesApi.list({ min_sigma: minSigma, limit: 100 })` — it does **not** pass `from` or
  `to` in any code path. No web UI caller currently sends these params.
- Despite the absence of current web callers, the registry entry must remain `known-violation`
  (not `501 probe`) until S23 designs the full fix. A 501 guard committed without a full design
  decision and ADR could constrain the S23 implementation unnecessarily.

---

## 3. Delivered scope (S22 / D-084)

### S22 — what was delivered

| Param | Fix type | Registry disposition |
|---|---|---|
| `app` | handler post-filter (extend existing loop, check `f.Scope.App`) | `known-violation` → `probe` |
| `stream` | handler post-filter (extend existing loop, check `f.Scope.StreamID`) | `known-violation` → `probe` |
| `limit` | handler slice-window after sort (OpenAPI default 50, max 500) | `known-violation` → `probe` |
| `cursor` | decimal integer offset cursor; invalid cursor → first page | `known-violation` → `probe` |
| `from` | not delivered — architectural gap; no 501 guard (ORCH decision, see §2) | `known-violation` retained |
| `to` | not delivered — same reason | `known-violation` retained |

Conformance test census after S22/D-084:

- **Probes**: 35 (up from 29; +4 anomalies Group A, +2 BUG-007 cursor probes promoted from exempt)
- **Known-violations**: 4 (down from 8; BUG-008 ?from, BUG-008 ?to, BUG-009 ?tenant ×2)
- **Exempt**: 47 (down from 49; the two BUG-007 cursor exempts became probes)

### S23 — architectural work deferred

Design and implement the `anomaly_flag_events` persistence layer:

- **Storage decision**: ClickHouse `anomaly_flag_events` table with TTL (same retention as
  other metrics per PRD); or meta/SQLite for low-cardinality deployments with a configurable
  backend. Needs an ADR.
- **Write path**: Background goroutine (or inline in `UpdateBaselines`) writes each emitted
  `AnomalyFlag` to the store; dedup by `(metric, scope, detected_at_bucket)` to avoid
  duplicate rows from hysteresis-suppressed re-fires.
- **Interface change**: `ComputeFlags` or a new `QueryFlagHistory(ctx, from, to, app, stream,
  limit, cursor)` method; bridge updated; handler routes to appropriate method based on
  `?from`/`?to` presence.
- **Known-violation removal**: Once the flag-event store is live, flip the `?from` and `?to`
  registry entries from `known-violation` to `probe` asserting real filtering behavior.
- **UI-caller audit**: The S22 audit confirmed the web UI does not currently pass `from`/`to`.
  S23 should re-audit any new callers before shipping the full fix.
- **Prerequisite**: S23 must include a capacity estimate for flag-event volume (number of
  flagged metrics per tick × tick frequency × retention days) to confirm ClickHouse is the
  right backend.

---

## 4. Registry consequence summary

After S22/D-084 (Group A delivered, Group B deferred):

| Registry key | S21 disposition | S22 disposition | Notes |
|---|---|---|---|
| `GET /anomalies ?from` | `known-violation` (BUG-008) | `known-violation` (BUG-008) | no 501 guard; S23 designs flag-event store |
| `GET /anomalies ?to` | `known-violation` (BUG-008) | `known-violation` (BUG-008) | same; web UI does not currently pass ?to |
| `GET /anomalies ?app` | `known-violation` (BUG-008) | `probe` | real filter on scope.App; response differential via fakeAnomalyDetector |
| `GET /anomalies ?stream` | `known-violation` (BUG-008) | `probe` | real filter on scope.StreamID |
| `GET /anomalies ?limit` | `known-violation` (BUG-008) | `probe` | real slice-window; next_cursor emitted when more items remain |
| `GET /anomalies ?cursor` | `known-violation` (BUG-008) | `probe` | decimal-offset cursor; page 1 ≠ page 2 differential |

Two BUG-008 `known-violation` entries remain (?from, ?to). The other four are closed.

---

## 5. Effort call

| Scope | Effort | Justification |
|---|---|---|
| S22 partial fix (app, stream, limit, cursor) | **S** | Handler-only; no interface change; no new store methods; extends existing post-filter loop pattern |
| S23 full fix (from/to real flag-event store) | **L** | New ClickHouse/meta table, migration, write path, interface redesign, bridge update, retention policy, dedup logic |

**Overall BUG-008 classification: split-S23.**

The S22 partial fix (Group A) is complete. The S23 architectural work is Large and requires
dedicated scoping, an ADR for storage backend selection, and at minimum one full session for
design before implementation begins.
