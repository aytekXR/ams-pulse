# Feature: Historical Audience Analytics (F2) — Phase 1 core, Phase 2 full

Views, uniques, watch time, peak concurrency, geo, device/OS/browser, protocol
breakdowns over arbitrary date ranges, per stream/app/node. CSV export per stream.

Acceptance: 13-month queries render <3s (server answers from rollups — keep client
charts dumb); geo to country level with optional region.

Owner: FE-01. Data: `/analytics/audience`, `/analytics/geo`, `/analytics/devices`.
