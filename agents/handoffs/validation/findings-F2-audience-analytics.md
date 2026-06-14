# Validation Findings: F2 Audience Analytics

**Verifier:** QA adversarial subagent
**Date:** 2026-06-14
**Area:** F2 — Historical Audience Analytics (geo/device/protocol, time-series)
**Verdict:** PARTIAL — geo/device breakdowns are stubs; enrichment is unwired; GAP-2-001 hides a real lookup coverage gap; 13-month budget measured without dimensional query.

---

## Finding 1 (CRITICAL): Geo and Device Breakdown API Handlers are Stubs

File: server/internal/api/server.go lines 681-687
Both GET /analytics/geo and GET /analytics/devices always return {"rows": []} regardless of data in ClickHouse. No ClickHouse query is executed. No tests assert non-empty data.

## Finding 2 (MAJOR): geoResolver/uaParser Never Passed to REST Poller

File: server/cmd/pulse/serve.go lines 163-177
restpoller.Config has GeoResolver/UAParser fields but serve.go does not pass the configured resolvers. Always uses NoopGeoResolver{}/NoopUAParser{} fallback.

## Finding 3 (MAJOR): Beacon Events Never Geo/UA Enriched

File: server/internal/collector/beacon/beacon.go lines 394-398
batchToDomain discards the HTTP request (r is ignored). Enrichment block is nil. No downstream pipeline applies geo/UA. viewer_sessions from beacon path have empty geo/device fields.

## Finding 4 (MAJOR): REST Path Has No IP Source

File: server/internal/collector/normalize.go line 36
buildEnrichment("", "", geo, ua) — always called with empty IP and UA. BroadcastDTO has no viewer IP fields. Even with wiring fixed, server-side events produce empty enrichment.

## Finding 5 (MAJOR): GAP-2-001 Hides Real Lookup Coverage Gap

TestGeo_MMDBFixture always SKIPS (BuildTestMMDB generates invalid mmdb). The full lookup path (open reader → resolve IP → assert country) is never exercised. Anonymize-before-lookup is only tested against an absent reader, not a working one.

## Finding 6 (MINOR): 13-Month Budget Measured Without Dimensional Query

The gate measured a simple 2-column aggregate (126ms). PRD F2 requires geo/device/protocol breakdowns. The production query would GROUP BY geo_country, client_device, protocol — unmeasured.

## Finding 7 (MINOR): No Tests for Geo/Device Endpoints Returning Data

Zero occurrences of geo/device analytics handler names in any test file.
