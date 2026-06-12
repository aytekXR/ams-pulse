# Fix-loop BE-01 completion report

**Agent:** BE-01  
**Date:** 2026-06-12  
**Decision:** D-006  
**Defects addressed:** D-W1-001, D-W1-005

---

## Changes made

### D-W1-001 — Remove erroneous `* 100` multipliers in NormalizeClusterNode

**File:** `server/internal/collector/normalize.go`

The `NormalizeClusterNode` function was multiplying `n.CPUUsage`, `n.MemoryUsage`, and
`n.DiskUsage` by 100 before storing them as `cpu_pct`, `mem_pct`, and `disk_pct`. The
AMS REST v2 API already returns these fields as 0–100 percentages (e.g., `cpuUsage=15.0`
means 15%), not 0.0–1.0 fractions. The multipliers caused `cpu_pct=1500` for a 15% CPU
node, which always triggered `status="degraded"` in the query service (threshold > 90).

**Fix:** removed all three `* 100` multipliers. Values now pass through unchanged.

```go
// Before (wrong):
"cpu_pct":  n.CPUUsage * 100,
"mem_pct":  n.MemoryUsage * 100,
"disk_pct": n.DiskUsage * 100,

// After (correct):
"cpu_pct":  n.CPUUsage,
"mem_pct":  n.MemoryUsage,
"disk_pct": n.DiskUsage,
```

**New test file:** `server/internal/collector/normalize_test.go`

Three regression-guard tests added in the `collector` package (white-box, no HTTP):

- `TestNormalizeClusterNode_CPUScale` — pins `cpuUsage=15.0 → cpu_pct=15.0` (primary D-W1-001 guard)
- `TestNormalizeClusterNode_NodeIDFallback` — empty NodeID falls back to IP field
- `TestNormalizeClusterNode_BoundaryValues` — table test: 0.0, 100.0, and mid values pass through

---

### D-W1-005 — Delete dead `get()` method in amsclient/client.go

**File:** `server/pkg/amsclient/client.go`

The `get()` method (lines 148–182 in the original file) was never called; all API methods
use `getJSON()`. The `get()` body also contained a double-decoder bug: it created a strict
`json.NewDecoder`, then discarded it (`_ = dec`) and created a second decoder — but
`resp.Body` was already partially drained by the first decoder's internal buffer, so the
second decoder would silently fail on larger responses. Deleted the entire `get()` method.
The `io` import is still needed by `getJSON()` (HTTP error body reading), so no import
change was required.

---

## Verification output

### go build
```
cd server && CGO_ENABLED=0 go build ./...
(exit 0 — no output)
```

### go vet
```
cd server && CGO_ENABLED=0 go vet ./...
(exit 0 — no output)
```

### Normalize tests (primary acceptance output)
```
=== RUN   TestNormalizeClusterNode_CPUScale
--- PASS: TestNormalizeClusterNode_CPUScale (0.00s)
=== RUN   TestNormalizeClusterNode_NodeIDFallback
--- PASS: TestNormalizeClusterNode_NodeIDFallback (0.00s)
=== RUN   TestNormalizeClusterNode_BoundaryValues
=== RUN   TestNormalizeClusterNode_BoundaryValues/zero
=== RUN   TestNormalizeClusterNode_BoundaryValues/max
=== RUN   TestNormalizeClusterNode_BoundaryValues/mid
--- PASS: TestNormalizeClusterNode_BoundaryValues (0.00s)
    --- PASS: TestNormalizeClusterNode_BoundaryValues/zero (0.00s)
    --- PASS: TestNormalizeClusterNode_BoundaryValues/max (0.00s)
    --- PASS: TestNormalizeClusterNode_BoundaryValues/mid (0.00s)
PASS
ok  	github.com/pulse-analytics/pulse/server/internal/collector	0.393s
```

### Full collector + amsclient test run
```
ok  github.com/pulse-analytics/pulse/server/internal/collector           0.393s
ok  github.com/pulse-analytics/pulse/server/internal/collector/logtail   0.224s
ok  github.com/pulse-analytics/pulse/server/internal/collector/restpoller 4.562s
```

### Pre-existing failures (not caused by BE-01, not in scope)

After `go clean -testcache && go test ./...`, the following packages fail with
`constraint failed: NOT NULL constraint failed: alert_rules.name (1299)`:

- `server/internal/alert` (6 tests)
- `server/internal/api` (1 test: `TestAPI_AlertRules_CreateAndList`)
- `server/internal/store/meta` (2 tests)

Root cause: INT-01 (fix-loop) added `name NOT NULL` to the `alert_rules` DDL (CR-1), but
BE-02 has not yet updated `CreateAlertRule` to pass a `name` value. These failures existed
before my changes (confirmed by `git stash` + rerun). Owner: BE-02, per D-006 fix-loop
assignment.

---

## Summary

D-W1-001 fixed — `NormalizeClusterNode` no longer multiplies by 100; node CPU, mem, and
disk percentages now reflect actual AMS values. Three unit tests added and passing,
pinning the correct scale against regression. D-W1-005 fixed — dead `get()` method with
double-decoder bug deleted from `amsclient/client.go`. Build, vet, and all
collector/amsclient tests green.
