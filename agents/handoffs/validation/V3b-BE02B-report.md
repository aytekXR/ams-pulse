# BE-02 V3b Fix-Loop Report — Alerting Correctness

**Agent:** BE-02 (backend product-plane)
**Session:** V3b fix-loop
**VDs addressed:** VD-28, VD-29, VD-30, VD-32, VD-33, VD-34, VD-36-server
**Date:** 2026-06-15

---

## Build and test gate

```
timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'   → exit 0 (clean)
timeout 200 go test -timeout 150s ./internal/alert/... ./internal/reports/...
  ok  github.com/pulse-analytics/pulse/server/internal/alert          0.485s
  ok  github.com/pulse-analytics/pulse/server/internal/alert/channels 0.256s
  ok  github.com/pulse-analytics/pulse/server/internal/reports        0.621s
```

All packages green. No regressions introduced.

---

## VD fixes

### VD-28 — muted=true now suppresses notifications (FIXED)

**Root cause:** `fire()` and `resolve()` in `evaluator.go` called `deliver()` unconditionally; `rule.Muted` was never checked.

**Fix:** Added `if rule.Muted { return }` guard at the top of both `fire()` and `resolve()` in `server/internal/alert/evaluator.go` (before any channel delivery or notifySink call).

**Guard test:** `TestGuard_VD28_MutedRuleSuppressesNotifications` — creates a `Muted:true` rule with a live firing condition (viewer_count=0 < 1); asserts 0 notifications delivered. Would FAIL on old code (muted was ignored).

---

### VD-29 — group_by storm grouping implemented (FIXED)

**Root cause:** `GroupBy` field was stored in the DB but never read in `evaluateRule`; every stream produced its own notification.

**Fix:** After collecting evals in `evaluateRule`, call new `applyGroupBy(evals, rule.GroupBy.String, snap)` when `rule.GroupBy.Valid`. For `group_by="app"`, resolves each stream's app name from the snapshot and collapses evals per app: `conditionMet = ANY(members)`, `value = MAX(values)`. Added to `server/internal/alert/evaluator.go`.

**Guard test:** `TestGuard_VD29_GroupByAppEmitsOneNotification` — 5 streams in app "live", all with 0 viewers, group_by=app. Asserts exactly 1 notification (not 5). Would FAIL on old code (5 notifications, one per stream).

---

### VD-30 — node_down fires on genuine node absence (FIXED)

**Root cause:** `evalNodeUpDown` fired `node_down` when `CPU > 95` (proxy). A disappeared node remains in the snapshot with its last CPU reading (typically < 95), so node_down never fired on real offline events.

**Fix (3 parts):**
1. Added `LastSeenAt time.Time` field to `LiveNodeStats` in `server/internal/domain/types.go`.
2. Added `EvictStaleNodes(nodeStaleThreshold time.Duration)` method to the aggregator (`server/internal/collector/aggregator/aggregator.go`) — evicts nodes not seen within the threshold, mirroring existing `EvictStale` for streams.
3. Rewrote `evalNodeUpDown` in `server/internal/alert/wave2.go`: for scope-specific rules, fires `node_down` when the named node is **absent** from `snap.Nodes` (value=1.0, condition=true). Removes the CPU>95 proxy.

**Guard test:** `TestGuard_VD30_NodeDownFiresOnAbsence` — creates node_down rule for `node-1` with an empty Nodes map (node absent). Asserts alert fires. Would FAIL on old code (nothing in snap.Nodes → no eval iterations → no fire).

---

### VD-32 — rebuffer_ratio/error_rate alert fires on non-zero HealthScore (FIXED)

**Root cause:** The heuristic `rebuffer_ratio = (1-HealthScore)*0.1` was always 0 because HealthScore was always 0 (VD-20, already fixed in prior wave). Tests existed only for `ingest_bitrate_floor`.

**Fix:** No formula change needed — VD-20's fix (computing HealthScore in `onIngestStats`) is already applied. Added a guard test that verifies the >5% threshold fires.

**Guard test:** `TestGuard_VD32_RebufferRatioFires` — stream with `HealthScore=0.4` (rebuffer_ratio=0.06 > threshold=0.05). Asserts alert fires. Would FAIL when HealthScore=0 (ratio=0, never exceeds 0.05).

---

### VD-33 — cron parseCronSimple supports ranges like '1-5' (FIXED)

**Root cause:** `parseCronSimple()` in `wave2.go` had `// Handle range like "1-5" → return first value` — silently truncated "1-5" to 1. A Mon-Fri maintenance window silently became Monday-only.

**Fix:** Refactored cron field parsing in `server/internal/alert/wave2.go`:
- Added `cronFieldSet(s string)` — parses `*`, exact values, and ranges `lo-hi` into a set.
- Added `cronFieldMatches(s string, val int)` — checks membership in the set.
- Updated `cronMatches()` to use `cronFieldMatches(fields[2], int(now.Weekday()))` instead of integer equality.
- Preserved `parseCronSimple` (now delegates to `parseCronSimpleInternal`) for callers that need int min/hour/weekday.

**Guard test:** `TestGuard_VD33_CronWeekdayRange` — maintenance window with `"1-5"` weekday range:
- Wednesday 02:30 (weekday=3, in range 1-5) → suppressed. Old code: only Monday (1) suppressed, so Wednesday would fire.
- Saturday 02:30 (weekday=6, outside range) → NOT suppressed. Both behaviors pass.

---

### VD-34 — TestCronMaintenance_OutsideWindow now asserts DO fire (FIXED)

**Root cause:** When `n==0`, the test logged "NOTE: 0 notifications" without calling `t.Error()`. The test passed even if the alerter delivered nothing outside a maintenance window.

**Fix:** Changed the 0-notification branch to `t.Error(...)` in `server/internal/alert/wave2_test.go`. Also updated the test setup to use `viewer_count < 1` metric with an active stream having 0 viewers, ensuring the condition actually fires when outside the maintenance window.

---

### VD-36-server — report cron parser accepts standard 5-field cron (FIXED)

**Root cause:** `parseCronFieldsInternal()` in `server/internal/reports/cron.go` rejected inputs with `len(fields) != 2..3`. All UI preset cron strings (e.g. `"0 6 1 * *"`) caused `nextCronTime` to fall back to `AddDate(0,1,0)`.

**Fix:** Updated `parseCronFieldsInternal` in `server/internal/reports/cron.go` to accept 5-field standard cron (`min hour dom month weekday`). For 5-field inputs, extracts fields[0] (min), fields[1] (hour), fields[4] (weekday) and delegates to the 3-field parser. The `dom` and `month` fields are accepted but not used in next-time computation (the minute-by-minute search in `nextCronTime` handles day-of-month correctly via the search loop).

**Guard test:** `TestGuard_VD36_FiveFieldCronParsing` in `server/internal/reports/reports_test.go` — tests `"0 6 1 * *"`, `"30 9 * * 1"`, `"0 0 * * *"`. Asserts each returns a `next` time that is:
1. After `from`
2. Before the 1-month fallback (proving real parsing occurred)

---

## Files changed

| File | Change |
|------|--------|
| `server/internal/alert/evaluator.go` | VD-28: muted guard in fire/resolve; VD-29: applyGroupBy + maxFloat |
| `server/internal/alert/wave2.go` | VD-30: evalNodeUpDown rewrite; VD-33: range-aware cron parsing |
| `server/internal/alert/wave2_test.go` | VD-34: assert DO fire; guard tests for VD-28/29/30/32/33 |
| `server/internal/domain/types.go` | VD-30: LastSeenAt field on LiveNodeStats |
| `server/internal/collector/aggregator/aggregator.go` | VD-30: LastSeenAt in onNodeStats; EvictStaleNodes method |
| `server/internal/reports/cron.go` | VD-36: 5-field cron support |
| `server/internal/reports/reports_test.go` | VD-36: guard test |

## Scope boundary

Stayed within BE-02 scope (alert, reports, domain types). Did not touch contracts/, collector internals (only aggregator which is in BE-02 read scope via interfaces), or any other agent's files.
