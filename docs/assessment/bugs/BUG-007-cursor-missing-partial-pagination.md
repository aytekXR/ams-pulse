# BUG-007: cursor declared but unimplemented on endpoints that otherwise read query params correctly

**Severity:** low-medium
**Component:** api / store (meta.Store) / query service
**Status:** OPEN — filed S21 / D-083, 2026-07-12 (parameter-conformance sweep)

## Summary

Two endpoints read most of their declared query params correctly but silently
drop the `cursor` pagination param. Callers cannot page past the first `limit`
results even when the result set exceeds `limit`.

## Affected Endpoints

### `GET /alerts/history` (server.go:~1451-1463)

Reads `from`, `to`, `limit`, `rule_id`, `state` via `parseTimeRange` / `q.Get`.
`cursor` is absent from:
- The handler parse block (no `q.Get("cursor")` call).
- The `store.ListAlertHistory` signature — response always returns
  `next_cursor: null`.

### `GET /probes/{probeId}/results` (wave3.go:~336-354)

Reads `from`, `to` via `parseTimeRange` and `limit` via `q.Get`.
`cursor` is absent from:
- The handler parse block.
- The `qsvc.QueryProbeResults(ctx, probeID, from, to, limit)` signature —
  response always returns `next_cursor: null`.

## Root Cause

Cursor threading was omitted from the store/service-layer method signatures at
authoring time. Unlike BUG-006 (where both `limit` and `cursor` are missing),
these endpoints partially implemented pagination but stopped at page 1: `limit`
IS read and applied, but the cursor needed to fetch the next page is never
produced or consumed.

## Impact

A caller that pages through `GET /alerts/history` with `limit=50` will see the
first 50 history entries repeatedly regardless of which cursor value it sends.
`meta.next_cursor` is always `null`.

## Fix Suggestion

1. Add `cursor string` parameter to `store.ListAlertHistory` and
   `qsvc.QueryProbeResults` method signatures.
2. Update SQL queries to apply keyset-cursor pagination.
3. Update handlers to read `q.Get("cursor")` and pass through.
4. Return a non-null `next_cursor` when more rows exist beyond the limit.

See also BUG-006 for the broader class of endpoints where both `limit` and
`cursor` are missing from the store layer.
