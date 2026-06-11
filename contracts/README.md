# Contracts — Source of Truth

Everything in this directory is a **binding interface contract**. Components
(`server/`, `web/`, `sdk/`) and development agents implement *against* these files;
they never invent their own shapes. Any interface change starts here, as a PR to a
contract file, before any implementation changes.

## Layout

| Path | Contract | Owner agent | Consumers |
|---|---|---|---|
| `openapi/pulse-api.yaml` | Pulse Query/Config REST API (OpenAPI 3.1) | INT-01 | server (implements), web (consumes), docs |
| `events/beacon-event.schema.json` | Player→Collector QoE beacon payload | INT-01 | sdk (emits), server (ingests) |
| `events/ams-server-event.schema.json` | Normalized internal event from AMS sources (REST poll, log tail, Kafka, webhook) | INT-01 | server (collector emits → store ingests) |
| `events/alert-notification.schema.json` | Alert payload sent to notification channels | INT-01 | server (emits), integrations |
| `db/clickhouse/` | ClickHouse event/rollup table migrations | BE-01 | server |
| `db/meta/` | Metadata store (SQLite/Postgres) migrations | BE-02 | server |

## Rules

1. **Versioned, additive-first.** Breaking changes bump the schema `version` field and
   require a migration note in the PR description.
2. **CI-validated.** `ci.yml` lints the OpenAPI spec and validates JSON schemas; server
   and SDK tests include round-trip validation against these schemas.
3. **AMS coupling is isolated.** Only `ams-server-event.schema.json` and
   `server/pkg/amsclient` may reference AMS-specific field names. Everything downstream
   uses the normalized shapes defined here (this is what makes the Phase 3
   Wowza/Red5/Flussonic portability play possible).
