# Wave-1 Gate Report

**Agent:** QA-01  
**Date:** 2026-06-12  
**Work order:** WO-105  
**Verdict:** PASS_WITH_LIMITATIONS

D-002 limitation: Docker Compose bundle is authored but not executed on this machine. ClickHouse, pulse, and mock-ams run as local processes. All testable criteria pass.

---

## Gate run commands

```
# Rebuild binaries
cd server && CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/
cd qa/mock-ams && CGO_ENABLED=0 go build -o /tmp/mock-ams .

# Run full gate
bash qa/wave-1/run-gate.sh

# Run budget regressions
bash qa/budgets/run-budget-tests.sh
```

Both scripts exit 0 on PASS, nonzero on FAIL. Safe to rerun mechanically.

---

## Criteria — PASS/FAIL

| # | Criterion | Measured | Verdict |
|---|-----------|----------|---------|
| 1 | Go unit tests: `go test ./...` green | 7 pkg PASS, 0 FAIL | PASS |
| 2 | Web `npm run build` green | tsc strict + vite build green | PASS |
| 3 | Web `npm run test` — 21 tests | 21/21 PASS (3 files) | PASS |
| 4 | ClickHouse migration: `pulse migrate` | 15 tables/views created | PASS |
| 5 | pulse serve starts + /healthz ok | All 3 components status=ok | PASS |
| 6 | New stream visible ≤ 10 s (F1) | **1064 ms** | PASS |
| 7 | Viewer counts ±2% of mock truth (F1) | stream-alpha: 133/133 (0%), stream-beta: 66/66 (0%), stream-gamma: 266/266 (0%) | PASS |
| 8 | Alert rules survive pulse restart | Rule persists in SQLite after kill+restart | PASS |
| 9 | Alert detection→notification ≤ 30 s (F5) | **15 s** (fake-clock unit test) | PASS |
| 10 | /healthz reports all components | clickhouse, meta_store, collector all present | PASS |
| 11 | Install path ≤ 15 min | Local stack up in < 2 min total | PASS |
| 12 | Live dashboard shows scenario streams | total_publishers=3, total_viewers=465, streams API=3 items | PASS |
| 13 | Docker Compose bundle authored | deploy/ exists with Dockerfile, docker-compose.yml | WAIVED (D-002) |

---

## Measured numbers

### Stream visibility latency (Criterion 6)

Scenario: 3 streams published to mock-ams (REST v2 control endpoint). pulse configured with 2s poll interval.

```
Published 3 streams at t=0 (control POST /control/publish)
Streams visible after 1064ms (GET /api/v1/live/streams count=3)
Budget: 10 000 ms
PASS: 1064 ms ≤ 10 000 ms
```

With default 5s poll interval (production default), worst-case = 5s ≤ 10s.

### Viewer count accuracy (Criterion 7)

```
stream-alpha: truth=133  pulse=133  error=0.0%  (budget ≤2%)  PASS
stream-beta:  truth=66   pulse=66   error=0.0%  (budget ≤2%)  PASS  
stream-gamma: truth=266  pulse=266  error=0.0%  (budget ≤2%)  PASS
```

Truth = WebRTC viewers + HLS viewers (as returned by mock AMS REST v2 `/truth/viewers/{id}`).  
pulse /api/v1/live/streams `viewers` field = matches exactly.

### Alert detection→notification latency (Criterion 9)

Proven by fake-clock unit test in `server/internal/alert/evaluator_test.go`:

```
Test: TestEvaluator_StreamOffline_FiresWithinBudget
  window_s=10, tick_interval=5s
  Stream offline condition met → fires at t=15s (3 ticks)
  Budget: 30s
  PASS: 15s ≤ 30s
```

Path: collector poll (≤5s) + evaluator tick (≤5s) + channel.Send (~0ms) = ≤10.1s worst-case.  
The fake-clock test conservatively measures 15s (window_s=10 must elapse + 1 tick confirmation).

### Budget regression tests

All 8 budget tests pass:
```
B-01: Stream visibility latency 1.5011065s ≤ 10s         PASS
B-02: Viewer count normalization sums all protocols        PASS
B-03: Alert latency 15s ≤ 30s                             PASS
B-04: ClickHouse DDL 14 create statements                  PASS
B-05: Meta DDL 14 CREATE TABLE statements                  PASS
B-06: CGO_ENABLED=0 go build ./... green                   PASS
B-07: Web bundle 696.79 kB pre-gzip / 206.55 kB gzip      PASS (no hard gate wave-1)
B-08: OpenAPI spec valid (0 errors, 0 warnings)            PASS
```

### Install time walkthrough

Steps completed from clean workspace:
1. `CGO_ENABLED=0 go build -o /tmp/pulse ./server/cmd/pulse/` — ~15s
2. `CGO_ENABLED=0 go build -o /tmp/mock-ams ./qa/mock-ams/` — ~5s  
3. Start ClickHouse (local binary) + `pulse migrate` — ~10s
4. Start pulse serve — ~2s
5. Stack ready and streams visible — ~30s total

Estimated clean-install time: < 2 minutes (local binary path). Docker Compose path (D-002 waived) would be `make up` < 5 min per typical compose-up times.

---

## Defects

### D-W1-001 — Node CPU/mem reported as 100x too high (major)

**Owner:** BE-01  
**Severity:** major  
**Component:** `server/internal/collector/normalize.go`

**Summary:** `NormalizeClusterNode` multiplies `n.CPUUsage * 100` and `n.MemoryUsage * 100`. The AMS REST v2 `cpuUsage` field is already a 0–100 percentage (e.g., 15.0 = 15%), not a 0.0–1.0 fraction. This causes all nodes to report `cpu_pct=1500` for a 15% CPU node, which triggers `status="degraded"` in the query service (threshold > 90).

**Repro:**
```
Mock AMS returns: {"cpuUsage": 15.0, "memoryUsage": 40.0}
normalize.go: cpu_pct = 15.0 * 100 = 1500
query.go:     if n.CPUPCT > 90 → status="degraded" (always)
/live/overview nodes[0].status = "degraded" (wrong)
```

**Fix:** Remove the `* 100` multipliers in `NormalizeClusterNode`. AMS returns 0-100 directly.

```go
// BEFORE (wrong):
"cpu_pct": n.CPUUsage * 100,
// AFTER (correct):
"cpu_pct": n.CPUUsage,
```

**Impact:** Nodes always appear degraded in the fleet view and alert rule `cpu_pct` thresholds are 100x off.

---

### D-W1-002 — /healthz latency_ms hardcoded null, never measures actual latency (minor)

**Owner:** BE-02  
**Severity:** minor  
**Component:** `server/internal/api/server.go` `handleHealthz`

**Summary:** The `/healthz` handler returns `latency_ms: null` for all components unconditionally. The contract (`ComponentStatus.latency_ms`) is optional (nullable), so this doesn't violate the schema — but a real liveness check should ping ClickHouse and the meta store and report actual latency.

**Repro:**
```
curl http://localhost:8090/healthz
{"components":{"clickhouse":{"latency_ms":null,"message":null,"status":"ok"},...}}
```

The handler always returns 200/ok even if ClickHouse is down. When ClickHouse is unreachable, the status should be "down" and HTTP 503 per spec.

---

### D-W1-003 — `pulse migrate` does not run meta migrations (minor)

**Owner:** BE-01 / BE-02  
**Severity:** minor  
**Component:** `server/cmd/pulse/main.go` `runMigrate`

**Summary:** `pulse migrate` runs ClickHouse migrations but skips meta migrations (marked `// HOOK(BE-02)`). The meta schema is only applied when `PULSE_META_DDL_PATH` env var is set and `pulse serve` starts. An operator running `pulse migrate` before `pulse serve` will find meta tables missing.

**Repro:**
```
PULSE_META_DSN=/tmp/test.db /tmp/pulse migrate
sqlite3 /tmp/test.db ".tables"
# → (empty) — no tables
```

**Impact:** Documentation gap; `pulse serve` auto-runs the DDL on start with `PULSE_META_DDL_PATH`, so this doesn't block operation but the `migrate` subcommand is misleading.

---

### D-W1-004 — Duplicate import alias in serve.go (cosmetic)

**Owner:** BE-02  
**Severity:** minor  
**Component:** `server/cmd/pulse/serve.go`

**Summary:** Both `clickhouse "..."` and `chstore "..."` import the same package. The `clickhouse` alias is used for `clickhouse.Config`, while `chstore` is used for `chstore.Store`. Both refer to `server/internal/store/clickhouse`. This compiles and works but is confusing.

**Repro:** `grep -n "clickhouse\|chstore" server/cmd/pulse/serve.go`

---

### D-W1-005 — Duplicate `get()` method in amsclient/client.go (cosmetic)

**Owner:** BE-01  
**Severity:** minor  
**Component:** `server/pkg/amsclient/client.go`

**Summary:** `client.go` defines both `get()` and `getJSON()`. The `get()` method is never called; all API methods use `getJSON()`. The `get()` body also has a bug (creates two decoders, discards the strict one, uses the second but `resp.Body` is already drained by the first).

**Impact:** Dead code; the unused `get()` with its two-decoder logic would silently fail if called.

---

### D-W1-006 — Matrix test workflow not fully implemented (minor)

**Owner:** QA-01  
**Severity:** minor  
**Component:** `.github/workflows/ams-version-matrix.yml`

**Summary:** The AMS version matrix workflow now has structure and REST v2 smoke tests, but the `TestAMSVersionMatrix` Go integration tests in `server/internal/collector/` do not exist yet. The workflow will emit a warning rather than fail for this step.

**Impact:** AMS format-drift detection (PRD §7.13) is partially mitigated by mock-ams shape validation but full matrix coverage requires real AMS containers in CI.

---

## Gaps verified from prior reports

| Gap | Agent | Status |
|-----|-------|--------|
| BE-01 gap 1: ClickHouse data artifacts at repo root | ORCH-00 | Open — `.gitignore` not updated |
| BE-01 gap 7: Duplicate import in serve.go | BE-02 | Confirmed (D-W1-004) |
| BE-01 gap 8: Duplicate `get()` in amsclient | BE-01 | Confirmed (D-W1-005) |
| BE-02 G3: bcrypt vs SHA-256 | BE-02 | Confirmed open (SHA-256 used) |
| BE-02 G4: /metrics stub only | BE-02 | Confirmed — only 2 metrics exported |
| FE-01: `gen:api` path uses relative `..` not `../..` | FE-01 | Present in package.json, works correctly |
| INT-01 path in contracts/README.md `../../contracts` | INT-01 | Confirmed — path is `../contracts` from web/ |

---

## D-002 waiver record

Per decision D-002 (no Docker on this machine), the following criteria are waived and recorded as environment limitations — not product defects:

- Docker Compose `make up` execution
- ClickHouse Docker container path
- AMS container in version matrix CI

Local-process equivalents verified above satisfy all functional requirements. The Compose bundle, Dockerfile, and Helm chart are authored and present in `deploy/`; lint-validation deferred to CI.

---

## Files produced by QA-01

| Path | Description |
|------|-------------|
| `qa/mock-ams/main.go` | Mock AMS REST v2 server |
| `qa/mock-ams/go.mod` | Go module for mock-ams |
| `qa/wave-1/run-gate.sh` | E2E gate runner (exit 0=PASS, 1=FAIL) |
| `qa/wave-1/gate-report.md` | This report |
| `qa/budgets/run-budget-tests.sh` | Budget regression suite |
| `.github/workflows/ams-version-matrix.yml` | Updated matrix workflow (replaces skeleton) |

---

## Summary

Wave-1 is **PASS_WITH_LIMITATIONS**. All testable gate criteria pass with measured numbers below budget. Three defects found: one major (D-W1-001, node CPU normalization 100x too high), two minor documentation/code-quality issues. The D-002 waiver covers Docker Compose unexecutability on this machine. Recommend ORCH-00 route D-W1-001 to BE-01 for fix before wave-2 deployment, as it affects fleet health display and alert threshold calibration.

---

## Re-gate after fix-loop (D-006)

**QA-01 re-verification date:** 2026-06-12  
**Verdict: PASS_WITH_LIMITATIONS**

All five assigned defects (D-W1-001 through D-W1-005) are fixed and verified with fresh measurements below. D-W1-006 (AMS version-matrix Go tests) remains deferred per D-006. No regressions vs the 12 original gate criteria.

### Per-defect verdicts

| Defect | Owner | Fix verified | Method | Verdict |
|--------|-------|-------------|--------|---------|
| D-W1-001 — Node CPU/mem normalized 100x too high | BE-01 | `NormalizeClusterNode` no longer multiplies by 100; `cpu_pct=15.0` for `cpuUsage=15.0` | `TestNormalizeClusterNode_CPUScale` PASS + code inspection of `normalize.go` | **FIXED** |
| D-W1-002 — /healthz latency_ms hardcoded null, never 503 | BE-02 | /healthz pings ClickHouse with 3s timeout; returns 503 + `status:down` when unreachable; `latency_ms` is measured integer | `TestAPI_Healthz_ClickHouseDown_Returns503` + `TestAPI_Healthz_MetaStoreLatency` PASS | **FIXED** |
| D-W1-003 — `pulse migrate` skips meta migrations | BE-02 | `pulse migrate` runs meta DDL from embedded `meta.EmbeddedDDL`; `sqlite3 /tmp/test.db ".tables"` shows 14 tables | Live `PULSE_META_DSN=/tmp/test_migrate.db /tmp/pulse migrate` + sqlite3 check | **FIXED** |
| D-W1-004 — Duplicate import alias in serve.go | BE-02 | Single `"github.com/pulse-analytics/pulse/server/internal/store/clickhouse"` import; `chstore` alias removed | Code inspection of `server/cmd/pulse/serve.go` | **FIXED** |
| D-W1-005 — Dead `get()` method with double-decoder bug | BE-01 | `get()` method deleted from `amsclient/client.go`; only `getJSON()` remains | Code inspection; `go vet ./...` exit 0 | **FIXED** |
| D-W1-006 — AMS version-matrix Go tests not implemented | QA-01 | Deferred per D-006; needs real AMS containers in CI | Carried forward to wave-2 validation sweep | **DEFERRED** |

### CR-1/CR-2 round-trip verification

**CR-1 (AlertRule.name):**
- OpenAPI: `name` is in `AlertRule.required` and `AlertRuleWrite.required` (line 1738 of `pulse-api.yaml`).
- DDL: `name TEXT NOT NULL` in `alert_rules` table (`contracts/db/meta/0001_init.sql`).
- API: `alertRuleFromAPI` returns 422 if `name` is missing; `alertRuleToAPI` emits `"name"` field.
- FE: `ruleDisplayName()` returns `rule.name` directly (no `group_by` workaround); `AlertRuleForm` uses `name` state backed by `AlertRuleWrite.name`.
- Test: `TestAPI_AlertRules_CreateAndList` PASS — POST body includes `"name"` field, 201 returned.
- Workaround check: `grep -n "group_by.*label\|label.*workaround" web/src/features/alerts/AlertRuleForm.tsx` → no output.

**CR-2 (AlertRule.enabled):**
- OpenAPI: `enabled` in `AlertRule.required`; `AlertRuleWrite.enabled` defaults to `true`; description documents `enabled=false` vs `muted=true` semantics.
- DDL: `enabled INTEGER NOT NULL DEFAULT 1` in `alert_rules`.
- Evaluator: `if !rule.Enabled { continue }` check before any evaluation — verified at `evaluator.go:200`.
- Test: `TestEvaluator_DisabledRule_NotEvaluated` PASS — disabled rule produces 0 notifications, 0 history writes.
- FE: `AlertRuleForm` renders `enabled` checkbox (defaults true); `AlertsPage` shows `disabled` badge when `rule.enabled === false`.

### Full mechanical re-run outputs

#### `cd server && CGO_ENABLED=0 go build ./...`
```
(exit 0 — no output)
```

#### `cd server && CGO_ENABLED=0 go vet ./...`
```
(exit 0 — no output)
```

#### `cd server && CGO_ENABLED=0 go test ./... -timeout 120s` (fresh, cache cleared)
```
ok  github.com/pulse-analytics/pulse/server/internal/alert          0.420s
ok  github.com/pulse-analytics/pulse/server/internal/api            1.169s
ok  github.com/pulse-analytics/pulse/server/internal/collector      0.460s
ok  github.com/pulse-analytics/pulse/server/internal/collector/logtail   1.269s
ok  github.com/pulse-analytics/pulse/server/internal/collector/restpoller 4.634s
ok  github.com/pulse-analytics/pulse/server/internal/domain         2.985s
ok  github.com/pulse-analytics/pulse/server/internal/store/meta     0.810s
PASS — 7 packages, 0 FAIL
```

#### `cd web && npm run build`
```
vite v6.4.3 building for production...
✓ 638 modules transformed.
dist/assets/index-DGUx5GLK.js   697.98 kB │ gzip: 206.87 kB
✓ built in 915ms
EXIT: 0
```

#### `cd web && npm run lint`
```
(no output — 0 errors, 0 warnings)
EXIT: 0
```

#### `cd web && npm run test`
```
✓ src/features/live/__tests__/LiveSocket.test.ts (8 tests) 4ms
✓ src/features/live/__tests__/StreamsTable.test.tsx (7 tests) 109ms
✓ src/features/alerts/__tests__/AlertRuleForm.test.tsx (6 tests) 168ms
Tests: 21 passed (21)
EXIT: 0
```

#### `npx @redocly/cli lint contracts/openapi/pulse-api.yaml`
```
contracts/openapi/pulse-api.yaml: validated in 37ms
Woohoo! Your API description is valid.
EXIT: 0
```

#### `bash qa/budgets/run-budget-tests.sh`
```
B-01: Stream visibility latency 1.500s ≤ 10s         PASS
B-02: Viewer count normalization sums all protocols   PASS
B-03: Alert latency 15s ≤ 30s                        PASS
B-04: ClickHouse DDL 14 create statements             PASS
B-05: Meta DDL 14 CREATE TABLE statements             PASS
B-06: CGO_ENABLED=0 go build ./... green              PASS
B-07: Web bundle 697.98 kB pre-gzip                   PASS
B-08: OpenAPI spec valid (0 errors, 0 warnings)       PASS
EXIT: 0
```

#### `bash qa/wave-1/run-gate.sh`
```
[PASS] go test ./... — all packages pass
[PASS] npm run build — green
[PASS] npm run test — all 21 tests pass
[PASS] ClickHouse started
[PASS] pulse migrate succeeded (30ms)
[PASS] ClickHouse migration: 15 tables created (≥9 expected)
[PASS] mock-ams started
[PASS] pulse serve started
[PASS] /healthz returns status=ok
[PASS] /healthz includes component: clickhouse
[PASS] /healthz includes component: meta_store
[PASS] /healthz includes component: collector
[PASS] Stream visible latency: 1061ms (budget: 10000ms)
[PASS] Viewer accuracy for stream-alpha: truth=133 pulse=133 error=0.0% (≤2%)
[PASS] Viewer accuracy for stream-beta: truth=66 pulse=66 error=0.0% (≤2%)
[PASS] Viewer accuracy for stream-gamma: truth=266 pulse=266 error=0.0% (≤2%)
EXIT: 0
```

#### D-W1-003 repro killed
```
$ rm -f /tmp/test_migrate.db && PULSE_META_DSN=/tmp/test_migrate.db /tmp/pulse migrate
[INFO] pulse migrate: meta store migrations done
[WARN] pulse migrate: ClickHouse migrations failed (non-fatal)
[INFO] pulse migrate: done
EXIT: 0

$ sqlite3 /tmp/test_migrate.db ".tables"
alert_channels     anomaly_baselines  license            tenants
alert_history      api_tokens         probes             users
alert_rules        cluster_nodes      report_schedules
ams_sources        ingest_tokens      schema_migrations
```
14 tables present — repro definitively killed.

### Regression check: original 12 gate criteria

| # | Criterion | Re-gate measurement | Verdict |
|---|-----------|---------------------|---------|
| 1 | Go unit tests green | 7 pkg PASS, 0 FAIL (fresh run) | PASS |
| 2 | Web `npm run build` green | tsc strict + vite build green | PASS |
| 3 | Web `npm run test` — 21 tests | 21/21 PASS (3 files) | PASS |
| 4 | ClickHouse migration: `pulse migrate` | 15 tables/views (≥9) | PASS |
| 5 | pulse serve starts + /healthz ok | All 3 components status=ok, latency_ms measured | PASS |
| 6 | New stream visible ≤ 10 s (F1) | **1061 ms** | PASS |
| 7 | Viewer counts ±2% of mock truth (F1) | stream-alpha 0%, stream-beta 0%, stream-gamma 0% | PASS |
| 8 | Alert rules survive pulse restart | Verified in existing `TestMetaStore_AlertRules_SurviveRestart` | PASS |
| 9 | Alert detection→notification ≤ 30 s (F5) | **15 s** fake-clock test (unchanged) | PASS |
| 10 | /healthz reports all components | clickhouse, meta_store, collector — now with real latency_ms | PASS |
| 11 | Install path ≤ 15 min | Binary build < 2 min (unchanged) | PASS |
| 12 | Live dashboard shows scenario streams | total_publishers=3, total_viewers=465, streams=3 (gate run) | PASS |
| 13 | Docker Compose bundle authored | Unchanged — deploy/ exists | WAIVED (D-002) |

### Remaining / carried defects

| ID | Owner | Severity | Summary |
|----|-------|----------|---------|
| D-W1-006 | QA-01 | minor | AMS version-matrix Go integration tests not yet written; deferred to wave-2 validation sweep per D-006. Needs real AMS containers in CI. |

### Re-gate conclusion

**PASS_WITH_LIMITATIONS.** All five assigned defects (D-W1-001 through D-W1-005) are confirmed fixed with fresh measurements. Both contract change requests (CR-1/CR-2) are fully implemented and tested end-to-end (contract → store → API → evaluator → FE). All 12 original testable gate criteria hold at measured values. The sole ongoing waiver is D-002 (no Docker) which carries D-W1-006. Wave 1 gate is closed; proceed to Wave 2.
