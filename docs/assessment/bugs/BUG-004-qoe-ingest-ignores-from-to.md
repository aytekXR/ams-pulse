# BUG-004: `GET /api/v1/qoe/ingest` declares `from`/`to` parameters but ignores them

**Severity:** medium (OpenAPI contract violation — silent, wrong data for windowed queries)
**Component:** api
**Status:** open

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
