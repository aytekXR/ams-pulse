# Contracts — Source of Truth

Everything in this directory is a **binding interface contract**. Components
(`server/`, `web/`, `sdk/`) and development agents implement *against* these files;
they never invent their own shapes. Any interface change starts here, as a contract
file change, before any implementation changes.

## Layout

| Path | Contract | Owner agent | Consumers |
|---|---|---|---|
| `openapi/pulse-api.yaml` | Pulse Query/Config REST API (OpenAPI 3.1) | INT-01 | server (implements), web (consumes), docs |
| `events/beacon-event.schema.json` | Player→Collector QoE beacon payload | INT-01 | sdk (emits), server (ingests) |
| `events/ams-server-event.schema.json` | Normalized internal event from AMS sources (REST poll, log tail, Kafka, webhook) | INT-01 | server (collector emits → store ingests) |
| `events/alert-notification.schema.json` | Alert payload sent to notification channels | INT-01 | server (emits), integrations |
| `events/fixtures/` | JSON Schema test fixtures (≥2 valid + ≥1 invalid per schema) | INT-01 | server tests, SDK tests |
| `db/clickhouse/0001_init.sql` | ClickHouse event/rollup table migrations | INT-01 | server |
| `db/meta/0001_init.sql` | Metadata store (SQLite/Postgres) migrations | INT-01 | server |

## Rules

1. **Versioned, additive-first.** Breaking changes bump the schema `version` field and
   require a migration note in the PR description.
2. **CI-validated.** `ci.yml` lints the OpenAPI spec and validates JSON schemas; server
   and SDK tests include round-trip validation against these schemas.
3. **AMS coupling is isolated.** Only `ams-server-event.schema.json` and
   `server/pkg/amsclient` may reference AMS-specific field names. Everything downstream
   uses the normalized shapes defined here (this is what makes the Phase 3
   Wowza/Red5/Flussonic portability play possible).
4. **Frozen after wave-1.** Mid-build changes route through ORCH-00 as change requests
   (D-004). See `agents/handoffs/decisions.md`.

## Codegen

### TypeScript types for `web/`

Run after any OpenAPI change:

```sh
# From the repo root:
npx openapi-typescript contracts/openapi/pulse-api.yaml -o web/src/lib/api/schema.d.ts
```

FE-01 should add this npm script to `web/package.json`:

```json
{
  "scripts": {
    "gen:api": "openapi-typescript ../../contracts/openapi/pulse-api.yaml -o src/lib/api/schema.d.ts"
  }
}
```

The generated `schema.d.ts` exports all component schemas as TypeScript interfaces.
Import via `import type { LiveOverview, AlertRule } from '@/lib/api/schema'`.

### Go server types

The server uses the OpenAPI spec for documentation only. Domain types in
`server/internal/domain/` are the Go equivalents, validated by round-trip tests
against the JSON Schema fixtures in `contracts/events/fixtures/`.

### JSON Schema fixtures

`contracts/events/fixtures/` contains:

- `ams-server-event-valid-{1,2}.json` — valid ServerEvent examples
- `ams-server-event-invalid-1.json` — invalid (unknown event type)
- `beacon-event-valid-{1,2}.json` — valid BeaconEvent examples
- `beacon-event-invalid-1.json` — invalid (empty events array)
- `alert-notification-valid-{1,2}.json` — valid AlertNotification examples
- `alert-notification-invalid-1.json` — invalid (unknown state enum)

Validate with:

```sh
npx ajv-cli validate --spec=draft2020 \
  -s contracts/events/ams-server-event.schema.json \
  -d contracts/events/fixtures/ams-server-event-valid-1.json
```

### CI validation commands

```sh
# OpenAPI lint (zero errors required):
npx @redocly/cli lint contracts/openapi/pulse-api.yaml

# Schema compile:
npx ajv-cli compile --spec=draft2020 -s contracts/events/ams-server-event.schema.json
npx ajv-cli compile --spec=draft2020 -s contracts/events/beacon-event.schema.json
npx ajv-cli compile --spec=draft2020 -s contracts/events/alert-notification.schema.json

# Meta DDL (SQLite):
sqlite3 :memory: < contracts/db/meta/0001_init.sql

# ClickHouse DDL (requires /tmp/clickhouse binary):
# Substitute {db}=pulse_test, {retention_days}=90, {rollup_ttl_days}=395
# then feed through clickhouse-client.
```
