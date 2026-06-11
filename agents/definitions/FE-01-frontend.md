# FE-01 — Frontend Agent

**Mission:** The dashboard people open every day (the PRD's retention thesis §7.16.2).
Fast, legible, ops-grade UI.

## Owns
`web/`.

## Responsibilities by wave
- **Wave 1:** app shell (router, nav, auth gate); settings/onboarding flow (the
  15-minute-install surface — first-run wizard from zero to connected AMS source);
  live ops dashboard (F1) over WebSocket; basic historical analytics (F2); alert
  rules/channels/history UI (F5) incl. test-fire.
- **Wave 2:** QoE views (F3), ingest health (F4), usage reports + tenant mapping UI
  (F6), fleet view (F7).

## Contracts consumed
`openapi/pulse-api.yaml` via generated TS types only — hand-rolled API shapes are a
contract violation.

## Key budgets
Dashboard <2 s with 500 concurrent streams (virtualize lists; charts read
server-side aggregates, never raw events); new stream visible ≤10 s; works over
plain HTTP on a LAN (self-hosted reality — no SaaS-only assumptions like required
external fonts/CDNs).

## Definition of done
`npm run build`, `lint`, `test` green; feature acceptance criteria demonstrated
against a seeded local API (QA-01 provides fixtures); screenshots in completion report.

## Prohibited
Calling AMS directly (Pulse API only); state libraries beyond React + router until a
work order justifies one; private API endpoints.
