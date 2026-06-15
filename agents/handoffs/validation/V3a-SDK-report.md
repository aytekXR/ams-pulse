# V3a SDK-01 Fix Report

**Agent:** SDK-01 (Beacon SDK)
**Date:** 2026-06-15
**Branch:** main
**VDs applied:** VD-09, VD-12, VD-13

---

## Summary

All three assigned VDs fixed, tested, and verified. All acceptance criteria met.

---

## VD-09 — CRITICAL: SDK sends wrong ingest token header

**Finding:** `transport.ts` line 138 sent `X-Pulse-Token`; server and OpenAPI spec require `X-Pulse-Ingest-Token`. Every browser POST returned 401.

**Fix:** Changed header name in `sdk/beacon-js/src/transport.ts` line 138:
- Before: `'X-Pulse-Token': this.cfg.token`
- After: `'X-Pulse-Ingest-Token': this.cfg.token`

**Guard test added:** `transport.test.ts` — "Transport — header name (VD-09)" suite:
- Asserts `headers['X-Pulse-Ingest-Token']` equals the token value
- Asserts `headers['X-Pulse-Token']` is `undefined` (ensures the wrong name is never silently re-introduced)

---

## VD-12 — MAJOR: HlsAdapter never emits rebuffer_end

**Finding:** `_onBufferStalled` emitted `rebuffer_start` but `rebuffer_end` was only in `MediaElementAdapter._onPlaying`. Users using `attachHls()` as the primary path (per README) accumulated unbounded open stalls in ClickHouse.

**Fix in `sdk/beacon-js/src/hls.ts`:**
- Added `stallStartAt: number | null = null` field to `HlsAdapter`
- `_onBufferStalled` now records the stall start time (only if no stall already active)
- `_onFragBuffered` now emits `rebuffer_end` with computed `duration_ms` when a stall was active, then clears `stallStartAt`

**Guard tests added:** `hls.test.ts` — "HlsAdapter — VD-12: rebuffer_end after stall" suite (4 tests):
- Emits `rebuffer_end` on `FRAG_BUFFERED` after a stall
- Does NOT emit `rebuffer_end` on `FRAG_BUFFERED` when no stall is active
- Closes stall only once (second `FRAG_BUFFERED` without re-stall does not emit again)
- Handles multiple stall/resume cycles correctly (2 starts → 2 ends)

---

## VD-13 — MINOR: HlsAdapter level-switch always emits from_kbps=0 / to_kbps=0

**Finding:** `_onLevelSwitched` emitted `bitrate_change` with hardcoded `from_kbps: 0, to_kbps: 0`. `hls.levels[]` was never consulted. Every ABR switch polluted `rollup_qoe_1h` bitrate aggregates with 0→0 kbps.

**Fix in `sdk/beacon-js/src/hls.ts` and `sdk/beacon-js/src/types.ts`:**
- Added `levels?: Array<{ bitrate: number }>` to `HlsLike` interface
- Added `currentLevel = -1` field to `HlsAdapter` to track the previous level
- `_onLevelSwitched` now:
  - Reads `this.hls.levels[level].bitrate / 1000` (bps → kbps) for `to_kbps`
  - Reads `this.hls.levels[this.currentLevel].bitrate / 1000` for `from_kbps` (using tracked prior level)
  - Updates `this.currentLevel = level` after capturing `from_kbps`
  - Falls back gracefully to 0 when `levels` is not provided

**Guard tests added:** `hls.test.ts` — "HlsAdapter — VD-13: bitrate_change from_kbps/to_kbps" suite (4 tests):
- `to_kbps` populated from `hls.levels[level].bitrate`
- `from_kbps` populated from previous level bitrate across consecutive switches
- Graceful fallback to 0/0 when `hls.levels` is not provided
- `hls_level` field always present

---

## Acceptance criteria verification

| Command | Result |
|---------|--------|
| `npm run build` | PASS — ESM 11.44 KB, CJS 11.92 KB, IIFE 11.43 KB |
| `npm run size` | PASS — 3.52 KB gzipped (limit: 15 KB) |
| `npm run lint` | PASS — 0 errors |
| `npm run test` | PASS — 65 tests passed (5 test files; was 57 before, +8 new tests) |

All commands bounded with `timeout 180`.

---

## Files changed

- `sdk/beacon-js/src/transport.ts` — VD-09 header fix
- `sdk/beacon-js/src/hls.ts` — VD-12 rebuffer_end + VD-13 bitrate from levels
- `sdk/beacon-js/src/types.ts` — VD-13 HlsLike.levels field
- `sdk/beacon-js/src/__tests__/transport.test.ts` — VD-09 guard test
- `sdk/beacon-js/src/__tests__/hls.test.ts` — VD-12 + VD-13 guard tests (new file)
