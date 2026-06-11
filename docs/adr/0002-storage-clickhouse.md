# ADR 0002: ClickHouse for events, SQLite/Postgres for metadata

**Status:** Accepted · **Date:** 2026-06-11

## Decision

Two stores with a strict split:

1. **ClickHouse** (single-node, pre-tuned, bundled in Docker Compose) for all events
   and materialized rollups. Retention: 90 days raw / 13 months rollups (configurable).
2. **SQLite** (default) or **Postgres** (HA option) for configuration, users, tokens,
   alert rules/history, report schedules, license state — behind one Go interface,
   DDL restricted to the common SQL subset.

## Rationale

- The PRD names ClickHouse and sets budgets that demand a columnar store: 13-month
  queries < 3 s, ~1–2 GB per million viewer-sessions. TTL-based retention and
  materialized-view rollups are native ClickHouse features — no custom compaction code.
- ClickHouse ops burden for tiny customers is a named risk (§7.13). Mitigation is
  pre-tuned single-node defaults sized for a 2-vCPU sidecar, not a different database:
  embedded alternatives (DuckDB) were rejected because concurrent streaming ingest +
  queries under load is exactly ClickHouse's lane, and a Free-tier customer growing
  into Business must not need a storage migration.
- SQLite default (via modernc.org/sqlite, CGO-free) keeps the small install at zero
  extra containers; Postgres is opt-in for customers who already run it.

## Consequences

`pulse migrate` owns all DDL from `contracts/db/`. No ORM — explicit SQL keeps the
two-backend meta store honest and ClickHouse queries reviewable.
