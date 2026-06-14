# V3a INT-01 Completion Report

**Agent:** INT-01 (Integration & Contracts)
**Date:** 2026-06-14
**Branch:** main
**Scope:** contracts/openapi/pulse-api.yaml, server/internal/license/license.go, server/internal/license/license_test.go, server/internal/api/v3a_contract_test.go

---

## Summary

Applied all 5 assigned VDs to the OpenAPI spec and Go license code. Redocly lint passes (0 errors). All server tests pass (17 packages, `go test ./...`). kin-openapi still loads the spec (validated in TestContract_AmsSourceStatus_SpecHasReachableRequired and TestContract_LicenseInfo_TierEnum_IncludesBusiness).

---

## VD-by-VD changes

### VD-01 — Add `business` to License.tier enum (FIXED)

**Spec change** (`contracts/openapi/pulse-api.yaml`):
- `LicenseInfo.tier` enum changed from `[free, pro, enterprise]` to `[free, pro, business, enterprise]`
- Added entitlement-matrix description block to `LicenseInfo` schema documenting the PRD §7.11 four-tier table (nodes, retention, channels, DataAPI, white-label, multi-tenant, anomalies per tier)
- Added per-tier description to the `tier` property field

**Go change** (`server/internal/license/license.go`):
- Added `TierBusiness Tier = "business"` constant between `TierPro` and `TierEnterprise`
- Added `businessTierEntitlements` var: MaxNodes=5, RetentionDays=396 (13 months), Channels=[email,slack,telegram,pagerduty,webhook], DataAPI=true, WhiteLabel=false
- Updated `buildEntitlements()` switch: added `case TierBusiness:` routing to business channel list
- Updated `CheckMultiTenant()`: changed guard from `t != TierEnterprise` to `t != TierBusiness && t != TierEnterprise` (PRD §7.11: multi-tenant is Business+, not Enterprise-only)
- Updated package doc comment to include `"business"` in the tier string

**Tests added** (`server/internal/license/license_test.go`):
- `TestTierConstants_FourTiersExist` — asserts all 4 PRD §7.11 tier constants exist with correct string values; would have caught the missing TierBusiness constant
- `TestBusinessTier_PagerDutyAllowed` — compile-time guard that TierBusiness="business" constant exists
- `TestMultiTenant_BusinessAllowed` — verifies CheckMultiTenant error mentions Business (not Enterprise-only)
- `TestAnomalies_RequiresEnterprise` — regression guard: anomaly detection still requires Enterprise (unchanged)

---

### VD-X3-A — AmsSourceStatus must include `reachable: boolean` (PARTIAL — spec already correct)

**Finding:** The spec already had `AmsSourceStatus.required: [reachable]` with `reachable: boolean`. The contract was already correct. The defect is in the handler (BE-02 scope: handler returns `{status, message, latency_ms}` without `reachable`).

**INT-01 action:** No spec change needed. Added guarding tests.

**Tests added** (`server/internal/api/v3a_contract_test.go`):
- `TestContract_AmsSourceStatus_SpecHasReachableRequired` — loads spec via kin-openapi and asserts `AmsSourceStatus.required` contains `reachable` as a boolean property; guards against spec regression
- `TestContract_AmsSourceStatus_HandlerReachableField` — end-to-end handler check, **skipped** with explicit note that this is BE-02 scope; will un-skip when BE-02 wires the `reachable` field

**Note for ORCH-00:** VD-X3-A handler fix (adding `reachable` to `handleTestSource` response) remains open for BE-02. The spec is already authoritative.

---

### VD-X3-D — Add 403 response to GET /anomalies (FIXED)

**Spec change** (`contracts/openapi/pulse-api.yaml`, `/anomalies` GET path responses):
- Added `"403"` response entry: "Enterprise tier required — anomaly detection (F9) is gated to Enterprise subscribers" with `$ref: Error` schema

**Tests added** (`server/internal/api/v3a_contract_test.go`):
- `TestContract_Anomalies_FreeTier_Returns403` — verifies free tier receives 403 with Error envelope; also validates the documented response matches the actual handler behaviour (handler already returns 403 with `code=LICENSE_REQUIRED`)

---

### VD-X3-C — Document DELETE endpoints as idempotent-204 (FIXED)

**Spec change** (`contracts/openapi/pulse-api.yaml`):
- `DELETE /admin/tokens/{tokenId}` (`revokeToken`): removed `"404"` response; updated summary to "Revoke a token (idempotent)"; added description documenting idempotent-delete semantics (204 always, even for non-existent tokens)
- `DELETE /admin/users/{userId}` (`deleteUser`): removed `"404"` response; updated summary to "Delete a local user (idempotent)"; added description documenting idempotent-delete semantics

**Chosen semantics:** idempotent-204 (handlers already return 204 unconditionally). Clients must not rely on 404 to detect missing resources.

**Tests added** (`server/internal/api/v3a_contract_test.go`):
- `TestContract_DeleteToken_Idempotent` — deletes non-existent tokenId; asserts 204 (not 404)
- `TestContract_DeleteUser_Idempotent` — deletes non-existent userId; asserts 204 (not 404)

---

### VD-S4 — Set beacon ingest body-size cap to 64 KB in spec (FIXED)

**Spec change** (`contracts/openapi/pulse-api.yaml`, `POST /ingest/beacon`):
- Description: changed "body size limit 256 KB" to "body size limit 64 KB (authoritative: hardened handler enforces `maxBodyBytes = 64 * 1024`)"
- 413 response description: changed "exceeds 256 KB limit" to "exceeds 64 KB limit (hardened handler enforces maxBodyBytes=65536)"

**Tests added** (`server/internal/api/v3a_contract_test.go`):
- `TestContract_BeaconIngest_64KB_BodySizeCap` — sends a 65 KB body and asserts the response is NOT 202 (acceptance); verifies the server does not accept over-limit bodies

---

## Acceptance verification

| Criterion | Result |
|-----------|--------|
| `npx @redocly/cli lint contracts/openapi/pulse-api.yaml` | 0 errors — "Your API description is valid" |
| `CGO_ENABLED=0 go build ./...` (server) | PASS — no compile errors |
| `CGO_ENABLED=0 go test ./...` (server, 17 packages) | PASS — all green |
| `go test ./internal/api/...` with new TestContract_* tests | PASS (1 SKIP: VD-X3-A handler, documented) |
| `go test ./internal/license/...` with new tier tests | PASS — all 8 test functions |

---

## Files changed

| File | Change |
|------|--------|
| `contracts/openapi/pulse-api.yaml` | VD-01 tier enum + description; VD-X3-D 403 on /anomalies; VD-X3-C idempotent DELETE; VD-S4 64 KB cap |
| `server/internal/license/license.go` | VD-01: TierBusiness constant, businessTierEntitlements, CheckMultiTenant fix, buildEntitlements switch |
| `server/internal/license/license_test.go` | VD-01 guarding tests (4 new functions) |
| `server/internal/api/v3a_contract_test.go` | NEW: VD-X3-A/VD-X3-D/VD-X3-C/VD-S4/VD-01 contract guards (7 new functions) |

---

## Open items for other agents

| VD | Agent | Status |
|----|-------|--------|
| VD-X3-A handler | BE-02 | Handler must return `reachable: bool` in `handleTestSource` response (see TestContract_AmsSourceStatus_HandlerReachableField which is `t.Skip`-ped) |
| VD-01 gate sites | BE-02 | Update all tier gate checks to use TierBusiness where appropriate (reports, channels, etc.) |
| VD-01 frontend | FE-01 | Update upsell copy and `isGated` logic for Business tier |
| VD-01 tests | QA-01 | Add per-tier matrix integration tests |
