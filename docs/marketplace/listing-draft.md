> **Internal working file.** Submission copy — the exact text to paste into the marketplace
> form — lives in [`listing.md`](listing.md). Do not paste from this file. This file holds
> internal notes, source-code cross-references, decision history, and status tracking only.

---

# Ant Media Marketplace — Listing Working Notes

**Product:** Pulse: Analytics & QoE Monitoring for AMS  
**Prepared:** 2026-07-13; last revised 2026-07-25  
**Contact for submission:** support@beyondkaira.com

---

## Tier entitlement source of truth

The entitlements below are drawn from `server/internal/license/license.go:90–150`
(the authoritative runtime implementation). Where the PRD §7.11 and license.go diverge,
the code governs.

| Feature | Minimum tier | Code gate |
|---------|-------------|-----------|
| QoE beacon events (ingest) | Pro+ | `CheckBeaconIngest()` in `license.go:405` (Pro/Business/Enterprise) |
| Prometheus `/metrics` endpoint | Business+ | `businessTierEntitlements` in license.go |
| 396-day retention | Business+ | `businessTierEntitlements` in license.go |
| CSV export / usage reports | Business+ | `CheckReports()` in `license.go:394` (Business/Enterprise) |
| Anomaly detection | Enterprise | `enterpriseTierEntitlements`; anomaly evaluator checks Enterprise tier flag |
| White-label PDF reports | Enterprise | `WhiteLabel: true` only in `enterpriseTierEntitlements` |
| PagerDuty / Webhook channels | Business+ | `businessTierEntitlements.Channels` |
| Air-gapped licensing | Enterprise (recommended) | Supported at all tiers via `PULSE_LICENSE_FILE` (no tier gate); Enterprise is recommended for air-gapped production |

**MaxNodes note:** The PRD §7.11 states "1 to 2 nodes" for Pro, but the code enforces
`MaxNodes = 10`. The code is the operative value. Ladder: Free 1 / Pro 10 / Business 50 /
Enterprise unlimited.

---

## Pricing and revenue-share context

Pricing is operator-delegated (2026-07-22 decision, committed in `docs/licensing-public.md`).
Operator may override any figure. Annual = 10× monthly (2 months free); standard rates from
year two.

**Revenue-share:** Ant Media's publicly stated first-year vendor terms are 100% to the vendor,
no commission (research 2026-07-22; see `submission-process.md` §1). Post-year-1 terms are not
published — get them in writing at the developer meeting. The PRD's older figure for revenue
share is superseded by the published terms; do not cite it.

---

## Demand evidence citations (internal analysis)

These are public GitHub issues; they do not require operator clearance to cite in the listing.

- **ant-media/Ant-Media-Server#3122** — Prometheus exporter requested 2021, closed 2023
  without implementation. Community workaround via `json_exporter` with a moved blog and lost
  dashboard. Pulse's `/metrics` endpoint ships this natively. Source: `docs/assessment/final-assessment.md` §3.

- **ant-media/Ant-Media-Server#7926** — open 2026-07-06: AMS freezes after ~24 h under high
  RTMP load; Java alive, OS metrics normal, HLS/API dead. Pulse's three-rung detection ladder —
  latency-creep anomaly flag (`ams_api_latency_ms`) → `node_degraded` alert (~15 s) →
  `node_down` on freeze — directly addresses this failure class. Source:
  `docs/assessment/final-assessment.md` §3.

---

## Cluster fleet view — known limitation (listing copy accuracy)

The listing copy in `listing.md` says "cluster fleet view with wire shape verified against AMS
3.0.3 source — live multi-node validation pending." This matches docs/known-limitations.md
LIM-10: cluster mode is unit-tested but not live-validated against a real multi-node AMS
cluster. The code calls a paginated endpoint shape; the exact AMS 3.x behavior is pending
confirmation at the developer meeting or on a real cluster. Do not strengthen the cluster claim
in the listing until live validation is complete.

---

## Air-gapped licensing — listing copy accuracy

Air-gapped licensing is **supported today** via `PULSE_LICENSE_FILE` — license keys are
self-contained ed25519-signed tokens; no connection to any external server is needed at
activation or runtime. The Enterprise tier is recommended for air-gapped production deployments
but the mechanism itself is not gated to Enterprise. The `listing.md` copy reflects this
correctly. (An earlier draft incorrectly labelled this as "roadmap" — that error is fixed in
the submission copy.)

---

## Support and licensing rows (status)

| Row | Status | Notes |
|-----|--------|-------|
| Support channel / SLA | RESOLVED | `support@beyondkaira.com`; Pro 2-day / Business 1-day / Enterprise 4-business-hour; Mon–Fri 09:00–18:00 UTC; mailbox live. Published in `docs/support.md`. |
| Public licensing terms | RESOLVED | Pricing, tiers, Founding Operators campaign, 14-day trial in `docs/licensing-public.md`. |
| Revenue-share terms | PARTIALLY KNOWN | First-year terms publicly stated: 100% to vendor, no commission. Post-year-1 terms: confirm at the developer meeting. |
| Listing submission | NEEDS-OPERATOR | Operator initiates contact with Ant Media developer-relations / marketplace team. |
| AMS version support requirement | NEEDS-OPERATOR | Ask Ant Media what minimum AMS version a marketplace product must support. |

---

## Screenshots — internal context

Screenshots are produced by `qa/marketplace/capture-live-screenshots.mjs` against a
route-mocked live-app build (Vite preview + Playwright; self-contained, starts and stops
its own server; no hardcoded machine paths). The committed set at `docs/marketplace/screenshots/`
holds six listing PNGs (`ss1-dashboard` … `ss6-probes`), one light-theme variant (`ss1-light`),
and nine user-guide shots (`ug-*`); regenerate at any time with the capture script.

SS3 (alerting), SS5 (usage reports), and SS6 (probes) are captured through the live-app
route-mock harness — not static hand-crafted screenshots.

---

*See `listing.md` for the submission copy. See `submission-process.md` for the full process
record, assumptions (A1–A10), and prerequisites. See `docs/licensing-public.md` for the
customer-facing licensing terms.*
