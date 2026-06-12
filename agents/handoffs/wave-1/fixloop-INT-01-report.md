# Fix-loop Completion Report — INT-01 (Wave-1 contract CRs)

**Agent:** INT-01  
**Date:** 2026-06-12  
**Triggered by:** ORCH-00 decision D-006 (wave-1 gate fix-loop)  
**Work items:** CR-1, CR-2, CR-3, CR-4

---

## Status: DONE

All four ORCH-00-approved contract change requests applied. Acceptance checks pass.

---

## Changes made

### CR-1 — `AlertRule.name` (required string)

**File:** `contracts/openapi/pulse-api.yaml`
- Added `name` to `AlertRule.required` array (`[id, name, metric, …]`).
- Added `name` property (type: string) with description to `AlertRule` schema.
- Added `name` to `AlertRuleWrite.required` array.
- Added `name` property to `AlertRuleWrite` schema.

**File:** `contracts/db/meta/0001_init.sql`
- Added `name TEXT NOT NULL` column to `alert_rules` table (positioned after `id`,
  before `metric`). Includes inline comment documenting its purpose.

### CR-2 — `AlertRule.enabled` (boolean, default true)

**File:** `contracts/openapi/pulse-api.yaml`
- Added `enabled` to `AlertRule.required` array alongside `muted`.
- Added `enabled` property (type: boolean) to `AlertRule` with a description
  documenting the distinction from `muted`:
  - `enabled=false` → rule not evaluated at all.
  - `muted=true` → rule evaluated, firings recorded, notifications suppressed.
- Added `enabled` property to `AlertRuleWrite` (default: true) with same
  distinction documented.
- Existing `muted` field descriptions updated on both schemas to reference the
  enabled/muted distinction explicitly.

**File:** `contracts/db/meta/0001_init.sql`
- Added `enabled INTEGER NOT NULL DEFAULT 1` column to `alert_rules` table
  (positioned before `muted`).
- Table-level comment updated to document enabled vs muted semantics.

### CR-3 — `POST /admin/sources/{sourceId}/test` + `AmsSourceStatus` schema

**File:** `contracts/openapi/pulse-api.yaml`
- Added path `/admin/sources/{sourceId}/test` with `POST testSource` operation
  under the `admin` tag.
- Responses: 200 (`AmsSourceStatus`), 401, 404, 500.
- Description notes that server-side implementation is deferred to wave 2.
- Added `AmsSourceStatus` schema to `components/schemas`:
  - Required field: `reachable` (boolean)
  - Optional fields: `version` (string|null), `latency_ms` (integer|null),
    `error` (string|null)

### CR-4 — `contracts/README.md` codegen path fix

**File:** `contracts/README.md`
- Changed `../../contracts/openapi/pulse-api.yaml` → `../contracts/openapi/pulse-api.yaml`
  in the `gen:api` npm script example.
  Rationale: `web/` sits one level below repo root, not two.

---

## Acceptance checks — verified outputs

### OpenAPI lint (zero errors required)

```
$ npx @redocly/cli lint contracts/openapi/pulse-api.yaml
validating contracts/openapi/pulse-api.yaml...
contracts/openapi/pulse-api.yaml: validated in 32ms
Woohoo! Your API description is valid. 🎉
```

Result: **0 errors, 0 warnings**

### Meta DDL — SQLite :memory: execution

```
$ sqlite3 :memory: < contracts/db/meta/0001_init.sql && echo "DDL OK"
DDL OK
```

Result: **clean execution, no errors**

### Budget regression suite (all 8 tests)

```
B-01: Stream visibility latency 1.5011065s ≤ 10s          PASS
B-02: Viewer count normalization sums all protocols         PASS
B-03: Alert latency 15s ≤ 30s                              PASS
B-04: ClickHouse DDL 14 create statements                   PASS
B-05: Meta DDL 14 CREATE TABLE statements                   PASS  (unchanged — 14 tables)
B-06: CGO_ENABLED=0 go build ./... green                    PASS
B-07: Web bundle 696.79 kB pre-gzip                         PASS
B-08: OpenAPI spec valid (0 errors, 0 warnings)             PASS
```

All 8 pass — no regressions introduced.

---

## Downstream notes for BE-02

BE-02 must update the `alert_rules` store/API layer to:
1. Accept and persist `name` (required; validate non-empty).
2. Accept and persist `enabled` (default true on create; toggle via PUT).
3. Update the alert evaluator to skip rules where `enabled=0` entirely
   (before any metric fetch, not just before notification dispatch).

FE-01 may consume `name` for display and `enabled` for the toggle control.
The `POST /admin/sources/{sourceId}/test` endpoint should return a stub 200
with `{"reachable": false, "error": "not implemented"}` until wave 2
implements the collector/amsclient wiring.

---

## Files changed

| File | Change |
|------|--------|
| `contracts/openapi/pulse-api.yaml` | CR-1 (name), CR-2 (enabled), CR-3 (test endpoint + schema) |
| `contracts/db/meta/0001_init.sql` | CR-1 (name column), CR-2 (enabled column) in alert_rules |
| `contracts/README.md` | CR-4 (codegen path fix) |
