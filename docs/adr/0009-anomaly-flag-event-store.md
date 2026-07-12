# ADR 0009: Anomaly flag-event store — making GET /anomalies ?from/?to honest (BUG-008 phase 2)

**Status:** Proposed · **Date:** 2026-07-12 · **Effort:** L · **Bugs:** BUG-008 · **Triage:** docs/assessment/bugs/BUG-008-triage-s22.md

## Context

`GET /anomalies` is backed by `anomalyDetector.ComputeFlags` (server/internal/api/wave3.go:88),
which is point-in-time only: it calls `d.live.CurrentSnapshot()` (server/internal/anomaly/anomaly.go:353),
loads all Welford baseline rows (anomaly.go:358), and computes z-scores for the five monitored metrics
(`viewers`, `ingest_bitrate_kbps`, `cpu_pct`, `mem_pct`, `disk_pct`). Emitted `AnomalyFlag` values
each carry a fresh UUID and `TS = time.Now().UnixMilli()` at call time (anomaly.go:377,420) and are never
persisted anywhere. A time-range query (`?from=T1&to=T2`) physically cannot be answered without stored
history.

This gap was classified **BUG-008 Group B** in the S22 triage
(docs/assessment/bugs/BUG-008-triage-s22.md §2). Group A parameters (`?app`, `?stream`, `?limit`,
`?cursor`) were fixed in D-084 by extending the in-memory post-filter loop. Group B required a
persistent event store; that work was deferred to S23 with an explicit decision not to ship a
`501 Not Implemented` guard (UI-caller audit confirmed the web UI does not currently pass `?from`
or `?to`; BUG-008-triage-s22.md §2).

Two conformance registry entries remain `known-violation` as a direct consequence
(server/internal/api/param_conformance_test.go:927–939):

```go
"GET /anomalies ?from": {
    // from/to: architectural gap — no persistent flag-event store.
    // S23: design anomaly_flag_events table (ClickHouse recommended).
    // S22 DECISION: known-violation retained; no 501 guard (behavior change
    // requires UI-caller audit; web/src does NOT currently pass from/to).
    disp:   paramKnownViolation,
    bugRef: "BUG-008",
},
"GET /anomalies ?to": {
    // Same S22 decision as ?from above.
    disp:   paramKnownViolation,
    bugRef: "BUG-008",
},
```

Closing these requires (a) a persistent store that records flag events as they are detected, (b) a
write path that populates it, and (c) a read path that queries it when `?from` or `?to` is present.

## Decision

### 1. Storage backend: ClickHouse table `anomaly_flag_events`

Flag events are an append-only, time-ordered event series and must live in ClickHouse.
ARCHITECTURE.md §3.3 (docs/ARCHITECTURE.md:133–135) states:

> "Two stores, strict split. ClickHouse = events and rollups (high volume, append-only). Meta store
> (SQLite/Postgres) = config and small relational state. Metrics never go in the meta store; config
> never goes in ClickHouse."

The same section records the authoritative precedents (docs/ARCHITECTURE.md:136–140):

> "Probe results go in ClickHouse (probe_results table, 90-day TTL). Probe config goes in the meta
> store (probes table). Decided Wave 1 (INT-01, Q2)."
>
> "Anomaly baselines go in the meta store (anomaly_baselines table). They are low-cardinality, mutated
> in-place rolling-window stats — config-like, not event-series. Decided Wave 1 (INT-01, Q3)."

Baselines are config-like (mutable stats, low cardinality). Flag events are event-series data (immutable
records of detections, subject to TTL-based expiry). They follow the probe-results precedent, not the
baselines precedent.

**Capacity estimate (reproducing scout arithmetic, anomaly.go:26–44 package doc).**
Default parameters: σ=4.0, `hysteresisTicks`=10, `tickInterval`=60 s (anomaly.go:70–78; tickInterval default documented at anomaly.go:40, wired via serve.go).

- Raw false-alarm rate: P(|Z| ≥ 4.0) ≈ 6.33×10⁻⁵ per tick; 1440 ticks/day → ~0.0912/day/metric.
- Effective rate after hysteresis renewal suppression:
  λ_eff = λ_raw / (1 + λ_raw × hysteresisTicks) = 0.638/week / (1 + 0.638×10) ≈ 0.0864/week ≈ **0.012/day/metric**.
- Enterprise scale — 100 nodes (3 node-scoped metrics each) + 500 streams (2 stream-scoped metrics each):
  - Node-scoped: 300 metrics × 0.012/day ≈ 3.6/day
  - Stream-scoped: 1000 metrics × 0.012/day ≈ 12.0/day
  - **Total: ≈ 17 flag-events/day**
- At 90-day retention: 17 × 90 ≈ **1,530 rows** stored at any time.

1,530 rows is trivially low for ClickHouse. A flusher goroutine or channel-based batching adds
complexity with no benefit at this volume.

### 2. Schema

```sql
CREATE TABLE IF NOT EXISTS {db}.anomaly_flag_events (
    id          String,
    metric      LowCardinality(String),
    node_id     String,
    app         String,
    stream_id   String,
    scope       String,          -- raw JSON {node_id, app, stream_id}; byte-identical to hysteresisKey.scope
    observed    Float64,
    expected    Float64,
    sigma       Float64,
    detected_at DateTime64(3)    -- tick time (UTC); NOT time.Now() at ComputeFlags call
) ENGINE = MergeTree()
ORDER BY (detected_at, metric, scope)
TTL toDate(detected_at) + toIntervalDay({retention_days});
```

**Denormalized columns.** `node_id`, `app`, and `stream_id` are extracted from `scope` at write time
(via `parseScopeJSON`, anomaly.go:485) and stored as first-class columns. This allows `WHERE` filtering
without `JSONExtractString` in the read path, keeping query plans simple and index-friendly.

**`scope` raw column.** The `scope` column stores the raw JSON string produced by `scopeJSON()`
(anomaly.go:430–454), byte-identical to `hysteresisKey.scope` (anomaly.go:127). It is never produced
by re-serializing a parsed struct; see Risk-4 (Alternatives §C) for the rationale.

**`detected_at`.** `DateTime64(3)` (millisecond precision), matching the `probe_results` column shape.
Value = tick timestamp captured once per `UpdateBaselines` call, not `time.Now()` at request time; see
Consequences §1.

**TTL.** Follows `contracts/db/clickhouse/0006_probe_results_ttl.sql:17`:
`toDate(detected_at) + toIntervalDay({retention_days})`. The `{retention_days}` placeholder is
substituted by `runner.go` at migration apply time, inheriting the tier-configured retention
(PRD §7.11, prd-report.md:345: Free 7 days, Pro 90 days, Business 13 months).

### 3. Migration file

`contracts/db/clickhouse/0010_anomaly_flag_events.sql`

**Migration number collision note.** The last committed migration in the tree is
`0008_probe_webrtc_rtp_stats.sql`; the next available number is 0009. However, `0009_recording_mv.sql`
is claimed by the BUG-002 agent running concurrently in D-085 (this same session). The anomaly
flag-event migration therefore takes **0010** to avoid a file-level collision. A build session must
confirm that both `0009_recording_mv.sql` and `0010_anomaly_flag_events.sql` exist before applying
migrations.

### 4. Write path: shared detection helper inside `UpdateBaselines` tick

Flags are detected and written from `Detector.UpdateBaselines` (anomaly.go:213–340), which runs
exclusively on the single `time.NewTicker(d.tickInterval)` goroutine created in `Detector.Run`
(anomaly.go:190). `Run` is the only production caller; it is wired via
`go s.anomalyDetector.Run(ctx)` in `server.Start` (serve.go:604–606).

**Shared detection helper.** A private method `checkFlags(ctx context.Context, tickAt time.Time,
baselines []AnomalyBaselineRow, liveValues map[string]float64)` is extracted from the z-score loop
currently duplicated between `UpdateBaselines` and `ComputeFlags`. `UpdateBaselines` calls it after
its Welford loop (insertion point: anomaly.go:338). The helper:

1. Runs the z-score pass with the current `d.hysteresis` map.
2. Writes detected events to `d.flagStore` (if non-nil) with `detected_at = tickAt`.
3. Updates `d.hysteresis[hk] = d.hysteresisTicks` for each new flag.

`ComputeFlags` (anomaly.go:348–424) continues to call equivalent logic for its on-demand ephemeral
response, but does **not** write to `d.flagStore` (write path is single-goroutine only).

**`detected_at` = tick time.** The tick timestamp is captured at the top of each `UpdateBaselines`
call (`tickAt := time.Now()`) before the Welford loop begins. All flag events emitted in that tick
share the same `detected_at`. This is a semantic change from `AnomalyFlag.TS`, which is set to
`time.Now()` per `ComputeFlags` call (anomaly.go:377,420); see Consequences §1.

**Synchronous non-batched insert.** Flag events are written via a single
`InsertAnomalyFlagEvent(ctx, event)` call per detected flag, following the `InsertProbeResult`
precedent (server/internal/store/clickhouse/clickhouse.go:651–702). The existing comment on that
method ("results are low frequency — one per probe per interval — so batching is not needed") applies
even more strongly to flag events (~17/day vs. potentially thousands of probe results/day). No flusher
goroutine is added. The insert serializes the `UpdateBaselines` tick only when a flag fires: less than
once per hour on average at Enterprise scale.

**Optional `flagStore` field.** `Detector` gains a new field `flagStore FlagEventStore` (nil by
default). When nil, `checkFlags` skips persistence. All existing tests that call `UpdateBaselines` or
`ComputeFlags` directly remain ClickHouse-free.

```go
// FlagEventStore persists detected anomaly flag events.
// Defined alongside BaselineStore in server/internal/anomaly/anomaly.go (anomaly.go:92–99).
type FlagEventStore interface {
    InsertAnomalyFlagEvent(ctx context.Context, event AnomalyFlagEvent) error
}
```

`clickhouse.Store` implements `FlagEventStore`. Injection at construction: either a `SetFlagStore`
method on `Detector` or a `Config.FlagEventStore` field (the build session decides); wire in
`serve.go` alongside the existing `anomalyDet` construction (serve.go:489).

### 5. Restart dedup: warm hysteresis from store

The in-memory `hysteresis` map (anomaly.go:142) resets to empty on every process restart. Without
mitigation, a flag detected at tick N (row written to `anomaly_flag_events`) triggers again at the
first post-restart tick if the anomaly is still present, producing a duplicate row.

**Warmup query.** A `WarmHysteresis(ctx context.Context)` method — called from `Detector.Run` before
the first tick — executes:

```sql
SELECT metric, scope, max(detected_at) AS last_at
FROM {db}.anomaly_flag_events
WHERE detected_at >= now() - toIntervalSecond({warmup_secs})
GROUP BY metric, scope
```

where `warmup_secs = hysteresisTicks × int(tickInterval.Seconds())` (defaults: 10 × 60 = 600 s).
For each returned row, the map is initialized:

```go
d.hysteresis[hysteresisKey{metric: row.Metric, scope: row.Scope}] = d.hysteresisTicks
```

**Failure mode closed.** Without warmup: flag fires at tick N → process restarts → flag re-fires at
tick N+1 → duplicate row in `anomaly_flag_events`. With warmup: `WarmHysteresis` reads the store,
reconstructs cooldown state, suppresses the re-fire for the remaining cooldown window.

### 6. Read path: separate `FlagHistoryQuerier` interface

A new interface `FlagHistoryQuerier` is defined in `server/internal/api/server.go` alongside (but
separate from) `AnomalyDetector` (server/internal/api/server.go:115–119):

```go
// FlagHistoryQuerier queries the persisted anomaly flag-event store.
// Separate from AnomalyDetector to preserve the single-method interface
// and avoid breaking existing test fakes (blast-radius argument; see §B).
type FlagHistoryQuerier interface {
    QueryFlagHistory(ctx context.Context,
        from, to time.Time,
        app, stream string,
        limit int, cursor string,
    ) (FlagHistoryPage, error)
}

type FlagHistoryPage struct {
    Items      []AnomalyFlagAPI
    NextCursor string // empty on last page
}
```

`Server` gains a field `flagHistoryQuerier FlagHistoryQuerier` and a setter:

```go
// SetFlagHistoryQuerier wires the flag-event store for GET /anomalies ?from/?to.
// Call after New, before Start. Follows the SetIngestQuerier precedent
// (server/internal/api/server.go:275).
func (s *Server) SetFlagHistoryQuerier(q FlagHistoryQuerier) {
    s.flagHistoryQuerier = q
}
```

**Handler routing** (`handleAnomalies`, wave3.go:38–140):

- If `?from` or `?to` is present **and** `s.flagHistoryQuerier != nil`:
  call `s.flagHistoryQuerier.QueryFlagHistory(...)` with parsed `time.Time` arguments.
- If `?from` or `?to` is present **and** `s.flagHistoryQuerier == nil`:
  return `400 BAD_REQUEST` with code `"FLAG_STORE_NOT_CONFIGURED"` (explicit error; not a silent
  fallback to ComputeFlags, which would re-introduce the existing dishonest behavior).
- Otherwise: existing `ComputeFlags` path unchanged.

**Cursor encoding (keyset).** The existing decimal-offset cursor (wave3.go:73–77) is ephemeral and
valid only over the in-memory sorted slice from `ComputeFlags`. The `QueryFlagHistory` path uses a
keyset cursor encoded as `base64(strconv.FormatInt(detected_at_ms, 10) + ":" + id)`. It is pushed
into the SQL `WHERE` clause as `(detected_at, id) > (?, ?)` to prevent page drift as new rows are
inserted. Filters for `app`, `stream`, and the `[from, to]` time range are pushed into SQL `WHERE`
clauses (not applied in-memory), since the store may hold millions of rows on long-retention plans.

### 7. Retention

`{retention_days}` — same placeholder as `probe_results`, `server_events`, and all other raw
ClickHouse tables. Inherits the tier-configured value at migration apply time (PRD §7.11,
prd-report.md:345: Free 7 days, Pro 90 days, Business 13 months). No separate anomaly
flag-history retention knob is introduced at MVP.

### 8. Conformance flip plan

Once the flag-event store ships, the two `known-violation` entries at
param_conformance_test.go:927–939 (quoted in Context) become `probe` entries backed by a
`recordingFlagHistoryQuerier` double — the same pattern as `captureIngestQsvc`
(param_conformance_test.go:494–526):

```go
type recordingFlagHistoryQuerier struct {
    calls []flagHistoryCall
}
type flagHistoryCall struct {
    From, To         time.Time
    App, Stream      string
    Limit            int
    Cursor           string
}
func (r *recordingFlagHistoryQuerier) QueryFlagHistory(
    ctx context.Context, from, to time.Time,
    app, stream string, limit int, cursor string,
) (api.FlagHistoryPage, error) {
    r.calls = append(r.calls, flagHistoryCall{from, to, app, stream, limit, cursor})
    return api.FlagHistoryPage{}, nil
}
```

The `?from` probe:
1. Wires the recording double via `s.SetFlagHistoryQuerier(recording)`.
2. Sends `GET /anomalies?from=<epochMs>` (Enterprise token).
3. Asserts `recording.calls[0].From == time.UnixMilli(epochMs)`.

The `?to` probe follows the same pattern. A non-overlapping-window differential sub-case sends two
requests with non-overlapping `[from, to]` ranges; the recording double returns empty pages for both,
confirming the arguments are routed through rather than silently dropped.

The `minProbes` floor comment (param_conformance_test.go:17, currently "35 probes as of S22/D-084;
floor = 35 − 2 = 33") updates to "37 probes; floor = 37 − 2 = **35**" when the two probes promote.

**Build gate: this ADR is design-only; the S23 session gate is build-only-if-Small and BUG-008 phase 2 is Effort L. The conformance flip, migration, write path, and read path are all deferred to the next build session designated for BUG-008 phase 2.**

---

## Rationale

### ClickHouse is mandated; the meta store is ruled out

The low volume (~17/day) makes the meta store superficially attractive. However, ARCHITECTURE §3.3
draws the boundary on data *character*, not volume: flag events are immutable detections with a natural
TTL — event-series data, structurally identical to `probe_results`. Using the meta store would cross
the two-store boundary and create a precedent for future metrics to follow, eroding the architectural
split. The cost of a ClickHouse table at this cardinality is negligible.

### Write path belongs in `UpdateBaselines`, not `ComputeFlags`

`ComputeFlags` (anomaly.go:348–424) is called per HTTP request with no locking on the calling
goroutine. Multiple concurrent requests within the same 60-second tick interval detect the same flag
(same snapshot, same baselines) and would each write a row. The in-memory hysteresis map suppresses
re-fires across tick boundaries but cannot prevent concurrent writes within the same tick.
`UpdateBaselines` is the single-goroutine tick-driven path and the correct serialization point.
Moving z-score detection to run in both `UpdateBaselines` (for persistence) and `ComputeFlags` (for
the ephemeral response) is bounded: one shared inner function, no new goroutines, no interface changes.

### Separate `FlagHistoryQuerier` interface limits blast radius to zero

Extending `AnomalyDetector` (server/internal/api/server.go:115–119) with a second method would break
`anomalyDetectorBridge` (serve.go:63–88) and every test fake that satisfies the interface, including
`fakeAnomalyDetector` (server/internal/api/bug008_anomalies_filter_test.go:37), which is used by the
four Group A probes landed in S22. The `SetIngestQuerier` precedent (server/internal/api/server.go:275)
shows the established pattern for adding an opt-in querier dependency without modifying an existing
interface. A separate `FlagHistoryQuerier` has zero blast radius: no existing file needs editing until
a build session wires it in.

### Scope JSON column stores raw bytes from `scopeJSON()` — never re-serialized

`hysteresisKey.scope` (anomaly.go:127) is the raw JSON string from `scopeJSON()` (anomaly.go:430–454):
a compact, non-padded, deterministic format like `{"stream_id":"s1"}`. The startup warmup query (§5)
reads `scope` back from the store and uses it to re-populate the hysteresis map. If the column stored
a re-serialized value (produced by `parseScopeJSON` + `json.Marshal`), Go's `encoding/json` could
produce a different byte sequence for the same logical value (field ordering, escaping), silently
producing a non-matching hysteresis key and defeating dedup. Storing the raw string byte-for-byte
guarantees key identity. This is why the schema carries both denormalized columns (for SQL filtering)
and the raw `scope` column (for dedup key identity).

### Synchronous insert is appropriate at flag-event frequency

The `InsertProbeResult` precedent (server/internal/store/clickhouse/clickhouse.go:651–702) uses
synchronous `conn.PrepareBatch + b.Append + b.Send`. Probe results are low-frequency relative to
server events; flag events are lower still (~17/day vs. potentially thousands of probe results/day at
full fleet scale). A flusher goroutine adds a channel, a mutex, a ticker, and a shutdown path — none
of which provides a measurable benefit when the average inter-event interval is ~1.4 hours. The
`UpdateBaselines` tick is serialized by the insert call only when a flag fires.

---

## Consequences

1. **`AnomalyFlag.TS` semantic change for the persisted path.** The `detected_at` stored in
   `anomaly_flag_events` is the tick timestamp (captured once per `UpdateBaselines` call), not
   `time.Now()` at `ComputeFlags` call time (anomaly.go:377,420). The existing ephemeral `ComputeFlags`
   path is unchanged: `AnomalyFlag.TS` continues to be set to `time.Now().UnixMilli()` for live
   responses. Callers that compare `?from`/`?to` window edges with live `ComputeFlags` TS values
   should expect up to one tick interval (default 60 s) of skew between the two.

2. **Synchronous insert adds bounded latency to affected detection ticks.** Each `UpdateBaselines`
   call that detects a new flag incurs one synchronous ClickHouse round-trip. At ~17 flag events/day
   (Enterprise scale), the tick is serialized roughly once every 1.4 hours; all other ticks are
   unaffected. This is negligible. Operators running `PULSE_ANOMALY_TICK_S=5` in CI will not observe
   the effect in practice (flag rate in unit tests is deterministic and controlled).

3. **Ephemeral behavior unchanged when `?from`/`?to` are absent.** The `ComputeFlags` path remains
   the default for all requests without time-range parameters. Deployments that have not yet applied
   migration 0010 (or that set `flagStore = nil`) fall back to the existing `ComputeFlags` path
   cleanly; `handleAnomalies` checks `flagHistoryQuerier != nil` before routing.

4. **Free-tier users get 7 days of flag history.** This matches the Free-tier dashboard retention
   (PRD §7.11) and is consistent with the general principle that Free-tier operators receive the same
   feature set at reduced retention.

5. **Restart dedup adds one ClickHouse query to Detector startup.** `WarmHysteresis` executes a
   single aggregation over a narrow time window (≤600 s by default). On cold starts (no existing flag
   rows), it returns immediately with zero rows. The query adds negligible startup time.

---

## Alternatives considered

### A. Meta store (SQLite/Postgres) as flag-event backend

The meta store could technically hold ~1,530 rows. However, ARCHITECTURE §3.3
(docs/ARCHITECTURE.md:133–135) explicitly reserves the meta store for "config and small relational
state" and states "Metrics never go in the meta store." Flag events are metrics by character: they are
immutable, time-stamped observations of anomaly occurrences subject to TTL expiry. Using the meta
store crosses the two-store boundary established in Wave 1 (INT-01, Q2/Q3) and sets a precedent for
future metrics to follow.

**Rejected:** violates ARCHITECTURE §3.3 (docs/ARCHITECTURE.md:133–135).

### B. Extending `AnomalyDetector` with `QueryFlagHistory`

Adding `QueryFlagHistory` to the existing `AnomalyDetector` interface (server/internal/api/server.go:115–119)
would touch `anomalyDetectorBridge` (serve.go:63–88) and every test fake satisfying the interface. The
four Group A probes landed in S22 use `fakeAnomalyDetector`
(server/internal/api/bug008_anomalies_filter_test.go:37); breaking them would require updating all four
probes for a change that does not affect them. The separate-interface pattern already appears in this
codebase (`SetIngestQuerier`, server/internal/api/server.go:275) precisely for this reason.

**Rejected:** high blast radius with no benefit over the separate-interface pattern.

### C. `ReplacingMergeTree` for automatic restart dedup

Using `ENGINE = ReplacingMergeTree()` with a version column would eventually collapse duplicate rows.
However, ClickHouse's ReplacingMergeTree deduplication is asynchronous (merges happen in the
background at an unspecified time). A query executed seconds after a duplicate insert can see both
rows. The startup warmup query (§5) cannot rely on a deduplicated view. Explicit warmup of the
in-memory hysteresis map gives an immediate, deterministic guarantee and does not require any
ClickHouse-level dedup machinery.

**Rejected:** eventual-merge semantics do not give read-time dedup guarantees.

### D. Writing flag events from `ComputeFlags`

`ComputeFlags` (anomaly.go:348–424) is called per HTTP request and is concurrent: multiple
simultaneous requests within the same 60-second tick interval would each detect the same flag (same
live snapshot, same baselines) and each attempt to write a row. The in-memory `hysteresis` map
(anomaly.go:142) suppresses re-fires across ticks but provides no inter-request serialization within a
tick (the lock at anomaly.go:380 serializes `hysteresis` map access but does not prevent two concurrent
requests from both detecting a new flag before either updates the map). `UpdateBaselines` is the
correct single-writer point.

**Rejected:** concurrent writes produce duplicate rows for the same detection event.

---

## Migration / CR checklist (for the BUG-008 phase-2 build session)

| # | Deliverable | File / Symbol | Notes |
|---|---|---|---|
| 1 | ClickHouse migration | `contracts/db/clickhouse/0010_anomaly_flag_events.sql` | CREATE TABLE + TTL; confirm 0009 exists first |
| 2 | `AnomalyFlagEvent` struct | `server/internal/anomaly/anomaly.go` | Fields: ID, Metric, NodeID, App, StreamID, Scope, Observed, Expected, Sigma, DetectedAt |
| 3 | `FlagEventStore` interface | `server/internal/anomaly/anomaly.go` (alongside `BaselineStore` at anomaly.go:92) | Single method: `InsertAnomalyFlagEvent` |
| 4 | `flagStore` field on `Detector` | `server/internal/anomaly/anomaly.go:132` | `flagStore FlagEventStore` (nil = no persistence) |
| 5 | Shared detection helper | `server/internal/anomaly/anomaly.go` | `checkFlags(ctx, tickAt, baselines, liveValues)` |
| 6 | `UpdateBaselines` write path | `server/internal/anomaly/anomaly.go:338` | Call `checkFlags` after Welford upsert loop |
| 7 | `WarmHysteresis` startup method | `server/internal/anomaly/anomaly.go` | Called from `Detector.Run` before first tick |
| 8 | CH store: write method | `server/internal/store/clickhouse/clickhouse.go` | `InsertAnomalyFlagEvent` — synchronous, non-batched |
| 9 | CH store: read method | `server/internal/store/clickhouse/clickhouse.go` | `QueryFlagHistory` — keyset cursor, WHERE filters pushed to SQL |
| 10 | `FlagHistoryQuerier` interface | `server/internal/api/server.go` (alongside `AnomalyDetector` at server.go:115) | Single method: `QueryFlagHistory` |
| 11 | `SetFlagHistoryQuerier` setter | `server/internal/api/server.go` (alongside `SetAnomalyDetector` at server.go:287) | Follows `SetIngestQuerier` pattern (server.go:275) |
| 12 | Handler routing | `server/internal/api/wave3.go:38–140` | Route on `?from`/`?to` presence; `nil` querier → 400 |
| 13 | `anomalyDetectorBridge` wiring | `server/cmd/pulse/serve.go` | Wire `flagStore` into `anomalyDet`; call `SetFlagHistoryQuerier(store)` near serve.go:492 |
| 14 | Conformance probes | `server/internal/api/param_conformance_test.go:927–939` | Flip two `known-violation` → `probe`; `minProbes` floor 33 → 35 |
| 15 | Capacity re-check | `server/internal/anomaly/anomaly_test.go` | Confirm modeled flag rate ≤ 1/node-week (ADR 0007 budget) after detection-helper refactor |
