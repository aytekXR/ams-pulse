# WO-101 Completion Report — Contract freeze, full surface (INT-01)

**Agent:** INT-01  
**Date:** 2026-06-12  
**Work order:** WO-101 (issued by ORCH-00 2026-06-11)

---

## Status: DONE

All acceptance criteria verified by running the actual commands.

---

## Acceptance criteria — verified outputs

### 1. OpenAPI lint — zero errors

```
$ npx @redocly/cli lint contracts/openapi/pulse-api.yaml
validating contracts/openapi/pulse-api.yaml...
contracts/openapi/pulse-api.yaml: validated in 42ms
Woohoo! Your API description is valid. 🎉
```

Result: **0 errors, 0 warnings** (down from the baseline 55 warnings with TODO bodies).

### 2. JSON Schema compilation — all three compile

```
$ npx ajv-cli compile --spec=draft2020 -s contracts/events/ams-server-event.schema.json
schema contracts/events/ams-server-event.schema.json is valid

$ npx ajv-cli compile --spec=draft2020 -s contracts/events/beacon-event.schema.json
schema contracts/events/beacon-event.schema.json is valid

$ npx ajv-cli compile --spec=draft2020 -s contracts/events/alert-notification.schema.json
schema contracts/events/alert-notification.schema.json is valid
```

### 3. Fixtures validate/reject as labeled

Valid fixtures (exit 0):
```
contracts/events/fixtures/ams-server-event-valid-1.json valid
contracts/events/fixtures/ams-server-event-valid-2.json valid
contracts/events/fixtures/beacon-event-valid-1.json valid
contracts/events/fixtures/beacon-event-valid-2.json valid
contracts/events/fixtures/alert-notification-valid-1.json valid
contracts/events/fixtures/alert-notification-valid-2.json valid
```

Invalid fixtures (exit 1 with schema violation):
```
ams-server-event-invalid-1.json — /type: must be equal to one of the allowed values
beacon-event-invalid-1.json    — /events: must NOT have fewer than 1 items
alert-notification-invalid-1.json — /state: must be equal to one of the allowed values
```

### 4. ClickHouse DDL — all 14 objects created

```
$ /tmp/clickhouse client --multiquery < /tmp/pulse_ch_test.sql
(no errors)

$ /tmp/clickhouse client --query "SELECT name, engine FROM system.tables WHERE database = 'pulse_test' ORDER BY name"
beacon_events     MergeTree
mv_audience_1d    MaterializedView
mv_audience_1h    MaterializedView
mv_qoe_1d         MaterializedView
mv_qoe_1h         MaterializedView
mv_usage_1d       MaterializedView
probe_results     MergeTree
rollup_audience_1d AggregatingMergeTree
rollup_audience_1h AggregatingMergeTree
rollup_qoe_1d     AggregatingMergeTree
rollup_qoe_1h     AggregatingMergeTree
rollup_usage_1d   SummingMergeTree
server_events     MergeTree
viewer_sessions   ReplacingMergeTree

Total: 14 tables (9 tables + 5 materialized views)
```

### 5. Meta DDL — executes cleanly in SQLite

```
$ sqlite3 :memory: < contracts/db/meta/0001_init.sql
SQLite DDL: OK
```

Tables created: schema_migrations, users, api_tokens, ingest_tokens, ams_sources,
cluster_nodes, alert_rules, alert_channels, alert_history, report_schedules,
tenants, license, probes, anomaly_baselines.

### 6. Zero TODO markers in contracts/

```
$ grep -r "TODO" contracts/
(no output)
```

### 7. No AMS-specific naming leaks in beacon/rollup/API shapes

```
$ grep -E "broadcastId|broadcast_id|webRTCAdaptor|hlsPlayback|rtmpURL" contracts/openapi/pulse-api.yaml
(no output — portability rule satisfied)
```

---

## Open questions — rulings

### Q1: Timestamp format (from/to params, response ts fields)

**Ruling:** All time parameters (`from`, `to`) accept either Unix epoch milliseconds (integer) or RFC 3339 strings. Responses always return Unix epoch milliseconds integers. Rationale: epoch ms is unambiguous, JSON-safe (no parsing required), and language-neutral. RFC 3339 input is accepted for human usability in direct API calls.

Documented in OpenAPI spec info.description section and each parameter description.

### Q2: Probe-results placement (ClickHouse vs meta store)

**Ruling:** Probe results go in **ClickHouse** (`probe_results` table). Probe config (URL, interval, enabled, last_result summary) goes in the **meta store** (`probes` table).

Rationale: probe results are time-series data with high write frequency and efficient range-query requirements — identical to event/rollup data patterns. The ClickHouse `probe_results` table is TTL-managed (90 days default). The meta store `probes` table holds config only, with a `last_result_id` denormalized field for fast listing without ClickHouse lookup.

Documented in ClickHouse DDL header comments. `GET /probes/{id}/results` queries ClickHouse; `GET /probes` reads from meta store.

### Q3: Anomaly-baseline placement (ClickHouse vs meta store)

**Ruling:** Anomaly baselines go in the **meta store** (`anomaly_baselines` table).

Rationale: baselines are rolling-window statistics (mean, stddev) that are low-cardinality, mutated in-place, and never queried with time-range predicates. They are config-like state, not event-series data. The meta store's `anomaly_baselines` table includes a unique index on (metric, scope, window_s) for efficient lookup. The `GET /anomalies` API queries the meta store for recent flags.

Documented in both DDL files' header comments.

### Q4: /metrics authentication

**Ruling:** `GET /metrics` is **unauthenticated by default**; operators may configure a separate scrape token via `PULSE_METRICS_TOKEN` environment variable. If set, the endpoint requires `Authorization: Bearer <token>`.

Rationale: Prometheus scrape configurations rely on network-level controls (private network, scrape IP allowlist). Requiring auth by default breaks standard Prometheus deployments. The optional env-var mechanism satisfies stricter environments without breaking standard ones. The scrape token is independent from API tokens and scoped only to this endpoint.

Documented in OpenAPI spec at the `/metrics` path description.

---

## Contract changelog

### openapi/pulse-api.yaml

**Before:** 18 paths / 23 operations, all with `description: TODO` bodies, 55 redocly warnings.

**After:** 32 paths / 46 operations / 66 schemas, 0 lint errors, 0 warnings.

New paths added vs. skeleton:
- Promoted from comments: `POST /ingest/beacon`, `GET /metrics`, `GET /healthz`
- Wave-3 MVP additions: `GET /anomalies`, `GET /probes`, `POST /probes`, `PUT /probes/{id}`, `DELETE /probes/{id}`, `GET /probes/{id}/results`
- Full CRUD added: `/alerts/channels/{channelId}` (PUT, DELETE), `/admin/sources/{sourceId}` (PUT, DELETE), `/admin/license` (PUT), `/admin/tokens` (GET, POST, DELETE), `/admin/users` (GET, POST, PUT, DELETE), `/reports/schedules/{scheduleId}` (PUT, DELETE)

New schemas added: 66 component schemas covering all feature response shapes, request bodies, and shared primitives (Error, PaginatedMeta, AlertScope, MaintenanceWindow, ProtocolMix, etc.).

Key decisions encoded in spec:
- OAS 3.1 format (no `nullable: true`; uses `type: [T, 'null']`)
- Three security schemes: `bearerAuth`, `ingestTokenHeader`, `wsTokenQuery`
- Live endpoints documented as serving from in-memory aggregates (gap #4 fix)
- `/metrics` dual-auth model documented
- Secrets fields write-only (documented with `credential_set` indicator pattern)

### events/ams-server-event.schema.json

**Before:** Skeleton with `data` as untyped `type: object`.

**After:** Full `data` payload per event type using `allOf` if/then discriminated on `type` field. All 9 event types have documented required and optional fields:
- `stream_publish_start`: `publish_type` (required)
- `stream_publish_end`: `duration_s`, `reason`
- `stream_stats`: `viewer_count` (required), `viewer_count_by_protocol`, `bitrate_kbps`, `speed_read_kbps`
- `webrtc_client_stats`: `client_id` (required), `rtt_ms`, `jitter_ms`, `packet_loss_pct`
- `ingest_stats`: `bitrate_kbps`, `fps`, `keyframe_interval_s`, `packet_loss_pct`, `jitter_ms`
- `node_stats`: `cpu_pct`, `mem_pct`, `disk_pct`, `net_in_mbps`, `net_out_mbps`, `jvm_heap_used_mb`
- `recording_ready`: `path` (required), `size_bytes`, `duration_s`
- `viewer_join`: `viewer_id` (required), `protocol` (required), `ip_hash`, `user_agent`, `referrer` — **fixes gap #5**
- `viewer_leave`: `viewer_id` (required), `protocol` (required), `watch_time_s`

Added `enrichment` block (geo.country, geo.region, client.device, client.os, client.browser) — **fixes gaps #2/#3** — documented as collector-added, not AMS-sent.

### events/beacon-event.schema.json

**Before:** Skeleton with `data` as untyped `type: object`.

**After:** Full per-type data using `allOf` if/then in the event item schema. All 9 event types documented:
- `session_start`: `page_url`, `autoplay`
- `startup_complete`: `startup_ms` (required), `bitrate_kbps` — CMCD-aligned
- `heartbeat`: `watch_ms` (required), `bitrate_kbps`, `buffer_ms`, `dropped_frames` — CMCD-aligned
- `rebuffer_start`: `buffer_ms`
- `rebuffer_end`: `duration_ms` (required)
- `error`: `code` (required), `message`, `fatal`
- `bitrate_change`: `from_kbps` (required), `to_kbps` (required) — CMCD-aligned
- `resolution_change`: `from` (required), `to` (required)
- `session_end`: `watch_ms`, `reason`

### events/alert-notification.schema.json

**Before:** Skeleton missing cooldown, group_key, test fields. Scope used `stream_id` already but was not explicitly aligned with server-event naming.

**After:**
- Added `cooldown_until` (Unix epoch ms or null) — **fixes gap #6 cooldown**
- Added `group_key` (string or null) — for grouped alert correlation
- Added `test` (boolean, default false) — for test-fire notifications
- `scope.node_id` / `scope.app` / `scope.stream_id` explicitly documented as matching normalized ServerEvent field names — **fixes gap #6 naming alignment**
- `metric` description explicitly uses normalized internal names, not raw AMS names

### db/clickhouse/0001_init.sql

**Before:** Skeleton with `SELECT 1` placeholder.

**After:** Complete DDL:
- Raw tables: `server_events` (MergeTree), `beacon_events` (MergeTree), `viewer_sessions` (ReplacingMergeTree), `probe_results` (MergeTree)
- Rollup tables: `rollup_audience_1h/_1d` (AggregatingMergeTree), `rollup_qoe_1h/_1d` (AggregatingMergeTree), `rollup_usage_1d` (SummingMergeTree)
- Materialized views: mv_audience_1h/1d, mv_qoe_1h/1d, mv_usage_1d
- All tables include geo (country, region) and device (device, os, browser) dimensions — **fixes gap #9**
- Variable substitution: `{db}`, `{retention_days}`, `{rollup_ttl_days}`
- Rationale comments: AggregatingMergeTree vs SummingMergeTree choice documented
- TTL: raw events 90 days, rollups 13 months (395 days)
- Probe-results design decision documented in header

### db/meta/0001_init.sql

**Before:** Skeleton with `SELECT 1` placeholder.

**After:** Complete DDL for all 14 tables:
- `schema_migrations`, `users`, `api_tokens`, `ingest_tokens`, `ams_sources`, `cluster_nodes`, `alert_rules`, `alert_channels`, `alert_history`, `report_schedules`, `tenants`, `license`, `probes`, `anomaly_baselines`
- SQLite/Postgres divergences documented as comments
- `probes` table holds config only; results in ClickHouse (design decision documented)
- `anomaly_baselines` holds rolling window stats; stays in meta store (design decision documented)
- Indexes: `alert_history` by (rule_id, ts) and (alert_id); `anomaly_baselines` unique on (metric, scope, window_s)

### contracts/README.md

Added codegen instructions:
- `openapi-typescript` command and npm script name for FE-01
- Fixture validation commands
- CI validation commands
- Design note on schema file layout

### contracts/events/fixtures/

New directory with 9 fixture files (3 per schema, 2 valid + 1 invalid each).

---

## Gaps resolved

| Gap | Resolution |
|-----|-----------|
| #2 (geo enrichment) | Added `enrichment.geo` block to ams-server-event.schema.json |
| #3 (device enrichment) | Added `enrichment.client` block to ams-server-event.schema.json |
| #4 (live endpoint data source) | Documented in OpenAPI spec info.description and /live path descriptions |
| #5 (viewer_join fields) | Added `viewer_id`, `protocol`, connection metadata to ViewerJoinData |
| #6 (alert scope alignment) | alert-notification scope fields aligned with normalized ServerEvent naming |
| #9 (geo/device rollup dimensions) | Added geo_country, geo_region, client_device, client_os, client_browser to all rollup tables |

---

## Downstream agent acknowledgments required

The following agents build against this freeze and must be notified:

- **BE-01** — server_events, beacon_events, viewer_sessions, probe_results DDL + all event schemas
- **BE-02** — meta DDL (all 14 tables), alert_notification.schema.json
- **FE-01** — OpenAPI response shapes (all 66 schemas), codegen command in contracts/README.md
- **SDK-01** — beacon-event.schema.json finalized payload shapes

After ORCH-00 commits this wave, contracts/ is frozen for waves 1+2+3-MVP per D-004.
Changes route through ORCH-00 as change requests.
