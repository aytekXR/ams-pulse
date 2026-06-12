# Fix-loop Completion Report — FE-01 (Wave-1 CRs)

**Agent:** FE-01  
**Date:** 2026-06-12  
**Triggered by:** ORCH-00 decision D-006 (wave-1 fix-loop)  
**Work items:** CR-1 (AlertRule.name), CR-2 (AlertRule.enabled), CR-3 (source test endpoint)

---

## Status: DONE

All acceptance criteria verified. `npm run build`, `npm run lint`, `npm run test` all green.

---

## Changes made

### Step 0 — Regenerate types from updated contract

```
cd web && npm run generate:api
✨ openapi-typescript 7.13.0
🚀 ../contracts/openapi/pulse-api.yaml → src/lib/api/schema.d.ts [56ms]
```

`schema.d.ts` now includes `name: string` and `enabled: boolean` on `AlertRule`
and `AlertRuleWrite`, and the new `AmsSourceStatus` schema.

---

### CR-1 — `AlertRule.name` field: remove group_by workaround

**Files changed:**
- `web/src/lib/api/schema.d.ts` — regenerated (includes `name` on AlertRule/AlertRuleWrite)
- `web/src/features/alerts/AlertRuleForm.tsx`
  - Replaced `label` state (was stored in `group_by`) with `name` state backed by
    `AlertRuleWrite.name` (required contract field).
  - `group_by` input moved into the Scope details section with its real semantics
    (grouping dimension, e.g. "stream_id", "app", "node_id").
  - Removed the workaround comment at the top of the file.
- `web/src/features/alerts/AlertsPage.tsx`
  - `ruleDisplayName()` simplified from `rule.group_by ?? fallback` to `rule.name`
    directly (now a required field, always present).

---

### CR-2 — `AlertRule.enabled` field: wire UI toggle

**Files changed:**
- `web/src/features/alerts/AlertRuleForm.tsx`
  - Added `enabled` checkbox (defaults to `true`), distinct from `muted`.
  - Labels clearly document the semantic difference:
    - Enabled: rule is evaluated; uncheck to pause without deleting.
    - Muted: evaluated and recorded, but no notifications sent.
  - `enabled` is passed through to `AlertRuleWrite.enabled` in `onSave`.
- `web/src/features/alerts/AlertsPage.tsx`
  - Rules list now shows `disabled` badge when `rule.enabled === false`.
  - Shows `muted` badge only when `rule.enabled && rule.muted` (rule is active but
    silenced). A disabled rule's muted state is not surfaced separately.

---

### CR-3 — Source test endpoint: switch to typed schema, handle 404/501 gracefully

**Files changed:**
- `web/src/lib/api/types.ts`
  - Added `AmsSourceStatus` export (re-exported from generated `Schemas["AmsSourceStatus"]`).
- `web/src/api/client.ts`
  - `testSource()` now returns `Promise<AmsSourceStatus>` (typed, from generated schema).
  - Removed hand-rolled `{ ok?: boolean; error?: string }` shape.
  - Wraps call in try/catch: if server returns 404 or 501 (wave-2 not yet deployed),
    returns synthetic `{ reachable: false, error: "Source test not yet implemented (wave 2)" }`
    so the UI degrades gracefully without throwing.
- `web/src/features/settings/OnboardingWizard.tsx`
  - `handleTest()` updated to use `status.reachable` (not `ok`) to set test status.
  - Shows latency + version from `AmsSourceStatus` when available.
  - On 404/501 (synthetic `reachable: false`): shows the error message in the fail
    banner rather than crashing or showing an untyped error.

---

## Acceptance checks — verified outputs

### `npm run build` (tsc strict + vite)

```
> @pulse/web@0.1.0 build
> tsc -b && vite build

vite v6.4.3 building for production...
✓ 638 modules transformed.
dist/index.html                   0.41 kB │ gzip:   0.29 kB
dist/assets/index-DjajK3KD.css    1.29 kB │ gzip:   0.66 kB
dist/assets/index-DGUx5GLK.js   697.98 kB │ gzip: 206.87 kB
✓ built in 903ms
```

Result: **GREEN**

### `npm run lint` (ESLint)

```
> @pulse/web@0.1.0 lint
> eslint src
(no output — 0 errors, 0 warnings)
```

Result: **GREEN**

### `npm run test` (21 tests)

```
> @pulse/web@0.1.0 test
> vitest run

 ✓ src/features/live/__tests__/LiveSocket.test.ts (8 tests) 5ms
 ✓ src/features/live/__tests__/StreamsTable.test.tsx (7 tests) 97ms
 ✓ src/features/alerts/__tests__/AlertRuleForm.test.tsx (6 tests) 149ms

 Test Files  3 passed (3)
      Tests  21 passed (21)
   Duration  903ms
```

Result: **21/21 GREEN**

Updated test assertions:
- `data.name === "CPU alert"` (was `data.group_by === "CPU alert"` — workaround removed)
- `data.enabled === true` (new assertion — CR-2 default)
- `initial` fixture for "edit heading" test now includes `name: "High CPU"` and `enabled: true`
  (both now required fields on `AlertRule`).

### No hand-rolled API shapes

```
$ cd web && git grep -rn "interface.*Response\|interface.*Request" -- "src/" \
    | grep -v "schema.d.ts\|types.ts"
(no output)
```

```
$ grep -n "ok.*boolean\|{ ok\?" src/api/client.ts
(no output)
```

All API-typed values flow through generated `schema.d.ts`. The old `{ ok?: boolean; error?: string }`
hand-rolled shape in `testSource` is replaced by `AmsSourceStatus` from the generated schema.

---

## Gaps / carried forward

None. All three CRs assigned are complete.

CR-3 note: the `testSource` 404/501 fallback ensures the wave-1 UI works with the
wave-1 server binary (which doesn't implement `POST /admin/sources/{id}/test` yet).
When BE-02 implements the endpoint in wave 2, the fallback path becomes unreachable
and the real `AmsSourceStatus` response will flow through unchanged.

---

## Files changed

| File | Change |
|------|--------|
| `web/src/lib/api/schema.d.ts` | Regenerated from updated contracts (CR-1, CR-2, CR-3 types) |
| `web/src/lib/api/types.ts` | Added `AmsSourceStatus` export |
| `web/src/api/client.ts` | `testSource` typed to `AmsSourceStatus`, handles 404/501 gracefully |
| `web/src/features/alerts/AlertRuleForm.tsx` | CR-1: real `name` field; CR-2: `enabled` toggle; `group_by` restored to real semantics |
| `web/src/features/alerts/AlertsPage.tsx` | CR-1: `ruleDisplayName` uses `rule.name`; CR-2: disabled/muted badges |
| `web/src/features/settings/OnboardingWizard.tsx` | CR-3: uses `AmsSourceStatus.reachable` with latency/version display |
| `web/src/features/alerts/__tests__/AlertRuleForm.test.tsx` | Updated assertions for CR-1 (`name`), CR-2 (`enabled`); fixed `initial` fixture |
