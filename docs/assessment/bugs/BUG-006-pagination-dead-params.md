# BUG-006: Pagination params (limit + cursor) declared in OpenAPI but unimplemented across all list endpoints

**Severity:** medium
**Component:** api / store (meta.Store)
**Status:** OPEN — filed S21 / D-083, 2026-07-12 (parameter-conformance sweep)

## Summary

Eight `GET` list endpoints declare `$ref limit` and `$ref cursor` in the OpenAPI
spec but pass neither to the store layer. All store methods take no pagination
args and return unbounded full result sets; all handlers hardcode
`next_cursor: null` in the response.

## Affected Endpoints

| Endpoint | Store method | Missing args |
|---|---|---|
| `GET /alerts/rules` | `ListAlertRules(ctx)` (server.go:~1257) | limit, cursor |
| `GET /alerts/channels` | `ListAlertChannels(ctx)` (server.go:~1341) | limit, cursor |
| `GET /reports/schedules` | `ListReportSchedules(ctx)` (reports_wave2.go:~74) | limit, cursor |
| `GET /probes` | `ListProbes(ctx)` (wave3.go:~121) | limit, cursor |
| `GET /admin/sources` | `ListAMSSources(ctx)` (server.go:~1508) | limit, cursor |
| `GET /admin/tokens` | `ListTokens(ctx, kind)` (server.go:~1727) | limit, cursor (kind IS read correctly) |
| `GET /admin/users` | `ListUsers(ctx)` (server.go:~1788) | limit, cursor |
| `GET /admin/tenants` | `ListTenants(ctx)` (reports_wave2.go:~181) | limit, cursor |

`GET /admin/tokens` is a partial instance: `kind` IS read correctly, but `limit`
and `cursor` are not.

## Root Cause

The `meta.Store` method signatures were scaffolded without pagination arguments,
and the handler wiring was never added. This is not a handler-only gap — the
store-layer method signatures themselves need to be extended with `(limit int,
cursor string)` parameters, and the SQL queries need `LIMIT`/cursor clauses.

## Impact

A caller passing `limit=1` to any of these endpoints receives the **full result
set** regardless of the limit value. On large deployments this violates the
declared `maximum: 500` contract and can produce unbounded memory allocations
in the handler. `next_cursor` is always `null` even when results are truncated.

## Reproduction

```
GET /api/v1/alerts/rules?limit=1
```

Response `items` count equals the total rule count regardless of `limit`;
`meta.next_cursor` is always `null`.

## Fix Suggestion

1. Extend each `meta.Store` list method signature to accept `(limit int, cursor string)`.
2. Update SQL queries to apply `LIMIT` and keyset-cursor pagination.
3. Update handlers to read `q.Get("limit")` / `q.Get("cursor")` and pass through.
4. Update the `meta.Store` interface and all test fakes.

BUG-007 tracks the subset of endpoints that partially implement pagination
(cursor missing but most other params correct).
