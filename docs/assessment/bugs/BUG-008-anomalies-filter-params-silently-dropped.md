# BUG-008: GET /anomalies — from, to, app, stream, limit, cursor all silently dropped (ComputeFlags API mismatch)

**Severity:** high
**Component:** api / anomaly detector (wave3.go)
**Status:** FIXED — S22/D-084 Group A (app/stream/limit/cursor handler-side), S24/D-086 Group B (from/to — anomaly_flag_events ClickHouse table + ADR-0009 flagHistoryBridge wired)

## Summary

`GET /anomalies` declares eight filter params in the OpenAPI spec. Six of them
(`from`, `to`, `app`, `stream`, `limit`, `cursor`) are entirely silently
dropped by the handler. A caller asking for
`?app=myapp&from=T1&to=T2` receives anomaly flags for **all apps across all
time** — the identical defect class as BUG-004 (declared-but-ignored params
returning wrong-scope data).

## Declared vs. Implemented Params

| Param | Status | Notes |
|---|---|---|
| `metric` | **reads=yes** | handler reads, used as post-filter |
| `min_sigma` | **reads=yes** | handler reads, passed to ComputeFlags |
| `from` | **DROPPED** | ComputeFlags takes no time range |
| `to` | **DROPPED** | ComputeFlags takes no time range |
| `app` | **DROPPED** | ComputeFlags takes no entity filter |
| `stream` | **DROPPED** | ComputeFlags takes no entity filter |
| `limit` | **DROPPED** | result set is unbounded |
| `cursor` | **DROPPED** | no pagination |

## Root Cause

The handler (`wave3.go:27-74`) calls
`anomalyDetector.ComputeFlags(ctx, sigmaThreshold)` — the only call — whose
signature accepts no time-range, entity filter, or pagination arguments. The
in-memory anomaly detector operates on a rolling baseline state with no
time-window or per-stream scope, making it **architecturally incompatible**
with the declared time-range and entity filters.

This is distinct from BUG-006/BUG-007 (store-layer pagination gaps). Here the
issue is that the anomaly-detector service API was designed without the filter
surface that the OpenAPI contract advertises.

## Impact

**Severity: high** — same class as BUG-004 (data returned is wrong scope).

- A caller asking `?app=live&from=T1&to=T2` receives anomaly flags for **every
  app and every time period**, producing false positives for unrelated streams.
- Items returned are unbounded (`limit` declared `maximum: 500` but never
  applied); a large deployment could produce an unbounded memory allocation.
- `cursor` is always `null` even when results exceed the practical limit.

Six out of eight declared params are dead code. Only `metric` and `min_sigma`
are read correctly.

## Reproduction

```
POST /api/v1/anomalies     # inject anomaly for app=other
GET  /api/v1/anomalies?app=live
```

Response includes the anomaly for `app=other` despite the `?app=live` filter.

## Fix Suggestion

The fix requires changes at the anomaly-detector service layer, not just the
handler:

1. Extend `anomaly.Detector.ComputeFlags` (or add a new method) to accept a
   time range and entity filter (`app`, `stream`, `from`, `to`).
2. Update the in-memory detector implementation to filter its baseline by the
   requested entity scope and time window.
3. Add `limit` / cursor support to the handler and the detector results.
4. Update the handler in `wave3.go` to read and pass all declared params.

This is a deeper architectural change than BUG-006/BUG-007.
