# BUG-009: `GET /live/overview` + `GET /live/streams` — `tenant` (and `cursor`) accepted by the query layer and silently dropped

**Severity:** medium (OpenAPI contract violation — silent; multi-tenant data isolation
expectation not met by these endpoints)
**Component:** query (`server/internal/query/query.go`)
**Status:** OPEN — filed S21 / D-083 (2026-07-12) by the parameter-conformance
verification round (adversarial verifier finding on the S21 registry).

## What is different from BUG-006/BUG-007/BUG-008

Those bugs live in the **handler** layer (params never read or never passed on).
Here the handler layer is CORRECT — `handleLiveOverview`/`handleLiveStreams` read
`tenant` (and `cursor`) and pass them to the query layer — but
**`query.LiveOverview` and `query.LiveStreams` accept the arguments and never use
them**, so the caller-visible effect is identical to the declared-but-ignored
class. A one-layer-deeper instance: parameter-conformance auditing must follow
the value to where it has an observable effect, not stop at the parse site.

## Reproduction Steps

1. `GET /api/v1/live/overview?tenant=acme` vs `GET /api/v1/live/overview` —
   byte-identical responses regardless of tenant assignment.
2. `GET /api/v1/live/streams?tenant=acme` — same result set as without.
3. `GET /api/v1/live/streams?cursor=<next_cursor from page 1>` — returns page 1
   again; paging past the first page is impossible.

## Actual (code evidence)

- `query.LiveOverview(ctx, app, nodeID, tenant)` (`query.go:54`) — `tenant`
  appears ONLY in the signature; the stream filter loop checks `app`/`nodeID`
  only.
- `query.LiveStreams(ctx, app, nodeID, tenant, limit, cursor)` (`query.go:145`) —
  `tenant` never used; `cursor` explicitly stubbed at `query.go:203`:
  `_ = cursor // wave 1: ignore cursor, return first page` (while `limit` IS
  honored and `next_cursor` IS emitted — so the contract *advertises* paging that
  can never advance).

## Expected (Contract)

`#/components/parameters/tenant` filters results by tenant; `cursor` continues
from the previous page's `next_cursor`.

## Fix Suggestion

Wire `tenant` filtering into both methods once tenant→stream assignment
exists in the live snapshot (F6 multi-tenancy); implement offset-cursor decode
in `LiveStreams` (the emit side already exists). Both are query-layer-only
changes; no contract change.

## Pinned by

`server/internal/api/param_conformance_test.go` — registry entries
`GET /live/overview ?tenant`, `GET /live/streams ?tenant`,
`GET /live/streams ?cursor` (disposition `known-violation`, bugRef BUG-009).
