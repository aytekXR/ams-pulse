# Validation Findings — F8: Data API + Prometheus Endpoint

Validator: QA adversarial agent (verify-only)
Date: 2026-06-14
Branch: main (f4f8fe3)
Scope: F8 public API + Prometheus per PRD §7 + ARCHITECTURE §3-4 + D-014

## Summary

F8 partially meets its stated acceptance criteria. The Prometheus /metrics endpoint exists,
produces valid text exposition, and enforces cardinality bounds. Bearer token auth is implemented
for all /api/v1 endpoints. The web UI exclusively uses the public API (ARCH §3 satisfied).

Critical gap: The PRD's "API + Prometheus = Business tier" requirement is entirely unenforced.
CheckDataAPI() exists in the license package but is never called in any API handler.
Free-tier callers can invoke every data API endpoint, create API tokens of any kind, and
see the Prometheus metrics URL with no license check. The "Business" tier is missing
from the tier enum (D-014), so no correct gate exists to add even if the call sites were fixed.

## Findings

### F8-001 CRITICAL: Data API tier gate never enforced (CheckDataAPI dead code)

CheckDataAPI() defined in internal/license/license.go:275 is never called in
server/internal/api/*.go. Every authenticated free-tier user can call all data API
endpoints including analytics, QoE, reports, fleet, and token creation.
TestAPI_FreeTier_QoE_Accessible asserts 200 on free tier, encoding the broken behavior as correct.

### F8-002 CRITICAL: "Business" tier missing from license enum (D-014 confirmed)

PRD §7.11 places "API + Prometheus" in Business ($299) tier. Tier enum has only
free|pro|enterprise. proTierEntitlements.DataAPI=true (Pro should NOT have Data API).
proTierEntitlements.MaxNodes=10 (PRD says Pro=1-2 nodes, Business=up to 5).
Web UI ReportsPage.tsx says "requires Business tier" but gate checks tier==="free",
allowing Pro users through.

### F8-003 MAJOR: /api/v1/reports/export route missing; web client calls dead endpoint

web/src/api/client.ts analyticsApi.exportCsv() and reportsApi.downloadExport() both
navigate to /api/v1/reports/export which is not registered in chi router and not
in the OpenAPI spec. The CSV download button in AnalyticsPage and ReportsPage silently fails
with 404/405. Actual CSV export works via format=csv on /api/v1/analytics/audience
but the client calls the wrong URL.

### F8-004 MAJOR: handleReportUsage and handleCreateReportSchedule have no tier check

reports_wave2.go handlers for reports/usage and reports/schedules are documented as
"Business-tier gated" in comments but contain no s.lic.Check*() call. Only tenant
CRUD handlers call CheckMultiTenant(). Report data is freely accessible on any tier.

### F8-005 MINOR: Bearer auth middleware does not enforce token kind

bearerAuthMiddleware does not check tok.Kind. An "ingest" token (kind="ingest") is
accepted for all /api/v1 routes including /admin/tokens and /admin/users. Ingest
tokens can call data API endpoints.

### F8-006 MINOR: Prometheus labels use %q format verb (over-escapes backslash in node IDs)

server.go:500,504 use fmt.Fprintf(..., "pulse_node_cpu_pct{node=%q} %g", nid, cpu).
%q adds Go string escaping; node IDs with backslashes produce double-escaped output
that Prometheus rejects. The cardinality test does not catch this because the test
fixture uses "node-1" which has no backslashes.

### F8-007 COSMETIC: /metrics executes a blocking SQLite full-scan on every scrape

ListAlertHistory(ctx, "", "firing", 0, 0, 1000) runs on every Prometheus scrape
to count firing alerts. This is a full table scan with ORDER BY at 15s scrape intervals.

## Acceptance Criteria Status

| Criterion | Status |
|---|---|
| API parity with dashboard data | PARTIAL (ungated free access) |
| Token-authenticated | PARTIAL (no kind/scope isolation) |
| Documented OpenAPI spec | PASS |
| /metrics gauges/counters only, bounded cardinality | PASS |
| Grafana-friendly | PARTIAL (%q edge case) |
| Scrape token optional gate | PASS |
| API + Prometheus = Business tier | FAIL (gate missing, tier missing) |
| Web UI uses only public API (ARCH §3) | PASS |
| /reports/export download | FAIL (route missing) |
