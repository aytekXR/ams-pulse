# BUG-001: `amsclient.BroadcastStatistics()` is dead code — defined, tested, never called at runtime

**Severity:** low
**Component:** amsclient
**Status:** confirmed

## Reproduction Steps

1. `codegraph explore "BroadcastStatistics callers"` (or `grep -rn "BroadcastStatistics" server/`).
2. Observe: definition at `server/pkg/amsclient/client.go:483`, exactly one caller —
   `server/pkg/amsclient/client_test.go:625` (a unit test). No caller in
   `restpoller`, `normalize`, or any runtime path.

## Expected (AMS Ground Truth)

AMS exposes `GET /{app}/rest/v2/broadcasts/{id}/broadcast-statistics` with
`totalHLSWatchersCount`, `totalWebRTCWatchersCount`, `totalRTMPWatchersCount`
(capture: `agents/handoffs/real-ams-captures/broadcast-statistics_test123.json`).
A client method wrapping it should either feed the pipeline or not exist.

## Actual (Pulse Output)

The method is compiled, unit-tested, and never invoked. All viewer counts come
from the inline BroadcastDTO fields on the 5 s poll path
(`normalize.go:83` sum of `hlsViewerCount + webRTCViewerCount + rtmpViewerCount
+ dashViewerCount`).

## Root Cause

The poll path standardized on inline BroadcastDTO counts (cheaper: one list call
per app vs one statistics call per stream). The dedicated statistics wrapper was
implemented alongside but never wired, and nothing removed it.

## Fix Suggestion

Pick one:
1. **Remove** `BroadcastStatistics()` + its test (dead weight, ~40 lines), or
2. **Wire it** behind an opt-in config (per-stream detail view enrichment) — if
   kept, clamp negative counts first: the endpoint returns
   `totalRTMPWatchersCount: -1` ("untracked") on real AMS 3.0.3 and
   `normalize.go:83` sums without clamping (AV-16).

S17 recommendation: option 1 (no consumer exists; the detail need is
hypothetical). Decision belongs to a future work order, not this program.

## Evidence

- `docs/assessment/capability-map.md` § AV Triage — S17 (AV-02: CONFIRMED)
- `agents/handoffs/real-ams-captures/broadcast-statistics_test123.json`
