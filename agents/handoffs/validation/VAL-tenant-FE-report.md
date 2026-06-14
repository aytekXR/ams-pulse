# VAL-tenant-FE-report — FE-01 Tenant CRUD UI

**Agent:** FE-01
**Date:** 2026-06-14
**Commit:** `a6ed6ba9b9023c3f0ed77610325c97de58f7e2c6`
**Work order:** VAL-TENANT (wo-tenant-crud.md)

---

## Status: DONE

All acceptance criteria verified. Tenant CRUD UI integrated into `/reports` tenants tab.

---

## Acceptance criteria — verified outputs

### 1. `npm run generate:api` — regenerated first

```
$ cd web && npm run generate:api

> @pulse/web@0.1.0 generate:api
> npm run gen:api

> @pulse/web@0.1.0 gen:api
> openapi-typescript ../contracts/openapi/pulse-api.yaml -o src/lib/api/schema.d.ts

✨ openapi-typescript 7.13.0
🚀 ../contracts/openapi/pulse-api.yaml → src/lib/api/schema.d.ts [71.1ms]
```

Result: **PASS** — schema.d.ts regenerated with Tenant/TenantWrite/TenantList +
listTenants/createTenant/getTenant/updateTenant/deleteTenant operations.

### 2. `npm run build && npm run lint && npm run test` — all green

```
$ cd web && npm run build

> @pulse/web@0.1.0 build
> tsc -b && vite build

vite v6.4.3 building for production...
✓ 643 modules transformed.
dist/index.html                   0.41 kB │ gzip:   0.29 kB
dist/assets/index-DjajK3KD.css    1.29 kB │ gzip:   0.66 kB
dist/assets/index-Ckngr_Y1.js   782.18 kB │ gzip: 223.12 kB
✓ built in 1.33s

$ cd web && npm run lint
> eslint src
(0 errors, 0 warnings)

$ cd web && npm run test

> @pulse/web@0.1.0 test
> vitest run

 ✓ src/components/__tests__/AuthGate.test.tsx (5 tests)
 ✓ src/features/live/__tests__/StreamsTable.test.tsx (7 tests)
 ✓ src/features/alerts/__tests__/AlertRuleForm.test.tsx (6 tests)
 ✓ src/features/reports/__tests__/ReportsPage.test.tsx (11 tests)
 ✓ src/features/fleet/__tests__/FleetPage.test.tsx (10 tests)
 ✓ src/features/anomalies/__tests__/AnomaliesPage.test.tsx (17 tests)
 ✓ src/features/qoe/__tests__/QoePage.test.tsx (11 tests)
 ✓ src/features/reports/__tests__/TenantsTab.test.tsx (18 tests)
 ✓ src/features/probes/__tests__/ProbesPage.test.tsx (34 tests)
 ✓ src/features/live/__tests__/LiveSocket.test.ts (8 tests)

 Test Files  10 passed (10)
      Tests  127 passed (127)
   Duration  2.55s
```

Result: **ALL GREEN** — build, lint, 127/127 tests (18 new tenant tests).

### 3. No hand-rolled API shapes — git-grep proof

```
$ cd web && git grep -rn "interface.*Response\|interface.*Request" -- "src/" \
    | grep -v "schema.d.ts\|types.ts\|WsMessage"
(no output — exit code 1 = no matches)
```

All Tenant types flow through `schema.d.ts` (generated) → `src/lib/api/types.ts`
(re-export aliases) → components. Hand-rolled shapes: 0.

### 4. Component tests

New test file: `web/src/features/reports/__tests__/TenantsTab.test.tsx` (18 tests)

| Group | Tests | Coverage |
|---|---|---|
| TenantForm validation | 8 | name required, whitespace, stream_pattern ok, meta tag ok, both ok, no matcher error, partial tag key only, partial tag value only |
| CRUD rendering (tier gate) | 2 | tenants tab visible on pro; upsell hides tab on free |
| CRUD rendering (list) | 3 | tenant list renders, stream_pattern badge, meta tag badge |
| CRUD rendering (empty state) | 1 | "No tenants configured" |
| CRUD rendering (interactions) | 4 | new form shown, delete confirm shown, createTenant called, deleteTenant called |

---

## Surfaces built

### Tenants CRUD tab (`/reports` → `tenants` tab)

**File:** `web/src/features/reports/ReportsPage.tsx`
**Test file:** `web/src/features/reports/__tests__/TenantsTab.test.tsx`

Components added:

**TenantForm** — create/edit form:
- `name` field (required, unique constraint server-enforced → 409)
- `stream_pattern` field (SQL LIKE / regex on stream_id)
- `meta_tag_key` + `meta_tag_value` fields (paired — both required for tag match)
- Client validation: name required; at least one matcher (pattern OR key+value pair)
- Server error surface: ApiError message shown in ErrorBanner
- Renders as "New tenant" or "Edit tenant" depending on `initial` prop

**DeleteConfirm** — confirmation dialog:
- Shows tenant name in confirmation text
- Explains that existing usage rows retain the tenant label
- Cancel / Delete buttons; Delete disabled while in-flight

**TenantsTab** — tab content:
- Loads `adminApi.listTenants()` on mount
- Loading / error / empty states (with retry on error)
- List of tenant cards with pattern/tag badges + Edit/Delete buttons
- Create form toggled by "+ New tenant" button
- Edit form inline on the tenant card (replaces row)

**ReportsPage** — integration:
- Added `"tenants"` to Tab union type and tab bar
- Business-tier gate: `tier === "free"` → `TierUpsell` shown, tabs hidden
- Tab renders `<TenantsTab onToast={toast} />`

```
┌─ Reports ──────────────────────────────────────────────────── [business] ─┐
│  [usage] [schedules] [tenants]                                             │
│                                                                            │
│  Tenants match streams by pattern or beacon meta tag for billing.         │
│                                                  [+ New tenant]           │
│                                                                            │
│  ┌─ Acme Corp ─────────────────────────────────────── [Edit] [Delete] ──┐ │
│  │  [pattern: live/acme-%]                                               │ │
│  │  Created 1/1/2024                                                     │ │
│  └────────────────────────────────────────────────────────────────────── ┘ │
│                                                                            │
│  ┌─ Beta Inc ──────────────────────────────────────── [Edit] [Delete] ──┐ │
│  │  [tenant_id=beta]                                                     │ │
│  │  Created 1/1/2024                                                     │ │
│  └────────────────────────────────────────────────────────────────────── ┘ │
└────────────────────────────────────────────────────────────────────────────┘
```

Delete confirm dialog:
```
┌─ Delete tenant ────────────────────────────────────────────────────────────┐
│  Are you sure you want to delete Acme Corp?                                │
│  Existing usage rows will retain the tenant label but no new streams       │
│  will be matched.                                                           │
│                                                          [Cancel] [Delete] │
└────────────────────────────────────────────────────────────────────────────┘
```

Create/edit form:
```
┌─ New tenant ───────────────────────────────────────────────────────────────┐
│  Name *                                                                     │
│  [Acme Corp                          ]                                     │
│                                                                            │
│  Provide at least one matcher: stream pattern OR both meta tag key/value.  │
│                                                                            │
│  Stream pattern (SQL LIKE / regex on stream_id)                            │
│  [live/acme-%                         ]                                    │
│                                                                            │
│  Meta tag key              Meta tag value                                  │
│  [tenant_id    ]           [acme       ]                                   │
│                                                                            │
│                                             [Cancel] [Save tenant]         │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## Files changed (all web/ scope — FE-01 only)

### Modified files
- `web/src/lib/api/schema.d.ts` — regenerated: Tenant schema + tenant CRUD operations
- `web/src/lib/api/types.ts` — added `Tenant`, `TenantWrite`, `TenantList` re-exports
- `web/src/api/client.ts` — added `listTenants`, `createTenant`, `getTenant`,
  `updateTenant`, `deleteTenant` to `adminApi`
- `web/src/features/reports/ReportsPage.tsx` — added `TenantForm`, `DeleteConfirm`,
  `TenantsTab` components; wired "tenants" tab into ReportsPage

### New files
- `web/src/features/reports/__tests__/TenantsTab.test.tsx` — 18 tests

---

## Numeric evidence

| Metric | Result |
|---|---|
| Build | PASS (643 modules, 782 KB JS / 223 KB gzip) |
| Lint | 0 errors, 0 warnings |
| Tests | 127/127 pass (18 new tenant tests) |
| Hand-rolled API shapes | 0 (git-grep proof) |
| D-011 compliance | Staged 5 explicit paths; no git add -A/. |

---

## Gaps / notes

- **Live stream preview**: The work order mentions "live preview of matched streams
  where cheap". This requires a server-side endpoint or the ability to do a
  stream_pattern lookup against the live streams list. No such endpoint exists in
  the current frozen spec. The current UI shows the configured pattern/tag text —
  the actual preview would need either a dedicated `/admin/tenants/{id}/preview`
  endpoint (server-side) or client-side filtering of the `/live/streams` list
  (not implemented to avoid unnecessary data loading on every tenant card). Filed
  as a gap for BE-02 to add a preview endpoint in a future wave if desired.

- All other acceptance criteria fully met.
