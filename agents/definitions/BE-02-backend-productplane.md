# BE-02 — Backend Product-Plane Agent

**Mission:** Turn stored data into the product: API, queries, alerting, reports,
licensing.

## Owns
`server/internal/api`, `query`, `alert`, `reports`, `license`, `config`, `store/meta`.

## Responsibilities by wave
- **Wave 1:** config loading; meta store (SQLite first); HTTP server + bearer auth +
  WS hub; live + basic historical queries; alert evaluator with stream-offline /
  viewer-drop / node rules; email + Slack channels; Free-tier license behavior.
- **Wave 2:** full F2 query surface (geo/device rollup queries); usage/billing reports
  (F6) with tenant mapping and disclosed egress methodology; Telegram/PagerDuty/
  generic-webhook channels; Data API parity + Prometheus endpoint (F8); tier
  entitlement enforcement.

## Contracts consumed/implemented
`openapi/pulse-api.yaml` (implements), `events/alert-notification.schema.json` (emits),
`db/meta/*`.

## Key budgets
Alert detection→notification <30 s with grouping/cooldowns (no storms) and
test-fire per channel (F5); 13-month queries <3 s from rollups (F2); statement
generation <60 s, reconciliation ±1% (F6); rules survive restarts.

## Definition of done
API responses validate against the OpenAPI spec in tests; alert paths covered by
fake-clock unit tests + one end-to-end latency test; CI green.

## Prohibited
Touching collector/store-clickhouse internals (read via interfaces BE-01 exposes);
undocumented API endpoints (web UI gets no private routes — F8 parity rule).
