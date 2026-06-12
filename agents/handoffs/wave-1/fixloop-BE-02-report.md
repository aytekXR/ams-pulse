# Wave-1 Fix-Loop Completion Report — BE-02

**Agent:** BE-02  
**Date:** 2026-06-12  
**Ordered by:** ORCH-00 D-006  
**Prereq:** fixloop-BE-01-report.md and fixloop-INT-01-report.md read and confirmed

---

## Summary of changes

Four defects/CRs addressed: CR-1/CR-2 (alert rule name + enabled fields), D-W1-002 (/healthz real probes + 503), D-W1-003 (meta migrate embedded DDL), D-W1-004 (duplicate import alias).

---

## Changes by item

### CR-1 + CR-2 — AlertRule.name and AlertRule.enabled

**Files changed:**

| File | Change |
|------|--------|
| `server/internal/store/meta/meta.go` | Added `Name string` and `Enabled bool` fields to `AlertRuleRow`; updated `CreateAlertRule`, `GetAlertRule`, `ListAlertRules`, `UpdateAlertRule`, `scanAlertRule` to include both columns in all SQL statements. Added `Ping(ctx)` method for /healthz probe. |
| `server/internal/api/server.go` | Updated `alertRuleToAPI` to emit `name` and `enabled`; updated `alertRuleFromAPI` to require `name` (422 if missing) and parse `enabled` (defaults true per OpenAPI spec). |
| `server/internal/alert/evaluator.go` | `evaluate()` now checks `!rule.Enabled` before `rule.Muted`; rules with `enabled=false` are completely skipped (no evaluation, no history). |
| `server/internal/alert/evaluator_test.go` | Added `Name` and `Enabled: true` to all existing `AlertRuleRow` literals (zero-value `Enabled=false` would have caused rules to be skipped). Added `TestEvaluator_DisabledRule_NotEvaluated` — verifies no notifications and no history when `enabled=false`. |
| `server/internal/store/meta/meta_test.go` | Added `Name` and `Enabled: true` to all `AlertRuleRow` literals in round-trip and survive-restart tests. |
| `server/internal/api/api_test.go` | Added `"name"` and `"enabled"` fields to `TestAPI_AlertRules_CreateAndList` POST body. |

**Semantic distinction enforced:**
- `enabled=false`: rule is not evaluated at all — no firing, no history write.
- `muted=true`: rule is evaluated and history is written, but no notification dispatched.
- Evaluator checks `!rule.Enabled` first (exits early), then checks `rule.Muted` within `evaluate()`.

---

### D-W1-002 — /healthz real probes + 503

**Files changed:**

| File | Change |
|------|--------|
| `server/internal/api/server.go` | Added `clickhouse` import; added `chConn clickhouse.Conn` field to `Server`; added `SetClickHouseConn(conn)` method; rewrote `handleHealthz` to ping ClickHouse (3 s timeout) and meta store (3 s timeout), measure real `latency_ms`, return HTTP 503 + `status:"down"` when any critical component is unreachable. |
| `server/cmd/pulse/serve.go` | Added `apiServer.SetClickHouseConn(store.GetConn())` after API server construction. (D-005 cmd edit declared.) |
| `server/internal/api/api_test.go` | Added `TestAPI_Healthz_ClickHouseDown_Returns503` — builds a server with an unreachable ClickHouse conn (127.0.0.1:19191), asserts HTTP 503 with `status:"down"`. Added `TestAPI_Healthz_MetaStoreLatency` — asserts `latency_ms` is a measured integer, not null, for the meta store component. |

**Verification:**
```
=== RUN   TestAPI_Healthz_ClickHouseDown_Returns503
    api_test.go:484: PASS: /healthz → 503 with body=map[components:map[clickhouse:map[latency_ms:0 message:dial tcp 127.0.0.1:19191: connect: connection refused status:down] collector:... meta_store:map[latency_ms:0 ... status:ok]] status:down]
--- PASS
=== RUN   TestAPI_Healthz_MetaStoreLatency
    api_test.go:527: PASS: /healthz meta_store latency_ms=0 (measured, not null)
--- PASS
```

---

### D-W1-003 — pulse migrate applies meta schema (embedded DDL)

**Files changed:**

| File | Change |
|------|--------|
| `server/internal/store/meta/embed.go` | New file. `//go:embed sql/0001_init.sql` embeds the meta DDL as `EmbeddedDDL string`. |
| `server/internal/store/meta/sql/0001_init.sql` | Copy of `contracts/db/meta/0001_init.sql` embedded at compile time. |
| `server/cmd/pulse/main.go` | `runMigrate` now runs meta migrations first (using `meta.EmbeddedDDL`, falling back to `PULSE_META_DDL_PATH` override). ClickHouse migration failure is now non-fatal (logged as Warn); meta migration failure is fatal. (D-005 cmd edit declared.) |

**Repro killed:**
```
$ rm -f /tmp/x.db && PULSE_META_DSN=/tmp/x.db pulse migrate 2>/dev/null && sqlite3 /tmp/x.db ".tables"
alert_channels     anomaly_baselines  license            tenants
alert_history      api_tokens         probes             users
alert_rules        cluster_nodes      report_schedules
ams_sources        ingest_tokens      schema_migrations
```

Schema present after `pulse migrate` without requiring `PULSE_META_DDL_PATH`.

---

### D-W1-004 — Duplicate import alias in serve.go

**Files changed:**

| File | Change |
|------|--------|
| `server/cmd/pulse/serve.go` | Removed `chstore "github.com/pulse-analytics/pulse/server/internal/store/clickhouse"` alias; kept `"github.com/pulse-analytics/pulse/server/internal/store/clickhouse"` as the single import; updated `server.store` field type from `*chstore.Store` to `*clickhouse.Store`. (D-005 cmd edit declared.) |

---

## Acceptance check results

```
CGO_ENABLED=0 go build ./...    → no output (PASS)
CGO_ENABLED=0 go vet ./...      → no output (PASS)
CGO_ENABLED=0 go test ./... -timeout 120s

ok  github.com/pulse-analytics/pulse/server/internal/alert          (7 tests)
ok  github.com/pulse-analytics/pulse/server/internal/api            (10 tests)
ok  github.com/pulse-analytics/pulse/server/internal/collector
ok  github.com/pulse-analytics/pulse/server/internal/collector/logtail
ok  github.com/pulse-analytics/pulse/server/internal/collector/restpoller
ok  github.com/pulse-analytics/pulse/server/internal/domain
ok  github.com/pulse-analytics/pulse/server/internal/store/meta
PASS — 0 failures
```

Budget regression tests:
```
B-05: Meta DDL has 14 CREATE TABLE statements (≥10)  PASS
B-06: CGO_ENABLED=0 go build ./... — PASS
B-08: OpenAPI spec valid — 0 errors  PASS
All 8 budget tests PASS
```

---

## cmd/ edits declared (D-005)

Per D-005 (BE-02 may extend cmd/pulse, declared in report):

1. `server/cmd/pulse/serve.go` — removed duplicate import alias; wired `SetClickHouseConn`.
2. `server/cmd/pulse/main.go` — extended `runMigrate` with meta migration logic using `meta.EmbeddedDDL`.

---

## Gaps

None for this fix-loop. All four assigned items are fully addressed and verified.
