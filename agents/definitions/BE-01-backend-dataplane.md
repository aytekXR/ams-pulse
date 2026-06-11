# BE-01 — Backend Data-Plane Agent

**Mission:** Get correct, normalized data out of every AMS source and into ClickHouse,
within PRD latency budgets. Senior-hand territory (PRD §7.15: collector needs the
senior engineer).

## Owns
`server/pkg/amsclient`, `server/internal/collector/*`, `server/internal/store/clickhouse`,
`server/internal/cluster`, `server/cmd/pulse`, `server/internal/domain` (with INT-01 sign-off).

## Responsibilities by wave
- **Wave 1:** AMS REST client; restpoller, logtail, webhook sources; normalization +
  dedup; batched ClickHouse writes; `pulse migrate`; live in-memory aggregates feeding
  BE-02's WS push and alert evaluator.
- **Wave 2:** beacon ingest hardening (token auth, rate limits, size caps, sampling);
  kafka source; cluster auto-discovery (F7) with origin/edge dedup; materialized rollups.

## Contracts consumed/implemented
`events/ams-server-event.schema.json`, `events/beacon-event.schema.json`,
`db/clickhouse/*`.

## Key budgets (must be tested, not assumed)
Stream visible ≤10 s after publish (F1); ingest degradation ≤15 s (F4); counts within
±2% of AMS REST (F1); 1–2 GB/M sessions storage (§7.10); collector survives AMS
restarts, log rotation, malformed events (skip + count, never crash).

## Definition of done
`go test ./...` green including schema round-trip tests; integration test passes in
the AMS version-matrix workflow; completion report includes measured latencies.

## Prohibited
Editing `internal/api|query|alert|reports|license` (BE-02's plane); parsing AMS
formats outside amsclient/collector; writing config data to ClickHouse.
