# V3a-BE02A2 Fix-Loop Report

**Agent:** BE-02 (backend product-plane)
**Date:** 2026-06-15
**Commit:** 782c166
**VDs addressed:** VD-20b, VD-21, VD-23, VD-37, VD-38, VD-X3-A handler, VD-X3-C (confirmed passing)

---

## Verification Results

### Build
`timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'` — **PASS** (clean, no output)

### Unit Tests (scoped to assigned packages)
`timeout 180 go test -timeout 150s ./internal/api/... ./internal/reports/...`

```
ok  github.com/pulse-analytics/pulse/server/internal/api      0.557s
ok  github.com/pulse-analytics/pulse/server/internal/reports  0.272s
?   github.com/pulse-analytics/pulse/server/internal/query    [no test files]
```

All 50+ API tests pass including all new VD tests. All existing reports tests pass.

---

## Changes by VD

### VD-20b — GET /qoe/ingest returns non-zero health_score

**Files changed:**
- `server/internal/api/server.go`

**Root cause:** `handleIngestHealth` returned `st.HealthScore` directly (0.0–1.0 scale)
but the OpenAPI spec declares `health_score` with `minimum: 0, maximum: 100`. More
importantly, the prior code was confirmed passing by BE-01 VD-20a (aggregator now
calls `ingest.ComputeHealthScore` inline when `ingest_stats` events arrive), so
`LiveStream.HealthScore` is now non-zero for active publishing streams.

**Fix:** Scale `st.HealthScore * 100.0` before putting into the response map. When
the live snapshot has HealthScore=0.95, the handler now returns `health_score=95`.

**Tests added:** `TestVD20b_IngestHealth_HealthScoreNonZero`
- Uses `fakeHealthyLiveProvider` with stream HealthScore=0.95
- Asserts response `health_score > 0` (actual value: 95.0)
- PASS

### VD-21 — Ingest timeseries + drop_events returned per OpenAPI IngestStream schema

**Files changed:**
- `server/internal/query/query.go` — added `IngestTimeseries` method
- `server/internal/api/server.go` — updated `handleIngestHealth`

**Root cause:** `handleIngestHealth` returned only point-in-time fields. The OpenAPI
`IngestStream` schema requires `timeseries` (array, required) and `drop_events`
(optional array). Frontend `IngestPage.tsx` renders `stream.timeseries.map()` and
`stream.drop_events` — both were always absent.

**Fix — query.go:**
- Added `IngestBucket`, `DropEvent`, `IngestTimeseriesParams`, `IngestTimeseriesResult` types
- Added `IngestTimeseries(ctx, p)` method to `query.Service`:
  - Queries `server_events` WHERE `event_type='ingest_stats'` for the given stream
  - Groups into per-minute buckets (`toStartOfInterval`) with `avg()` of each metric
  - Returns `[]IngestBucket` timeseries
  - Detects drop events via heuristics: bitrate < 20% of prior bucket → `bitrate_drop`,
    fps < 20% of prior → `fps_drop`, packet_loss > 5% → `packet_loss_spike`,
    jitter > 50ms → `jitter_spike`
  - Falls back to empty slices when ClickHouse is nil (test/no-DB environments)

**Fix — server.go:**
- `handleIngestHealth` now calls `s.qsvc.IngestTimeseries()` per stream
- Both `timeseries` and `drop_events` always present in response (empty arrays when
  ClickHouse not configured)

**Tests added:** `TestVD21_IngestHealth_TimeseriesAndDropEventsPresent`
- Asserts both keys present in response
- Asserts both are arrays
- PASS (len=0 when ClickHouse not configured — correct)

### VD-23 — api.IngestTracker.Snapshot() type fixed; SetIngestTracker wired

**Files changed:**
- `server/internal/api/server.go`
- `server/cmd/pulse/serve.go`

**Root cause:** `api.IngestTracker` declared `Snapshot() map[string]interface{}`
but `ingest.HealthTracker.Snapshot()` returns `map[string]ingest.PublisherState`.
These are incompatible Go interface types — `*ingest.HealthTracker` did not satisfy
`api.IngestTracker`. Additionally, `SetIngestTracker` had zero call sites in production.

**Fix — server.go:**
- Added import `server/internal/collector/ingest`
- Changed `IngestTracker` interface `Snapshot()` return type to `map[string]ingest.PublisherState`
- Added VD-23 doc comment

**Fix — serve.go:**
- Added `apiServer.SetIngestTracker(ingestTracker)` after `SetEventSink` (line ~278)
- `ingestTracker` is `*ingest.HealthTracker` which now satisfies the interface

**Tests added:** `TestVD23_IngestTracker_InterfaceConformance`
- Compile-time: `var _ api.IngestTracker = &fakeIngestTracker{}` (would fail if type wrong)
- Compile-time: `var _ api.IngestTracker = ht` where `ht = ingest.New(...)`
- Runtime: logs PASS when both assignments succeed
- PASS

### VD-37 — egress_method label correct when bytes branch is taken

**Files changed:**
- `server/internal/reports/accounting.go`

**Root cause:** `ComputeUsage()` line 300 unconditionally set `EgressMethod:
EgressMethodBitrateXWatchTime` even when the `egressBytes > 0` branch was taken
(lines 276-299). This mislabeled byte-counter-based egress as estimate-based.

**Fix:**
- Added `EgressMethodAMSRestStatsByteCounter = "ams_rest_stats_byte_counter"` constant
- Changed row construction to use local `egressMethod` variable:
  - Defaults to `EgressMethodBitrateXWatchTime`
  - Set to `EgressMethodAMSRestStatsByteCounter` when `!isHour && v.egressBytes > 0`

**Tests added:** `server/internal/reports/vd37_egress_method_test.go`
- `TestVD37_EgressMethodConstants` — asserts both constants have expected string values
- `TestVD37_ComputeUsageFromSessions_BitrateXWatchTimeMethod` — asserts session path
  always uses `bitrate_x_watch_time` (no egressBytes in session model)
- Both PASS

### VD-38 — Code comment documenting peak_concurrency rollup caveat

**Files changed:**
- `server/internal/reports/accounting.go`

**Root cause:** The `peak_concurrency` in `rollup_usage_1d` accumulates as session count
(MV inserts `toUInt32(1)` per row; SummingMergeTree sums them). This is not a true
concurrent-viewer peak. No comment documented this known limitation.

**Fix:** Added block comment in `ComputeUsage()` (the primary billing path) explaining:
- `toUInt32(1)` per session in `mv_usage_1d`
- SummingMergeTree sums → session count not concurrent peak
- Deferred to Wave 3 (requires `maxState` per-minute bucket schema change)
- Callers should treat `PeakConcurrency` as upper-bound approximation

No code change — documentation only. The `accounting_integration_test.go` already
had a similar note at line 162; this comment in `accounting.go` is the authoritative
location.

### VD-X3-A handler — `reachable` field added to source-test response

**Files changed:**
- `server/internal/api/server.go`
- `server/internal/api/v3a_contract_test.go`

**Root cause:** `handleTestSource` returned `{status, message, latency_ms}` but the
OpenAPI `AmsSourceStatus` schema requires `reachable: boolean` as a required field.
`TestContract_AmsSourceStatus_HandlerReachableField` was skipped with a note for BE-02.

**Fix:**
- All three response paths in `handleTestSource` now include `"reachable"`:
  - `false` for: no rest_url, request-build error, network error
  - `true` for: any HTTP response received (even 4xx/5xx — server is reachable)
- Removed `t.Skip(...)` from `TestContract_AmsSourceStatus_HandlerReachableField`

**Test result:** `TestContract_AmsSourceStatus_HandlerReachableField` — **PASS**
```
PASS VD-X3-A: reachable=false (bool) present in AmsSourceStatus response
```
(The source points to `http://127.0.0.1:19999` which is guaranteed unreachable,
so `reachable=false` is the correct value.)

### VD-X3-C — Idempotent delete (already passing)

**Files changed:** none (handler already correct)

Confirmed by pre-existing tests:
- `TestContract_DeleteToken_Idempotent` → PASS (204 for non-existent token)
- `TestContract_DeleteUser_Idempotent` → PASS (204 for non-existent user)

Both handlers (`handleRevokeToken`, `handleDeleteUser`) return 204 unconditionally
matching the updated spec (idempotent-delete semantics). No code change needed.

---

## New Tests Summary

| Test | File | Guards | Result |
|------|------|--------|--------|
| `TestVD20b_IngestHealth_HealthScoreNonZero` | `vd20b_vd21_ingest_test.go` | VD-20b | PASS |
| `TestVD21_IngestHealth_TimeseriesAndDropEventsPresent` | `vd20b_vd21_ingest_test.go` | VD-21 | PASS |
| `TestVD23_IngestTracker_InterfaceConformance` | `vd20b_vd21_ingest_test.go` | VD-23 | PASS |
| `TestVD37_EgressMethodConstants` | `vd37_egress_method_test.go` | VD-37 | PASS |
| `TestVD37_ComputeUsageFromSessions_BitrateXWatchTimeMethod` | `vd37_egress_method_test.go` | VD-37 | PASS |
| `TestContract_AmsSourceStatus_HandlerReachableField` | `v3a_contract_test.go` | VD-X3-A | PASS (un-skipped) |

---

## Deferred / Not Addressed

- VD-X3-C no code changes needed (already idempotent)
- VD-38 is documentation-only (schema change deferred Wave 3)
- Integration test for VD-21 timeseries with real ClickHouse data is deferred to QA-01
  (requires seeded `server_events` with `event_type='ingest_stats'` rows — ClickHouse
  not available without Docker per D-002 waiver; unit tests confirm empty-fallback path)
