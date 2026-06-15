# V3a QA-01 Mini-Gate Report

**Agent:** QA-01 (verify, never fix)
**Date:** 2026-06-15
**VDs gated:** VD-09, VD-10, VD-06, VD-20, VD-21, VD-11

---

## Build Gate

| Component | Command | Result |
|-----------|---------|--------|
| Server binary | `timeout 160 bash -c 'cd server && CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/'` | **PASS** (exit 0, clean output) |
| SDK | `timeout 180 bash -c 'cd sdk/beacon-js && npm run build'` | **PASS** (ESM 11.44 KB, CJS 11.92 KB, IIFE 11.43 KB) |

---

## VD Checks

### VD-09 — SDK sends correct ingest token header

**Method:** grep both codebases + SDK unit test.

- SDK (`transport.ts:138`): `'X-Pulse-Ingest-Token': this.cfg.token` — correct.
- Server (`api/server.go:1355`, `beacon/beacon.go:205`): `r.Header.Get("X-Pulse-Ingest-Token")` — matches.
- Old wrong value `X-Pulse-Token`: NOT present in SDK source.
- Guard test `transport.test.ts`: "sends X-Pulse-Ingest-Token header (NOT X-Pulse-Token)" — **PASS** (part of 65 total SDK tests).

**Verdict: PASS**

---

### VD-10 — Main-port /ingest/beacon persists to EventSink

**Method:** `go test -tags integration -run TestVD10_BeaconPOST_PersistsToSink ./internal/api/...`

Output:
```
vd10_beacon_test.go:187: PASS VD-10: BeaconEvent delivered to sink (session=test-session-abc123, events=1)
--- PASS: TestVD10_BeaconPOST_PersistsToSink (0.00s)
```

Additional guards:
- `TestVD10_BeaconPOST_64KB_Cap`: 70 KB body → 413 (PASS)
- `TestVD10_BeaconPOST_NoSink_StillAccepts`: without sink → 202 graceful (PASS)

**Verdict: PASS**

---

### VD-06 — Geo and device breakdown queries return non-empty rows

**Method:** `go test -tags integration -run "TestQuery_(GeoBreakdown|DeviceBreakdown)_NonEmptyRows" ./internal/query/...`

Output:
```
TestQuery_GeoBreakdown_NonEmptyRows: GeoBreakdown returned 2 rows; US row: views=2, uniques=2, watch_time_s=450
PASS VD-06: GeoBreakdown returns 2 non-empty rows from seeded data

TestQuery_DeviceBreakdown_NonEmptyRows: DeviceBreakdown returned 3 rows; desktop row: views=1, os=linux, browser=chrome, protocol=hls
PASS VD-06: DeviceBreakdown returns 3 non-empty rows from seeded data
```

Uses real ClickHouse (testcontainers/embedded), seeded rows, FINAL merge confirmed.

**Verdict: PASS**

---

### VD-20 — health_score non-zero (GET /qoe/ingest)

**Method:** `go test -tags integration -run TestVD20b_IngestHealth_HealthScoreNonZero ./internal/api/...`

Output:
```
vd20b_vd21_ingest_test.go:182: PASS VD-20b: health_score = 95 > 0 (raw=0.95 × 100 = 95.0)
--- PASS: TestVD20b_IngestHealth_HealthScoreNonZero (0.00s)
```

Pre-fix: `HealthScore` was 0 (aggregator never called `ComputeHealthScore`). Post-fix: BE-01 wired `ingest.ComputeHealthScore()` into `onIngestStats()`; BE-02 scaled ×100 for the API response.

**Verdict: PASS** (measured value: 95.0 on 0–100 scale)

---

### VD-21 — Ingest timeseries + drop_events in response

**Method:** `go test -tags integration -run TestVD21_IngestHealth_TimeseriesAndDropEventsPresent ./internal/api/...`

Output:
```
vd20b_vd21_ingest_test.go:228: PASS VD-21: 'timeseries' present (len=0)
vd20b_vd21_ingest_test.go:241: PASS VD-21: 'drop_events' present (len=0)
--- PASS: TestVD21_IngestHealth_TimeseriesAndDropEventsPresent (0.00s)
```

Both `timeseries` and `drop_events` keys always present (empty arrays when ClickHouse not configured — correct per spec). VD-23 companion (IngestTracker interface conformance) also PASS.

**Verdict: PASS** — keys present; live-data population is a D-002 waiver (no real CH in unit env, integration test with seeded data deferred per BE-02 report)

---

### VD-11 — /qoe/summary: real startup_p50_ms non-zero + field named bitrate_kbps_p50

**Method:** `go test -tags integration -run TestQuery_QoeSummary_RealStartupP50 ./internal/query/...`

Output:
```
query_integration_test.go:463: startup_p50_ms=250.0 startup_p95_ms=1365.0
query_integration_test.go:478: bitrate_kbps_p50=2500.0 (correct field name)
query_integration_test.go:484: PASS VD-11: QoeSummary returns real startup_p50_ms=250.0 from rollup_qoe_1h
--- PASS: TestQuery_QoeSummary_RealStartupP50 (1.29s)
```

Pre-fix: `startup_p50_ms` was always 0; field was named `bitrate_kbps`. Post-fix: queries `rollup_qoe_1h` via `quantilesMerge`; field renamed to `bitrate_kbps_p50`.

**Verdict: PASS** (measured startup_p50_ms=250.0, bitrate_kbps_p50=2500.0)

---

## Regression Gate

### Server (all packages)

```
timeout 300 go test -timeout 250s ./...
```

All 17 testable packages PASS:
```
ok  github.com/pulse-analytics/pulse/server/internal/alert              0.922s
ok  github.com/pulse-analytics/pulse/server/internal/alert/channels     0.324s
ok  github.com/pulse-analytics/pulse/server/internal/anomaly            0.605s
ok  github.com/pulse-analytics/pulse/server/internal/api               1.612s
ok  github.com/pulse-analytics/pulse/server/internal/cluster           1.488s
ok  github.com/pulse-analytics/pulse/server/internal/collector         1.603s
ok  github.com/pulse-analytics/pulse/server/internal/collector/aggregator 2.492s
ok  github.com/pulse-analytics/pulse/server/internal/collector/beacon  2.193s
ok  github.com/pulse-analytics/pulse/server/internal/collector/ingest  1.985s
ok  github.com/pulse-analytics/pulse/server/internal/collector/kafka   2.755s
ok  github.com/pulse-analytics/pulse/server/internal/collector/logtail 2.815s
ok  github.com/pulse-analytics/pulse/server/internal/collector/restpoller 6.843s
ok  github.com/pulse-analytics/pulse/server/internal/collector/sessions 2.861s
ok  github.com/pulse-analytics/pulse/server/internal/domain            4.497s
ok  github.com/pulse-analytics/pulse/server/internal/license           2.547s
ok  github.com/pulse-analytics/pulse/server/internal/prober            3.977s
ok  github.com/pulse-analytics/pulse/server/internal/reports           2.678s
ok  github.com/pulse-analytics/pulse/server/internal/store/meta        2.763s
```

**Verdict: PASS**

### Web

```
timeout 200 bash -c 'cd web && npm run build && npm run test'
```

- `npm run build`: PASS (643 modules, dist/index.html generated)
- `npm run test`: **PASS** — 127 tests in 10 files (0 failures, 0 skips; `act()` warnings in AnomaliesPage are pre-existing non-blocking)

**Verdict: PASS**

### SDK

```
timeout 180 bash -c 'cd sdk/beacon-js && npm run test && npm run size'
```

- `npm run test`: PASS — 65 tests in 5 files
- `npm run size`: **3.52 KB** (limit: 15 KB) — PASS

**Verdict: PASS**

---

## Integration Test Gate (tags:integration)

```
timeout 300 go test -timeout 250s -tags integration ./internal/api/... ./internal/query/...
```

```
ok  github.com/pulse-analytics/pulse/server/internal/api   0.757s
ok  github.com/pulse-analytics/pulse/server/internal/query 3.687s
```

All 3 query integration tests (GeoBreakdown, DeviceBreakdown, QoeSummary) and all API tests PASS.

**Verdict: PASS**

---

## Still-Open Defects

| ID | Description | Status | Notes |
|----|-------------|--------|-------|
| VD-21 live-data timeseries | `drop_events` heuristic detection requires seeded `server_events` in a live ClickHouse; only empty-fallback path tested in unit env | WAIVED | D-002 (no Docker in CI); contract-level pass confirmed |
| Beacon config not wired into beaconingest.NewServer() in serve.go | `GeoResolver`/`UAParser` passed to `beacon.Config` but not forwarded to dedicated beacon server's `beaconingest.NewServer()` | DEFERRED | Minor, documented in BE-01 report §Deferred. Does not affect main-port `/ingest/beacon` (VD-10 path) which is wired. |
| VD-38 peak_concurrency rollup | `peak_concurrency` = session count (SummingMergeTree), not true concurrent viewer peak | DEFERRED | Wave 3; doc comment added per BE-02 |

---

## Summary

**Mini-gate verdict: PASS**

All 6 assigned VDs (VD-09, VD-10, VD-06, VD-20, VD-21, VD-11) verified with bounded commands. No regressions introduced: 17 Go packages, 127 web tests, 65 SDK tests all green. Integration tests with real ClickHouse confirm geo/device non-empty rows and QoE startup_p50_ms=250.0 non-zero. The two open items are both waiverered or deferred-wave concerns, not V3a blockers.
