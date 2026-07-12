# BUG-010: `GET /analytics/audience` reads an UNDECLARED `format=csv` query param — reverse-direction contract gap

**Severity:** low (feature invisible to the contract; no wrong data served)
**Component:** contracts (`contracts/openapi/pulse-api.yaml`) — the code is the
side telling the truth here
**Status:** FIXED (S22/D-084) — contract CR landed in S22; `?format` and `text/csv` response added to `/analytics/audience` GET; registry entry flipped to `paramProbe`; 29 conformance probes now pass.

## What it is

The reverse of the BUG-004/BUG-005/BUG-006 class: instead of a declared
parameter the handler ignores, this is an **implemented parameter the contract
never declares**. `handleAudienceAnalytics` (`server/internal/api/server.go:1048`)
implements CSV export via `?format=csv` (sets `text/csv`, a `Content-Disposition`
attachment, and streams the timeseries as CSV) — the in-code comment even says
"format=csv per spec" — but the `/analytics/audience` GET operation in
`contracts/openapi/pulse-api.yaml` declares no `format` parameter, and no
`format` query parameter exists anywhere in the spec. Generated clients
(`web/src/api/`) therefore cannot see or type the CSV export.

## Reproduction

`GET /api/v1/analytics/audience?format=csv` → 200 `text/csv` attachment.
The OpenAPI spec for the operation declares only
`from,to,app,stream,node,tenant,interval` and a JSON-only 200 response.

## Fix Suggestion

Declare `format` (enum `[json, csv]`, default `json`) on `/analytics/audience`
and add the `text/csv` 200 response content type — a CONTRACT change (INT-01
scope, contract-first per repo rules, then `npm run gen:api`). Alternatively
remove the CSV branch (not recommended — it closes PRD G5).

## Why the conformance gate does not pin this today

`param_conformance_test.go` enumerates DECLARED params (spec → registry); an
undeclared-but-read param is invisible to that direction of the audit. A future
iteration could grep handlers for `.Get(` / `FormValue(` calls and diff against
the spec — recorded as a possible S22+ extension in the session ledger.
