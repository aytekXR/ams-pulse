# BUG-003: Probe scheduler produces near-duplicate result rows at periodic intervals

**Severity:** medium
**Component:** probe runner / scheduler (server-side)
**Status:** confirmed via TC-P-07 evidence (S18)

## Reproduction Steps

1. Create an HLS probe with `interval_s=30`.
2. Let it run for ~180 s (6 scheduled intervals).
3. `GET /probes/{id}/results` — observe 7 rows instead of 6, with 2 pairs
   of consecutive entries whose `ts` values differ by ≤ 1 ms (gap is 0 ms
   or 1 ms).

## Expected Behaviour

One result row per probe interval; consecutive rows should be separated by
approximately `interval_s * 1000` ms (±50% scheduling jitter is acceptable).
No two consecutive rows should have the same (or within 1 ms) timestamp.

## Actual (Evidence)

TC-P-07 run `S18-TC-P-07-20260711T141544Z`:

```
Result 1: ts=1783779400987
Result 2: ts=1783779430988   gap=30001 ms  OK
Result 3: ts=1783779460988   gap=30000 ms  OK
Result 4: ts=1783779460989   gap=1 ms      DUPLICATE   ← gap 3
Result 5: ts=1783779490990   gap=30001 ms  OK
Result 6: ts=1783779520989   gap=29999 ms  OK
Result 7: ts=1783779520989   gap=0 ms      DUPLICATE   ← gap 6
```

Two duplicate pairs appear at the 60 s and 120 s marks after the first
result, suggesting a periodic recurrence (every 2nd probe interval).

## Root Cause (Hypothesis)

The Pulse probe scheduler likely runs two concurrent execution paths for the
same probe (e.g. an "immediate run on create" goroutine and a periodic ticker
goroutine that share a common clock alignment).  At every other tick the two
paths fire within ≤ 1 ms of each other, producing a second nearly-identical
result row.  Because `probe_results` uses `MergeTree ORDER BY (probe_id, ts)`,
rows with distinct `ts` values (even by 1 ms) are stored as separate rows and
both are returned by the results API.

## Impact

- Results endpoint returns N+1 or N+2 rows per test window.
- Any consumer computing inter-result timing (e.g. the TC-P-07 gap check)
  will observe 0–1 ms "gaps" that look like scheduling failures.
- A monitoring dashboard showing "last check time" could display the
  duplicate timestamp rather than the real latest check.

## Fix Suggestion

1. **Dedup at insert**: before writing to `probe_results`, check whether a
   row for the same `probe_id` exists within the last `min(interval_s, 5) s`;
   if so, drop the duplicate.
2. **Scheduler guard**: use a per-probe mutex / single-flight primitive in the
   probe runner so only one goroutine executes a check at any given time.
3. **Results API dedup**: the `/probes/{id}/results` query could deduplicate
   consecutive rows with `|ts_current - ts_prev| < 1000 ms` before returning.

## Evidence

- `qa/realams/evidence/S18-TC-P-07-20260711T141544Z/timeline.txt`
- `qa/realams/evidence/S18-TC-P-07-20260711T141544Z/probe-ts-values.json`
- `qa/realams/evidence/S18-TC-P-07-20260711T141544Z/probe-results-all.json`
