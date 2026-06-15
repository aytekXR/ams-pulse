# V3b FE-01 Report

**Agent:** FE-01 (Frontend)
**Date:** 2026-06-15
**Commit:** 9a0ba42
**VDs addressed:** VD-01, VD-36, VD-02, VD-X3-B

---

## Summary

All four assigned VDs fixed. Build, lint, and test (150 tests) all green.

---

## VD-01: Tier upsell logic + copy

**Root cause:** `isGated` in `ReportsPage.tsx` was `license?.tier === "free"`, allowing
`pro` tier users into Business-gated features (reports, tenants, schedules).

**Fix:** Changed gate to `license != null && license.tier !== "business" && license.tier !== "enterprise"`.
Copy already said "requires Business tier" — now the gate matches the copy.

**Files changed:**
- `web/src/features/reports/ReportsPage.tsx` — gate logic (line ~759)
- `web/src/features/alerts/__tests__/TierGate.test.ts` — NEW: 14 tests for per-tier matrix
- `web/src/features/reports/__tests__/ReportsPage.test.tsx` — updated: `pro` tier now shows upsell; added `business` tier "entitled" test
- `web/src/features/reports/__tests__/TenantsTab.test.tsx` — updated: 4 tests changed from `pro` → `business`; added guard test confirming `pro` is gated

**Guard test:** `TierGate.test.ts` — "pro tier is GATED from reports (not 'free' check — it needs business+)" would have failed on the old `tier==='free'` guard.

**Tier matrix verified:**
| Tier | Reports | Anomalies | PagerDuty/Webhook channel |
|------|---------|-----------|--------------------------|
| free | gated | gated | gated |
| pro | gated | gated | gated |
| business | entitled | gated | entitled |
| enterprise | entitled | entitled | entitled |

---

## VD-36: Cron preset validation

**Root cause:** Presets used 5-field cron (`0 6 1 * *`) but server only accepted 2-3 fields.
BE-02-B fixed `server/internal/reports/cron.go` to accept 5-field cron in this wave.

**Finding:** The presets in `ReportsPage.tsx` were already correct 5-field cron expressions.
The presets are: `"0 6 1 * *"` (monthly), `"0 6 * * 1"` (weekly Monday), `"0 6 * * *"` (daily).

**Fix:** Added tests in `ReportsPage.test.tsx` verifying each preset has exactly 5 fields
and passes the schedule form's field-count validation. Added guard test confirming 3-field
cron (old format) fails validation.

**Files changed:**
- `web/src/features/reports/__tests__/ReportsPage.test.tsx` — VD-36 cron preset validation suite (3 new tests)

---

## VD-02: WS LiveOverview field mapping

**Root cause:** Server was broadcasting `LiveSnapshot` (missing `total_publishers`, `protocol_mix`, `apps`)
instead of `LiveOverview` shape. FE code already read payload as `LiveOverview` correctly —
the FE fix is to add guard tests that would catch a regression if BE reverts to old shape.

**Finding:** `useLiveDashboard.ts` and `LiveDashboard.tsx` already correctly use:
- `overview?.total_publishers` (line 19)
- `overview?.protocol_mix` (line 109)  
- `overview?.apps` (line 123, 135)

**Fix:** Added 2 guard tests in `LiveSocket.test.ts`:
1. Snapshot message carries `total_publishers`, `protocol_mix`, and `apps`
2. Delta message carries `total_publishers` and `protocol_mix`

These tests would fail if a WS message arrived without those fields.

**Files changed:**
- `web/src/features/live/__tests__/LiveSocket.test.ts` — 2 new tests (VD-02 guard)

---

## VD-X3-B: client.ts interval param rename

**Root cause:** `analyticsApi.getAudience()` sent `granularity=` query param; spec and server
use `interval`.

**Fix:** Renamed param from `granularity` to `interval` in `client.ts` line ~145. No callers
pass this param currently (AnalyticsPage calls with `{ from, to }` only), so no cascade changes.

**Files changed:**
- `web/src/api/client.ts` — param rename (lines 134-149)

---

## API types regeneration

Ran `npm run generate:api` as required. `schema.d.ts` now reflects the updated OpenAPI spec:
- `LicenseInfo.tier` is now `"free" | "pro" | "business" | "enterprise"` (was missing `"business"`)

All tier literal strings in FE code (`"business"`, `"enterprise"`, etc.) match the generated
schema union exactly — no hand-rolled API shapes introduced.

---

## Acceptance gate

| Check | Result |
|-------|--------|
| `npm run build` | PASS (clean, no type errors) |
| `npm run lint` | PASS (0 warnings) |
| `npm run test` | PASS (150 tests, 11 suites) |
| VD-01 guard test (pro gated) | PASS |
| VD-36 preset validation tests | PASS |
| VD-02 WS field mapping tests | PASS |
| VD-X3-B param rename | PASS (build validates no TS errors) |

---

## Scope adherence

Changes are strictly within `web/` (FE-01 charter scope). No contracts edited. No server files touched. Schema regenerated from existing spec (read-only operation). Other modified files in the working tree (`server/internal/...`) are from other agents and were NOT staged.
