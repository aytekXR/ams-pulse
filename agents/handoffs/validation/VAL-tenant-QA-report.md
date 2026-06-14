# VAL-tenant-QA-report — QA-01 verification of D-010 /admin/tenants CRUD

**Agent:** QA-01  
**Date:** 2026-06-14  
**Status:** PASS_WITH_DEFECT  

---

## Executive summary

Tenant CRUD (D-010) is substantially PASS. The server implementation, OpenAPI contract, and live API integration all pass. One defect (DEF-QA-001) blocks the `npm run build` step in all three gate scripts due to TypeScript type errors in FE-01's `TenantsTab.test.tsx`.

---

## A. Binary rebuild (D-013 lesson)

| Binary | Command | Result |
|--------|---------|--------|
| `/tmp/pulse` | `cd server && CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/` | PASS — 0 errors |
| go vet | `CGO_ENABLED=0 go vet ./...` | PASS — 0 warnings |

---

## B. Server unit test suite (freshly rebuilt binary)

```
cd server && CGO_ENABLED=0 go test -count=1 ./... -timeout 180s
```

| Package | Status |
|---------|--------|
| internal/api | PASS |
| internal/license | PASS |
| internal/alert | PASS |
| internal/alert/channels | PASS |
| internal/anomaly | PASS |
| internal/cluster | PASS |
| internal/collector | PASS |
| internal/collector/beacon | PASS |
| internal/collector/ingest | PASS |
| internal/collector/kafka | PASS |
| internal/collector/logtail | PASS |
| internal/collector/restpoller | PASS |
| internal/collector/sessions | PASS |
| internal/domain | PASS |
| internal/license | PASS |
| internal/prober | PASS |
| internal/reports | PASS |
| internal/store/meta | PASS |
| **All packages** | **PASS** |

---

## C. Tenant-specific unit tests (17/17 PASS, fresh run)

```
cd server && CGO_ENABLED=0 go test -count=1 ./internal/api/... -v -run TestTenant
```

| Test | Status | Measured |
|------|--------|----------|
| TestTenant_CRUD_HappyPath | PASS | POST→201, GET→200, LIST→200 (1 item), PUT→200, DELETE→204, GET-after-DELETE→404 |
| TestTenant_MetaTagMatcher_HappyPath | PASS | POST with meta_tag_key+value → 201 |
| TestTenant_DuplicateName_409 | PASS | duplicate name → 409 DUPLICATE_NAME |
| TestTenant_UpdateDuplicateName_409 | PASS | PUT with conflicting name → 409 |
| TestTenant_GetUnknownID_404 | PASS | GET /admin/tenants/nonexistent → 404 |
| TestTenant_UpdateUnknownID_404 | PASS | PUT /admin/tenants/no-such-id → 404 |
| TestTenant_DeleteUnknownID_404 | PASS | DELETE /admin/tenants/no-such-id → 404 |
| TestTenant_NoMatcher_422 | PASS | no matcher → 422 INVALID_TENANT |
| TestTenant_PartialMetaTag_422 | PASS | meta_tag_key without value → 422 |
| TestTenant_FreeTier_Blocked_403 (5 ops) | PASS | all 5 ops → 403 on Free tier |
| TestTenant_NonEnterpriseTier_Blocked_403 | PASS | non-enterprise → 403 |
| TestTenant_StreamPattern_ResolvesStream | PASS | Resolve("live/stream1")="LiveCorp", Resolve("other/stream")="" |
| TestTenant_MetaTag_ResolvesStream | PASS | meta-tag match → "OrgTenant", non-match → "" (unassigned) |
| TestTenant_OpenAPI_ListConforms | PASS | listTenants response conforms to spec |
| TestTenant_OpenAPI_CreateConforms | PASS | createTenant response conforms to spec |
| TestTenant_OpenAPI_GetConforms | PASS | getTenant response conforms to spec |
| TestTenant_OpenAPI_UpdateConforms | PASS | updateTenant response conforms to spec |
| TestTenant_OpenAPI_DeleteConforms | PASS | deleteTenant response conforms to spec |

Total elapsed: 0.407s

---

## D. Live stack integration test

**Stack config:**
- ClickHouse: 127.0.0.1:9250 (wave-2 gate instance, v26+), database `pulse`
- Pulse: `/tmp/pulse serve` on :8097 with Enterprise tier license (freshly signed dev key)
- License tier confirmed: `{"tier":"enterprise","valid":true}`

### D.1 CRUD via live API

| Step | Request | Expected | Actual | Verdict |
|------|---------|----------|--------|---------|
| Create Tenant 1 | POST /admin/tenants `{"name":"Acme Corp","stream_pattern":"live/acme-%"}` | 201 | 201, id=c22d500d-... | PASS |
| Create Tenant 2 | POST /admin/tenants `{"name":"Beta Inc","stream_pattern":"live/beta-%"}` | 201 | 201, id=a2de15ae-... | PASS |
| List tenants | GET /admin/tenants | 200, 2 items | 200, 2 items | PASS |
| Get by ID | GET /admin/tenants/{T1_ID} | 200 | 200 | PASS |
| Update | PUT /admin/tenants/{T1_ID} `{"name":"Acme Corp Updated","stream_pattern":"live/acme-%"}` | 200 | 200, name updated | PASS |
| 409 duplicate name | POST `{"name":"Beta Inc","stream_pattern":"other/%"}` | 409 DUPLICATE_NAME | 409 DUPLICATE_NAME | PASS |
| 404 unknown ID | GET /admin/tenants/non-existent-uuid-12345 | 404 | 404 | PASS |
| 422 no matcher | POST `{"name":"No Matcher Corp"}` | 422 INVALID_TENANT | 422 INVALID_TENANT | PASS |
| Delete Tenant 2 | DELETE /admin/tenants/{T2_ID} | 204 | 204 | PASS |
| Verify deleted | GET /admin/tenants/{T2_ID} | 404 | 404 | PASS |

### D.2 Free/Pro tier 403 gate

Verified via unit tests (TestTenant_FreeTier_Blocked_403 PASS x5 ops, TestTenant_NonEnterpriseTier_Blocked_403 PASS). Live instance is Enterprise; separate instance needed for direct live 403 test — covered by unit tests (same handler code path).

### D.3 Reconciliation scenario (±1% accuracy criterion)

**Seeded data in ClickHouse:**

| stream_id | sessions | watch_time_s | tenant (label) |
|-----------|----------|--------------|----------------|
| live/acme-stream1 | 3 | 180 | Acme Corp Updated |
| live/acme-stream2 | 2 | 120 | Acme Corp Updated |
| live/beta-stream1 | 3 | 360 | Beta Inc |
| other/unmatched-s1 | 2 | 400 | (empty = unassigned) |

**Known truth:**
- Acme Corp Updated: 300s = 5.0 viewer-minutes
- Beta Inc: 360s = 6.0 viewer-minutes
- Unmatched (other/unmatched-s1): 400s = 6.6667 viewer-minutes

**API responses (GET /reports/usage with tenant= filter):**

| Tenant filter | API viewer_minutes | Known truth | Drift | Verdict |
|---------------|--------------------|-------------|-------|---------|
| `tenant=Acme Corp Updated` | 5.0 | 5.0 | 0.0000% | PASS ≤ ±1% |
| `tenant=Beta Inc` | 6.0 | 6.0 | 0.0000% | PASS ≤ ±1% |
| (no filter) total | 17.6667 | 17.6667 | 0.0000% | PASS |
| Unmatched stream | API row: tenant='', viewer_minutes=6.6667 | 6.6667 | 0.0000% | PASS (falls to 'unassigned') |

**Reconciliation unit test (TestSeedMonth_ReconcileWithinOnePct):**
- n=10000 sessions, truth=148900 viewer-minutes, computed=148900, drift=0.0000%, elapsed=3.97ms

All figures reconcile within ±1% of known truth. Unmatched stream correctly assigned to unassigned (tenant='').

---

## E. Web build / lint / test regression

| Check | Command | Result |
|-------|---------|--------|
| `npm run lint` | `cd web && npm run lint` | PASS — 0 errors, 0 warnings |
| `npm run test` | `cd web && npm run test` | PASS — 127/127 tests pass (18 new TenantsTab tests) |
| `npm run build` | `cd web && npm run build` (= `tsc -b && vite build`) | **FAIL** — tsc reports 10 errors in `TenantsTab.test.tsx` (see DEF-QA-001) |
| `npx vite build` (production only) | production bundle | PASS — 643 modules, 782 KB |

---

## F. Gate script regression

| Script | Result | Root cause |
|--------|--------|------------|
| `qa/wave-1/run-gate.sh` | **FAIL** — 1 criterion failed | `npm run build` FAILED (DEF-QA-001) |
| `qa/wave-2/run-gate.sh` | **FAIL** — C2 npm run build failed | Same (DEF-QA-001) |
| `qa/wave-3/run-gate.sh` | **FAIL** — G4d, G4h, B-07 | Same (DEF-QA-001) |

All non-build criteria PASS. The sole regression is the TS build failure introduced by FE-01's test file.

---

## G. Defect register

### DEF-QA-001 (BLOCKER) — FE-01 TenantsTab.test.tsx TypeScript type errors

**Owner:** FE-01  
**Severity:** Blocker — breaks `npm run build` (tsc -b fails), blocking all three gate scripts (wave-1 B-07, wave-2 C2, wave-3 G4d/G4h)  
**Scope:** Test file only; production bundle (vite build) succeeds; runtime behavior unaffected  

**Repro:**
```
cd web && npm run build
# → 10 TypeScript errors in src/features/reports/__tests__/TenantsTab.test.tsx
```

**Errors:**
1. `meta: { total, limit, cursor }` — `limit` does not exist in `PaginatedMeta` type. Correct shape is `{ next_cursor?: string | null, total?: number | null }`.  
   Lines: 184, 205, 222, 236, 250, 264, 280, 296, 339  
2. `tier: "business"` — not assignable to `"free" | "pro" | "enterprise"`. Should be `"enterprise"`.  
   Line: 202  

**Contract:** `PaginatedMeta` in `contracts/openapi/pulse-api.yaml` → `{ next_cursor, total }`. No `limit` or `cursor` field exists.

**Fix (FE-01 only — QA-01 does not fix product code):** Replace `meta: { total: N, limit: 50, cursor: null }` with `meta: { next_cursor: null, total: N }` in all affected test mock data. Replace `tier: "business"` with `tier: "enterprise"`.

---

## H. OpenAPI contract verification

```
npx --yes @redocly/cli lint --skip-rule=path-parameters-defined contracts/openapi/pulse-api.yaml
```
Result: 0 errors, 0 warnings. All 5 tenant operations lint clean. (Verified by INT-01 report; server tests load the spec via kin-openapi with 0 conformance errors.)

---

## I. Waivers applied

| Waiver | Reason |
|--------|--------|
| D-002 (no Docker) | No Docker on this machine; gate scripts use local ClickHouse binary |
| D-007.5 (no Kafka) | Not relevant to tenant CRUD |

---

## J. Acceptance criteria scorecard

| Criterion | Result | Measured |
|-----------|--------|----------|
| Binary rebuild (D-013) | PASS | Clean build, 0 errors |
| Server go test ./... | PASS | All packages pass |
| 17/17 tenant unit tests | PASS | 0.407s elapsed |
| CRUD happy path (live API) | PASS | All 5 ops correct status codes |
| 409 duplicate name (live API) | PASS | 409 DUPLICATE_NAME |
| 404 unknown id (live API) | PASS | 404 |
| 422 no matcher (live API) | PASS | 422 INVALID_TENANT |
| Free/Pro tier 403 | PASS | Unit test verified (5 ops) |
| Reconciliation ±1% (Acme Corp) | PASS | drift=0.0000% |
| Reconciliation ±1% (Beta Inc) | PASS | drift=0.0000% |
| Unmatched stream falls to 'unassigned' | PASS | tenant='' in CH row |
| web lint | PASS | 0 errors |
| web tests | PASS | 127/127 |
| web build (tsc -b) | **FAIL** | DEF-QA-001 — 10 TS errors in TenantsTab.test.tsx |
| wave-1 gate regression | **FAIL** | 1 criterion (npm run build) — DEF-QA-001 |
| wave-2 gate regression | **FAIL** | C2 (npm run build) — DEF-QA-001 |
| wave-3 gate regression | **FAIL** | G4d/G4h/B-07 — DEF-QA-001 |

**Overall verdict:** PASS_WITH_DEFECT. Server, API, and reconciliation pass completely. Web build is blocked by DEF-QA-001 (FE-01 test file TS type mismatch). Route to FULL_PASS once FE-01 fixes `TenantsTab.test.tsx`.
