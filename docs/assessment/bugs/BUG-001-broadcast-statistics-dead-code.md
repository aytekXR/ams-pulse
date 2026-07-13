# BUG-001: `amsclient.BroadcastStatistics()` is dead code — defined, tested, never called at runtime

**Severity:** low
**Component:** amsclient
**Status:** FIXED (S26, D-088)

## Disposition

Resolved by deletion in S26 (D-088). The `BroadcastStatisticsDTO` struct and the
`BroadcastStatistics` method were removed from `server/pkg/amsclient/client.go`, and
the companion test `TestBroadcastStatistics_RealFields` and its fixture
`testdata/broadcast_statistics_real.json` were deleted alongside them.

The live pipeline was never affected: all viewer counts have always been sourced from
the inline `BroadcastDTO` fields on the 5 s poll path (`normalize.go` sums
`hlsViewerCount + webRTCViewerCount + rtmpViewerCount + dashViewerCount`). No runtime
behavior changed.

The real-AMS wire shape of the endpoint — including the observed
`totalRTMPWatchersCount: -1` ("untracked") quirk on AMS 3.0.3 — remains documented in
`agents/handoffs/real-ams-captures/broadcast-statistics_test123.json` for reference.
The `qa/mock-ams` `/statistics` stub is retained deliberately: it mirrors the real AMS
REST surface and may be useful for future work.

## Original Reproduction Steps

1. `codegraph explore "BroadcastStatistics callers"` (or `grep -rn "BroadcastStatistics" server/`).
2. Observe: definition at `server/pkg/amsclient/client.go:483`, exactly one caller —
   `server/pkg/amsclient/client_test.go:625` (a unit test). No caller in
   `restpoller`, `normalize`, or any runtime path.

## Expected (AMS Ground Truth)

AMS exposes `GET /{app}/rest/v2/broadcasts/{id}/broadcast-statistics` with
`totalHLSWatchersCount`, `totalWebRTCWatchersCount`, `totalRTMPWatchersCount`
(capture: `agents/handoffs/real-ams-captures/broadcast-statistics_test123.json`).
A client method wrapping it should either feed the pipeline or not exist.

## Actual (Pulse Output — historical)

The method was compiled, unit-tested, and never invoked. All viewer counts came
from the inline BroadcastDTO fields on the 5 s poll path
(`normalize.go:83` sum of `hlsViewerCount + webRTCViewerCount + rtmpViewerCount
+ dashViewerCount`).

## Root Cause

The poll path standardized on inline BroadcastDTO counts (cheaper: one list call
per app vs one statistics call per stream). The dedicated statistics wrapper was
implemented alongside but never wired, and nothing removed it.

## Evidence

- `docs/assessment/capability-map.md` § AV Triage — S17 (AV-02: CONFIRMED)
- `agents/handoffs/real-ams-captures/broadcast-statistics_test123.json`
