# V3a-BE02A1 Fix-Loop Report

**Agent:** BE-02 (backend product-plane)
**Date:** 2026-06-15
**VDs addressed:** VD-10, VD-06, VD-11
**Commit:** 5996f2e

---

## Verification Results

### Build
`timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'` — **PASS** (clean, no output)

### Unit Tests (scoped to assigned packages)
`timeout 180 go test -timeout 150s ./internal/api/... ./internal/query/...`

```
ok  github.com/pulse-analytics/pulse/server/internal/api   0.596s
?   github.com/pulse-analytics/pulse/server/internal/query  [no test files]
```

### Integration Tests (ClickHouse-backed, assigned packages)
`timeout 240 go test -tags integration -timeout 200s ./internal/api/... ./internal/query/...`

```
ok  github.com/pulse-analytics/pulse/server/internal/api   0.893s
ok  github.com/pulse-analytics/pulse/server/internal/query  3.185s
```

All 3 new integration tests green; all existing API tests green.

---

## Changes by VD

### VD-10 — Main-port /ingest/beacon persists to EventSink

**Files changed:**
- `server/internal/api/server.go`
- `server/cmd/pulse/serve.go`

**Root cause:** `handleIngestBeacon` decoded the JSON body but discarded all events — no `sink.WriteBeaconEvent()` call. The `Server` struct had no `EventSink` field.

**Fix:**
- Added `eventSink domain.EventSink` field to `api.Server`
- Added `SetEventSink(sink domain.EventSink)` method with doc comment
- Rewrote `handleIngestBeacon` to:
  1. Use `io.ReadAll(r.Body)` on a `MaxBytesReader` for correct 413 detection
  2. Parse batch JSON into typed struct (same fields as beacon handler)
  3. Build `domain.BeaconEvent` and call `go s.eventSink.WriteBeaconEvent(evt)` if sink is wired
  4. Graceful degradation when sink is nil (returns 202 without write)
  5. Body cap aligned to 64 KB (from 256 KB) per spec
- `serve.go`: added `apiServer.SetEventSink(fanout)` after the full fanout is constructed (line after `SetClickHouseConn`)

**Tests added:** `server/internal/api/vd10_beacon_test.go`
- `TestVD10_BeaconPOST_PersistsToSink` — POSTs a valid beacon batch, asserts 202 AND sink received the `BeaconEvent` with correct `session_id` and `stream_id`
- `TestVD10_BeaconPOST_64KB_Cap` — 70 KB body returns 413 (regression for cap alignment)
- `TestVD10_BeaconPOST_NoSink_StillAccepts` — without sink wired, still returns 202

### VD-06 — Geo and device breakdown queries implemented

**Files changed:**
- `server/internal/query/query.go`
- `server/internal/api/server.go`

**Root cause:** `handleGeoAnalytics` and `handleDeviceAnalytics` unconditionally returned `{"rows":[]}`. No query methods existed in `internal/query`.

**Fix:**
- Added `GeoParams`, `GeoRow` types and `GeoBreakdown(ctx, GeoParams)` method to `query.Service`
  - Queries `viewer_sessions FINAL` with `GROUP BY geo_country` (and `geo_region` when `p.Region=true`)
  - Returns `[]GeoRow` with country/region/views/uniques/watch_time_s
  - Uses `toInt64(count())`, `toInt64(uniq(...))`, `toInt64(sum(...))` for CH type compatibility
  - Falls back to empty when ClickHouse is not configured
- Added `DeviceParams`, `DeviceRow` types and `DeviceBreakdown(ctx, DeviceParams)` method
  - Queries `viewer_sessions FINAL` with `GROUP BY client_device, client_os, client_browser, protocol`
  - Returns `[]DeviceRow`
- Added `buildSessionTimeWhere` helper for `started_at`-based time filters on `viewer_sessions`
- Updated `handleGeoAnalytics` to call `s.qsvc.GeoBreakdown()` and return real rows
- Updated `handleDeviceAnalytics` to call `s.qsvc.DeviceBreakdown()` and return real rows

**Tests added:** `server/internal/query/query_integration_test.go` (build tag: `integration`)
- `TestQuery_GeoBreakdown_NonEmptyRows` — starts CH, seeds 3 viewer_sessions rows (2×US, 1×DE), asserts `GeoBreakdown` returns 2 non-empty rows and US row has `views=2`, `watch_time_s=450`
- `TestQuery_DeviceBreakdown_NonEmptyRows` — seeds 3 sessions (desktop/mobile/mobile), asserts 3 rows returned including a `desktop` row

### VD-11 — /qoe/summary queries rollup_qoe_1h; startup_p50_ms non-zero; field name fixed

**Files changed:**
- `server/internal/query/query.go`
- `server/internal/api/server.go`

**Root cause:** `handleQoeSummary` derived all QoE metrics from the live-snapshot ingest health scores (a heuristic proxy) rather than querying `rollup_qoe_1h`. `avgStartupMS` was never updated so `startup_p50_ms` was always 0. The bitrate timeline field was named `bitrate_kbps` but the OpenAPI spec and UI expect `bitrate_kbps_p50`.

**Fix:**
- Added `QoeParams`, `QoeTotals`, `BitrateBucket`, `QoeSummaryResult` types to `query.go`
- Added `QoeSummary(ctx, QoeParams)` method to `query.Service`:
  - Queries `rollup_qoe_1h` (or `rollup_qoe_1d` for `interval=day`)
  - Uses `quantilesMerge(0.5, 0.95)(startup_ms_state)[1/2]` for real startup p50/p95
  - Uses `sumMerge(rebuffer_total_ms) / sumMerge(watch_time_ms)` for rebuffer_ratio
  - Uses `sumMerge(error_count) / countMerge(session_count)` for error_rate
  - Timeline uses `quantilesMerge(0.5, 0.95)(bitrate_kbps_state)[1]` → field `BitrateKbpsP50` (json: `bitrate_kbps_p50`)
  - Falls back to empty when ClickHouse is not configured or no data
- Replaced `handleQoeSummary` live-snapshot heuristic with `s.qsvc.QoeSummary()` call
- Field name fix: `BitrateBucket.BitrateKbpsP50` → serialized as `bitrate_kbps_p50` (was `bitrate_kbps`)

**Tests added:** `server/internal/query/query_integration_test.go` (integration tag)
- `TestQuery_QoeSummary_RealStartupP50` — inserts beacon_events (startup_complete + heartbeat); MV populates `rollup_qoe_1h`; asserts `startup_p50_ms != 0` (pre-fix was always 0) and `bitrate_kbps_p50` field is populated in the timeline

---

## API Surface Changes (for FE-01 / downstream agents)

| Endpoint | Before | After |
|----------|--------|-------|
| `GET /analytics/geo` | always `{"rows":[]}` | real rows from `viewer_sessions GROUP BY geo_country` |
| `GET /analytics/devices` | always `{"rows":[]}` | real rows from `viewer_sessions GROUP BY client_device,...` |
| `GET /qoe/summary` | `startup_p50_ms=0` always; `bitrate_kbps` field | real `startup_p50_ms` from `rollup_qoe_1h`; field renamed to `bitrate_kbps_p50` |
| `POST /ingest/beacon` (main port) | 202 but silently drops events | 202 AND writes to EventSink; body cap 64 KB |

## Not addressed (out of scope)

- VD-01 (Business tier enum) — INT-01 scope, already fixed
- VD-02 (WS delta shape) — needs FE-01 coordination
- All V3b (tier gating) VDs — explicitly deferred per work order
