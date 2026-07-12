# BUG-011: `EvictStaleNodes` implemented but never wired — `node_down` structurally unable to fire

**Severity:** high
**Component:** collector/aggregator wiring (serve.go)
**Status:** **FIXED (S25 / D-087, 2026-07-12)**

## Symptom

`node_down` alerts never fire in production. A frozen AMS node (rung 3 of the
upstream failure class ant-media/Ant-Media-Server#7926 — OS metrics normal, Java
alive, HLS/API dead) is never removed from the live snapshot, so the alert
evaluator never sees the node as absent and cannot trigger a `node_down` alert.

This is the reason the S19 assessment matrix's "node offline detection" claim was
downgraded to "honest-N/A": the code appeared to support it
(`EvictStaleNodes` at `aggregator.go:202-224`), but the goroutine calling it was
never wired, making the feature structurally impossible to reach in production.

## Reproduction

1. Start Pulse against a live AMS node.
2. Stop the AMS node (or block its REST API port) while keeping the OS alive.
3. Wait 3× the configured poll interval (default 15 s) for the node to become stale.
4. `GET /live` — the node card remains visible indefinitely. No `node_down` alert fires.

## Root Cause

`aggregator.EvictStaleNodes(threshold)` was implemented in VD-30 and correctly
deletes stale node entries, rebuilds the snapshot, and notifies subscribers. However,
**no goroutine in `serve.go` ever calls it**:

```go
// serve.go before the fix — only EvictStale (stream eviction) is wired:
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.agg.EvictStale()  // ← streams only; EvictStaleNodes is never called
        }
    }
}()
```

`EvictStale` (stream eviction) has a 30 s ticker. `EvictStaleNodes` (node eviction)
has nothing. The function was a dead-end: reachable only from the test suite, never
from the runtime.

**Compounding factor (D-087 design ruling):** Rung 2 of the detection ladder
(consecutive API-failure streak → `node_degraded` alert) previously refreshed
`LastSeenAt` on every failure event, because `onNodeStats` always called the
full-replace path regardless of event type. This meant that even if `EvictStaleNodes`
had been wired, failure events flowing from a frozen AMS node would have kept the
node "fresh" indefinitely — `EvictStaleNodes` would never evict it, and `node_down`
still could not fire. Both defects must be fixed together.

## Fix (as landed — S25, D-087)

Two coordinated changes:

**1. Failure-path in-place update (`aggregator.go` — `onNodeStats`):**

`onNodeStats` now distinguishes failure-streak events (`api_unreachable=true`) from
normal events. Failure events update **only** `ConsecAPIErrors` in-place on the
existing `LiveNodeStats` entry; they do NOT replace the struct, do NOT refresh
`LastSeenAt`, and do NOT create a new entry for unknown nodes. Normal events
(no `api_unreachable`) continue to do a full replace with `LastSeenAt=now`.

**2. `wireNodeEviction` goroutine (`serve.go` — `Start`):**

A new helper `wireNodeEviction(ctx, agg, pollInterval)` is extracted for
testability and called from `Start()` immediately after the stream-eviction loop:

```go
// BUG-011 fix (D-087): stale-node eviction loop.
wireNodeEviction(ctx, s.agg, s.pollInterval)
```

The goroutine runs with:
- `threshold = 3 × pollInterval` (default 15 s at 5 s poll): a node must miss
  three consecutive poll windows before eviction.
- `cadence = threshold / 2` (default 7.5 s): the ticker fires at least twice
  before a node can become stale, so eviction happens within one cadence window
  after the threshold is crossed.

## Evidence

### RED (before fix)

**restpoller streaks tests:** All 7 `TestAPIStreak_*` tests failed — the poller
emitted no `api_latency_ms`, no `api_unreachable` events, and no `consec_api_errors`
field at all (features not yet implemented):

```
--- FAIL: TestAPIStreak_RTTPresentOnSuccess (0.02s)
--- FAIL: TestAPIStreak_RTTAbsentOnFailure (3.00s)
--- FAIL: TestAPIStreak_IncrementsOnConsecutiveFailures (4.01s)
--- FAIL: TestAPIStreak_ResetsOnSuccess (5.00s)
--- FAIL: TestAPIStreak_BroadcastFailure_NoStreakIncrement (0.01s)
--- FAIL: TestAPIStreak_ClusterPath_RTTOnSuccess (0.01s)
--- FAIL: TestAPIStreak_ClusterPath_FailureEvent (3.00s)
FAIL github.com/pulse-analytics/pulse/server/internal/collector/restpoller  19.296s
```

**aggregator tests:** Build failure — `domain.LiveNodeStats` had no `ConsecAPIErrors`
or `APILatencyMS` fields:

```
aggregator_node_streak_test.go:89:11: node2.ConsecAPIErrors undefined
aggregator_node_streak_test.go:220:10: node.APILatencyMS undefined
FAIL github.com/pulse-analytics/pulse/server/internal/collector/aggregator [build failed]
```

**serve test:** Build failure — `wireNodeEviction` undefined:

```
cmd/pulse/serve_wiring_test.go:289:2: undefined: wireNodeEviction
FAIL github.com/pulse-analytics/pulse/server/cmd/pulse [build failed]
```

### GREEN (after fix)

All new pins pass:

```
--- PASS: TestAPIStreak_RTTPresentOnSuccess
--- PASS: TestAPIStreak_RTTAbsentOnFailure
--- PASS: TestAPIStreak_IncrementsOnConsecutiveFailures
--- PASS: TestAPIStreak_ResetsOnSuccess
--- PASS: TestAPIStreak_BroadcastFailure_NoStreakIncrement
--- PASS: TestAPIStreak_ClusterPath_RTTOnSuccess
--- PASS: TestAPIStreak_ClusterPath_FailureEvent
ok   github.com/pulse-analytics/pulse/server/internal/collector/restpoller

--- PASS: TestAggregator_FailureStreak_LastSeenAtFrozen
--- PASS: TestAggregator_FailureStreak_EvictStillWorks
--- PASS: TestAggregator_FailureStreak_UnknownNodeCreatesNothing
--- PASS: TestAggregator_NormalPath_ExtractsNewFields
--- PASS: TestAggregator_ConsecAPIErrors_PropagatesViaFailurePath
ok   github.com/pulse-analytics/pulse/server/internal/collector/aggregator

--- PASS: TestBUG011_NodeEviction_Wired
ok   github.com/pulse-analytics/pulse/server/cmd/pulse
```

Full suite:

```
ok   github.com/pulse-analytics/pulse/server/internal/collector      (all packages)
ok   github.com/pulse-analytics/pulse/server/cmd/pulse
ok   github.com/pulse-analytics/pulse/server/internal/anomaly
ok   github.com/pulse-analytics/pulse/server/internal/alert
ok   github.com/pulse-analytics/pulse/server/internal/api
```

## Impact

Without this fix, the three-rung AMS early-warning detection ladder for
ant-media/Ant-Media-Server#7926 was incomplete:

| Rung | Feature | Status before D-087 |
|------|---------|-------------------|
| 1 | `ams_api_latency_ms` → Welford anomaly flag | Working (D-086) |
| 2 | Consecutive API-failure streak → `node_degraded` (~15 s) | Working (D-087 rung-2 data added) |
| 3 | No node stats for 3×PollInterval → eviction → `node_down` | **BROKEN** — goroutine never wired |

After the fix, all three rungs are operational. A frozen AMS node that stops
responding to API calls will:
1. Trigger latency-creep anomaly flags as RTT climbs (rung 1).
2. Fire a `node_degraded` alert after 3 consecutive API failures (~15 s at 5 s poll) (rung 2).
3. Be evicted from the snapshot and fire `node_down` after 3×PollInterval of no
   successful stats (default 15 s, so `node_down` fires at most 15 s after
   `node_degraded`) (rung 3).
