# BUG-004: `GET /api/v1/qoe/ingest` declares `from`/`to` parameters but ignores them

**Severity:** medium (OpenAPI contract violation — silent, wrong data for windowed queries)
**Component:** api
**Status:** **FIXED (S20 / D-082, 2026-07-12)** — see "Fix (as landed)" below.
One residual carved out as **BUG-005** (`interval` param, same class).

## Reproduction Steps

1. `contracts/openapi/pulse-api.yaml` — the `/qoe/ingest` GET declares
   `$ref` parameters `from`, `to` (and `app`).
2. Publish at 2000 kbps, then re-publish the same stream at 200 kbps.
3. `GET /api/v1/qoe/ingest?from=<after-republish-ms>&to=<now-ms>`.
4. The response is identical with and without `from`/`to`: the top-level
   `bitrate_kbps` comes from the live aggregator snapshot and `timeseries`
   from an ALL-TIME ClickHouse query — the handler never reads the params
   (`IngestTimeseries` is invoked with no From/To).

## Expected (Contract)

Declared query parameters constrain the response window; `timeseries` should
contain only buckets within `[from, to]`.

## Actual (Pulse Output)

`from`/`to` are accepted and silently discarded. A consumer asking for the
post-degradation window receives era-mixed buckets (observed: 60 s bucket
averaging 2000-era and 200-era samples → ~649 kbps, S18 TC-I-04 root cause).

## Root Cause

Handler wiring gap: the `/qoe/ingest` handler never parses `from`/`to` and
calls the query layer without a time window, while the OpenAPI spec (from
which web types are generated) advertises the parameters.

## Fix Suggestion

Either parse and pass `from`/`to` into `IngestTimeseries` (preferred —
matches every other windowed endpoint), or remove the parameter refs from the
`/qoe/ingest` spec entry. Add a response-body contract test either way
(the CI spec-lint cannot catch declared-but-ignored params).

## Evidence

- S18 TC-I-04 diagnosis (`qa/realams/evidence/S18-TC-I-04-*/`): timeseries
  window unaffected by `from`; scenario now reads the live-aggregator field.
- decisions.md D-080 (S18 fix-round notes).

---

## Fix (as landed — S20, D-082, 2026-07-12)

**Approach:** implementation catches up to the contract. `contracts/openapi/pulse-api.yaml`
is **UNCHANGED** (verified: `npm run gen:api` + `git diff --exit-code` on
`web/src/api/` and `contracts/` → clean, no generated-type drift).

- `handleIngestHealth` (`server/internal/api/server.go`) now reads `from`, `to`,
  `app`, `stream`, `node` from the query string. `from`/`to` are plumbed into
  `query.IngestTimeseriesParams{From, To}`; `app`/`stream`/`node` filter WHICH
  active streams are returned (previously every active stream was returned).
- New `parseTimeParam` helper returns a **zero** `time.Time` for absent/unparseable
  input — `parseTimeRange` could not be reused because it applies a 7-day default,
  which would have silently introduced a window where there was none. The query
  layer already guards `From`/`To` with `IsZero()`, so absent params ⇒ **no time
  filter** ⇒ byte-identical back-compat behavior (pinned by a test).
- Seam: an additive `IngestQuerier` interface + `iqsvc` field on `Server`
  (nil-guarded assignment to avoid the nil-concrete-pointer-in-interface trap).
  `qsvc` and all its existing callers are untouched.

**Tests (TDD red→green):** `server/internal/api/vd20b_vd21_ingest_test.go`
— `TestBUG004_IngestHealth_HonorsTimeRange` (5 subtests: epoch-ms, RFC3339,
absent-both, only-from, only-to), `TestBUG004_IngestHealth_AppStreamNodeFilter`
(7 subtests), `TestBUG004_IngestHealth_BackCompat_NoParams`. Red output captured
before the fix (0 `IngestTimeseries` calls observed); green after.
Gates: `go test -race` api → **0 FAIL / 0 SKIP**, api coverage 76.9% → **78.0%**,
Go total **74.8%** (floor 70.2).

**Real-world impact of the bug (found during the fix):** `web/src/api/client.ts`
`getIngestHealth` sends `from=now-15min&to=now` on every Ingest-page load, so the
**production Ingest dashboard was receiving all-time, era-mixed buckets** — this
was never a test-only defect.

## Residual → BUG-005 (`interval` declared but ignored — same class)

The shared OpenAPI `interval` parameter (enum `[hour, day]`, default `day`) is
**still not read** by `/qoe/ingest`. `IngestTimeseriesParams.BucketSeconds` is left
at `0`, so `IngestTimeseries` falls back to its internal 60 s bucket default —
a caller asking for `interval=day` silently receives one-minute buckets.

This is the **identical defect class** as BUG-004 (declared-but-ignored parameter)
and was deliberately scoped OUT of the S20 fix rather than left undocumented.
Natural mapping: `hour → BucketSeconds=3600`, `day → BucketSeconds=86400` — but
note the bucket width interacts with the F4 "degradation visible within 15 s"
acceptance criterion, so the default must stay 60 s when `interval` is absent.

**Lesson (why the response-body/parameter contract tests in RESUME-PROMPT §6 matter):**
CI lints the OpenAPI spec but never asserts that handlers honor what the spec
declares — so BUG-004 and BUG-005 were both invisible to the pipeline. A
parameter-conformance test would have caught both at authoring time.
