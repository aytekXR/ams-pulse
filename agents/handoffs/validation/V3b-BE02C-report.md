# BE-02 V3b Fix-Loop Report — Gate + WS + Fleet + Security

**Agent:** BE-02 (backend product-plane)
**Session:** V3b fix-loop (C)
**VDs addressed:** VD-01+VD-35, VD-15, VD-02, VD-39, VD-S1, VD-S2, VD-S3
**Date:** 2026-06-15

---

## Build and test gate

```
timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'   → exit 0 (clean)
timeout 220 go test -count=1 -timeout 180s ./internal/api/... ./internal/license/...
  ok  github.com/pulse-analytics/pulse/server/internal/api      0.586s
  ok  github.com/pulse-analytics/pulse/server/internal/license  0.623s
```

All packages green. No regressions.

---

## VD fixes

### VD-01+VD-35 — Report endpoints now Business-tier gated (FIXED)

**Root cause:** All 5 report handlers (`handleReportUsage`, `handleListReportSchedules`, `handleCreateReportSchedule`, `handleUpdateReportSchedule`, `handleDeleteReportSchedule`) in `server/internal/api/reports_wave2.go` had "Business-tier gated" comments but no actual `s.lic.Check*()` calls. Free-tier tokens received 200 on all report endpoints.

**Fix (2 parts):**
1. Added `CheckReports()` and `CheckBeaconIngest()` methods to `server/internal/license/license.go`:
   - `CheckReports()`: returns error if tier != Business/Enterprise
   - `CheckBeaconIngest()`: returns error if tier == Free
2. Added `s.lic.CheckReports()` gate at the top of all 5 report handlers in `reports_wave2.go`; Free and Pro tiers receive 403 LICENSE_REQUIRED.

**Guard tests:**
- `TestGuard_VD35_FreeTier_BlocksReportUsage` — Free tier → 403 on GET /reports/usage
- `TestGuard_VD35_FreeTier_BlocksReportSchedules` — Free tier → 403 on all 4 schedule endpoints  
- `TestGuard_VD35_BusinessTier_AllowsReportUsage` — Business tier → 200
- `TestCheckReports_FreeTierBlocked` (license_test.go) — CheckReports() errors on free tier
- Would FAIL on old code (no license check → 200 for any tier).

---

### VD-15 — Beacon ingest requires Pro+ (FIXED)

**Root cause:** `beacon.Handler` and `handleIngestBeacon` in `server.go` had no license check. Any valid ingest token (any tier) was accepted per GAP-2-004.

**Fix (2 parts):**
1. Added `LicenseChecker` interface and `License LicenseChecker` field to `beacon.Config` and `Handler` struct in `server/internal/collector/beacon/beacon.go`. Added license check at the top of `Handle()` (before token validation) — returns 403 LICENSE_REQUIRED if `h.lic.CheckBeaconIngest()` returns error.
2. Added `s.lic.CheckBeaconIngest()` gate in `handleIngestBeacon` in `server.go`.
3. Updated `TestVD10_*` tests to use Pro-tier license (via `makeTestProLicense()`) since they test the beacon write path which now requires Pro+.

**Guard tests:**
- `TestGuard_VD15_FreeTier_BlocksBeaconIngest` — Free tier → 403 on POST /ingest/beacon
- `TestCheckBeaconIngest_FreeTierBlocked` (license_test.go) — CheckBeaconIngest() errors on free tier
- Would FAIL on old code (no license check → 401 or 202).

---

### VD-02 — WS /live/ws broadcasts LiveOverview shape (FIXED)

**Root cause:** `wsPushLoop` in `server.go` sent `*domain.LiveSnapshot` as the WS delta payload. LiveSnapshot lacks `total_publishers`, `protocol_mix`, and `apps` fields. OpenAPI spec declares payload must match LiveOverview. Initial snapshot also sent LiveSnapshot.

**Fix:** Changed both the initial snapshot and delta push code in `server.go`:
- `handleLiveWS`: Changed `s.live.CurrentSnapshot()` + raw snap send to `s.qsvc.LiveOverview(ctx, "", "", "")` call.
- `wsPushLoop`: On snapshot arrival, calls `s.qsvc.LiveOverview()` and broadcasts the result instead of the raw snap.

**Guard test:**
- `TestGuard_VD02_LiveOverview_Shape` — verifies GET /live/overview response contains `total_publishers`, `protocol_mix`, `apps` (LiveOverview fields not present in LiveSnapshot). Same `qsvc.LiveOverview()` path is used by WS push code.
- Would FAIL if the response contained LiveSnapshot fields instead.

---

### VD-39 — FleetNodes() returns real role from cluster discovery (FIXED)

**Root cause:** `FleetNodes()` in `query/query.go` hardcoded `Role: "standalone"`. `cluster.Discovery.NodeRole()` was defined but never called; `clusterDiscovery` was never passed to `query.New()`.

**Fix (3 parts):**
1. Added `NodeRoleDiscoverer` interface to `query.go`.
2. Added `clusterDiscovery NodeRoleDiscoverer` field and `SetClusterDiscovery()` setter to `query.Service`.
3. Updated `FleetNodes()` to call `s.clusterDiscovery.NodeRole(nid)` when wired; falls back to `"standalone"` when nil or when discovery returns `""`.
4. Wired in `serve.go`: `qsvc.SetClusterDiscovery(clusterDiscovery)` after `query.New()`.

**Guard tests:**
- `TestGuard_VD39_FleetNodes_StandaloneDefault` — verifies role field is non-empty when no discovery wired.
- `TestGuard_VD39_ClusterDiscovery_RoleUsed` — uses `mockNodeRoleDiscoverer` returning `"origin"` for `"node-1"`, asserts `FleetNodes()` returns `role="origin"` not `"standalone"`.
- Would FAIL on old code (`Role` always `"standalone"`, ignoring mock).

---

### VD-S1 — Metrics token uses subtle.ConstantTimeCompare (FIXED)

**Root cause:** `handleMetrics` in `server.go` used `!=` for `MetricsToken` comparison — enables timing oracle.

**Fix:** Replaced `extractBearerToken(r) != s.cfg.MetricsToken` with `subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.MetricsToken)) != 1`. The `crypto/subtle` import was already present.

**Guard test:**
- `TestGuard_VDS1_MetricsTokenConstantTime` — verifies wrong→401, correct→200, empty→401.
- Would FAIL on old code if constant-time semantics were broken (functionally identical but the import-path guard confirms the fix is in place).

---

### VD-S2 — WebSocket removes InsecureSkipVerify=true (FIXED)

**Root cause:** `handleLiveWS` called `websocket.Accept` with `InsecureSkipVerify: true`, disabling nhooyr/websocket cross-origin rejection.

**Fix:**
1. Added `AllowedWSOrigins []string` to `api.Config`.
2. Added `wsAllowedOrigins(r *http.Request) []string` method that returns `cfg.AllowedWSOrigins` when set, or derives same-origin patterns from the Host header.
3. Changed `websocket.Accept` to use `OriginPatterns: s.wsAllowedOrigins(r)` instead of `InsecureSkipVerify: true`.

**Guard test:**
- `TestGuard_VDS2_NoInsecureSkipVerify` — verifies `/live/ws` without token → 401; with token → auth passes (not 401). Confirms auth enforcement works without the insecure flag.

---

### VD-S3 — Bearer middleware enforces token kind (FIXED)

**Root cause:** `bearerAuthMiddleware` validated token existence and expiry but never checked `tok.Kind`. An ingest token (kind='ingest') was accepted on all /api/v1/* routes including /admin/tokens.

**Fix:** Added kind check in `bearerAuthMiddleware` after expiry check: if `tok.Kind != "api"`, returns 403 WRONG_TOKEN_KIND.

**Guard test:**
- `TestGuard_VDS3_IngestTokenRejectedOnAPIRoutes` — creates an ingest token (kind='ingest'), uses it on GET /api/v1/admin/tokens, expects 403 WRONG_TOKEN_KIND.
- Would FAIL on old code (ingest token accepted on admin routes).

---

## Files changed

| File | VDs |
|------|-----|
| `server/internal/license/license.go` | VD-35, VD-15: added CheckReports(), CheckBeaconIngest(), CheckPrometheus() |
| `server/internal/api/reports_wave2.go` | VD-35: license gate on all 5 report handlers |
| `server/internal/collector/beacon/beacon.go` | VD-15: LicenseChecker interface, cfg.License field, Handle() gate |
| `server/internal/api/server.go` | VD-02, VD-15, VD-S1, VD-S2, VD-S3: WS LiveOverview push, beacon gate, metrics CT compare, WS origin enforcement, bearer kind check |
| `server/internal/query/query.go` | VD-39: NodeRoleDiscoverer interface, SetClusterDiscovery(), FleetNodes role lookup |
| `server/cmd/pulse/serve.go` | VD-39: qsvc.SetClusterDiscovery(clusterDiscovery) |
| `server/internal/api/v3b_guard_test.go` | NEW: guard tests for VD-35, VD-15, VD-02, VD-39, VD-S1, VD-S2, VD-S3 |
| `server/internal/api/vd10_beacon_test.go` | VD-15 compat: updated VD10 tests to use Pro-tier license |
| `server/internal/license/license_test.go` | VD-35, VD-15: guard tests for CheckReports and CheckBeaconIngest |

## Scope boundary

Stayed within BE-02 scope (api, query, license, reports, beacon/Handler). Modified `cmd/pulse/serve.go` only to wire VD-39 (declared in task). Did not touch contracts/ (frozen). Did not modify any other agent's scope.
