# Findings: X3 Contract Conformance

**Area:** X3-contract-conformance  
**Verifier:** QA adversarial subagent  
**Date:** 2026-06-14  
**Verdict:** PARTIAL

---

## Confirmed Findings (3 Schema Violations, 2 Frontend Drifts)

### F-X3-001 (major): GET /qoe/summary — BitrateBucket uses wrong field name

Spec `BitrateBucket` requires `bitrate_kbps_p50` (required). Handler emits `"bitrate_kbps"`.
Measured by kin-openapi: "property 'bitrate_kbps_p50' is missing".
File: server/internal/api/server.go line 715.

### F-X3-002 (major): GET /qoe/ingest — IngestStream missing required `timeseries`

Spec `IngestStream` requires `timeseries` (array). Handler never includes it.
Measured by kin-openapi: "property 'timeseries' is missing".
File: server/internal/api/server.go lines 736-757.

### F-X3-003 (major): POST /admin/sources/{sourceId}/test — wrong response shape

Spec `AmsSourceStatus` requires `reachable: boolean`. Handler returns `{"status":"...","message":"...","latency_ms":...}`.
Measured: "missing property 'reachable'".
File: server/internal/api/server.go lines 1060-1108.

### F-X3-004 (minor): Frontend calls /api/v1/reports/export — not in spec or server

web/src/api/client.ts lines 170 and 408 navigate to `/api/v1/reports/export`.
This path does not exist in the spec or server router. Results in a 404.
The server-side CSV path (GET /analytics/audience?format=csv) works but frontend calls wrong URL.

### F-X3-005 (minor): Frontend sends `granularity` param; spec/server use `interval`

web/src/api/client.ts line 145 sends `granularity=` but spec defines parameter as `interval`.
Server ignores `granularity`; `interval` always defaults to "day".
Currently not reached by any UI page but function signature is a drift risk.

### F-X3-006 (minor): DELETE tokens/users returns 204 even for non-existent resources

Spec declares 404 for DELETE /admin/tokens/{tokenId} and DELETE /admin/users/{userId}.
Both handlers always return 204 (idempotent-delete pattern contradicts spec contract).
