# BUG-003: Probe scheduler produces near-duplicate result rows at periodic intervals

**Severity:** medium
**Component:** probe runner / scheduler (server-side)
**Status:** **FIXED (S20 / D-082, 2026-07-12)** — see "Root cause (ACTUAL)" +
"Fix (as landed)" below. The filed hypothesis below was **WRONG**; kept for provenance.

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

---

## Root cause (ACTUAL — S20, D-082) — the filed hypothesis above was WRONG

The hypothesis above guessed an **"immediate run on create" goroutine racing a
periodic ticker**. No such path exists. The real mechanism, found by reading the
scheduler (D-042: read the code, never bump the timeout):

`Runner.Run` (`server/internal/prober/prober.go`) reloads the enabled-probe list
on a **60 s refresh ticker** and called `spawnProbe(p)` for **every** probe on
**every** tick. `spawnProbe` **unconditionally cancelled and respawned** that
probe's scheduler goroutine — even when nothing about the probe had changed. The
respawned `runProbeScheduler` waits only `jitter(interval)` before its first fire,
and in **production** `serve.go` constructs `prober.New(prober.Config{Workers: 4}, …)`,
leaving `MaxJitterFraction` at its zero value ⇒ `jitter()` returns **0** ⇒ the
respawned goroutine fires **immediately** — landing 0–1 ms on top of the original
goroutine's own periodic fire, which is phase-aligned to the same start instant.

That is exactly the evidence signature: a duplicate pair **every 60 s** (the
refresh period), not every probe interval. The 30 s-interval probe in TC-P-07
duplicated at the 60 s and 120 s marks — the two refresh ticks inside the window.

**Why this matters beyond duplicate rows:** every refresh also *reset every
probe's phase*, so probe timing in production was never actually periodic.

## Fix (as landed — S20, D-082)

Mechanism fix, not a dedup band-aid (all three "Fix Suggestion" options above were
**rejected**: insert-time dedup, results-API dedup, and a per-probe mutex all hide
the duplicate instead of removing it, and none fix the phase reset):

- `probeEntry` now stores the `domain.ProbeConfig` alongside the cancel func.
  `spawnProbe` compares whole-struct (`e.config == p`; `ProbeConfig` is all-scalar
  and comparable) and **returns early when the config is unchanged** — the existing
  goroutine keeps running with its established phase. A **changed** config still
  cancels + respawns; a **removed** probe is still cancelled and deleted.
- The refresh loop moved from a real `time.NewTicker(60*time.Second)` to a re-armed
  `r.clock.After(cfg.RefreshInterval)`, so a `FakeClock` can drive it
  deterministically. New `Config.RefreshInterval` defaults to 60 s when ≤ 0, so
  **production behavior is unchanged** (`serve.go` passes nothing).

**Tests:** `server/internal/prober/prober_bug003_test.go` —
`TestBUG003_NoRespawnOnUnchangedConfig` (the regression pin),
`TestBUG003_ChangedConfigRespawns`, `TestBUG003_RemovedProbeStops`,
`TestBUG003_N24_FirstFireImmediate` (guards the N24 <100 ms first-result budget —
a new probe must still fire immediately; the fix adds no startup delay).

**Red→green proof (ORCH-run, the authoring agent died before producing it):** the
pin was re-run against the **pre-fix** `spawnProbe` in an isolated pristine-copy
tree and fails with the bug's exact signature —
`expected exactly 4 probe fires in 100 virtual seconds (1 immediate + 3 interval;
refresh at T=100 must NOT respawn an unchanged probe), got 5`. Green on the fix.
Gates: `go test -race -count=3 ./internal/prober/...` → ok, 0 FAIL / 0 SKIP;
prober coverage 72.6% → **74.3%**; Go total **74.8%** (floor 70.2).
