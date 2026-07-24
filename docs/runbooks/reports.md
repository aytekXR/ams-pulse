# Pulse — Usage Reports Runbook

**PRD ref:** F6 (usage/billing reports) · **Status: Shipped (Wave 2 + V3b + Wave-3-Plus)**  
**Tier:** Business tier required for all report access (on-demand, CSV, PDF, scheduled S3 exports). White-label PDF header requires Enterprise tier. Free and Pro tiers receive 403 on all report endpoints (enforced V3b VD-35).

---

## Overview

Pulse generates per-tenant viewer-minute and egress usage statements. Operators
create tenant mapping rules that associate stream-name patterns or stream metadata
tags with tenant identifiers. Reports are available as CSV and PDF (Business+ tier)
with white-label PDF headers (Enterprise tier). Scheduled exports can push reports to S3.

---

## Tenant mapping

Tenant mapping rules determine which tenant a viewer session is attributed to.
Each rule has a pattern (glob match on stream name), an optional metadata tag match,
and a tenant identifier.

### Precedence

Rules are evaluated in this order:

1. **Metadata tag match** — if the session's `meta` tags (from the beacon SDK
   `metadata` field) include a `tenant` key matching a rule's tag condition,
   that rule wins.
2. **Stream name glob** — if the stream name matches a rule's glob pattern,
   that rule wins.
3. **Unassigned** — if no rule matches, the session is attributed to tenant `""`.
   Unassigned sessions appear in reports as blank-tenant rows.

### Glob semantics

Patterns use SQL LIKE semantics:
- `%` matches any substring (zero or more characters)
- `_` matches exactly one character
- Literal `%` or `_` can be escaped with `\`

Examples:

| Pattern | Matches |
|---|---|
| `live/tenant-a/%` | `live/tenant-a/stream1`, `live/tenant-a/main` |
| `%auction%` | `live/auction-stage`, `broadcast/auction-replay` |
| `vod/client-_/__` | `vod/client-1/ab`, `vod/client-x/yz` |
| `%` | All streams (catch-all; place last) |

**Warning:** Overlapping patterns have undefined resolution order. Operators
should avoid configuring patterns that match the same stream across multiple rules.
Use metadata tag matching (which has higher precedence) to resolve ambiguity.

### Managing tenant mapping rules

**Via UI:** Settings → Reports → Tenant Mapping → Add rule.

**Via API:**
```sh
# List rules
curl http://localhost:8090/api/v1/admin/tenants \
  -H "Authorization: Bearer plt_<admin_token>"

# Create rule (stream-name glob)
curl -X POST http://localhost:8090/api/v1/admin/tenants \
  -H "Authorization: Bearer plt_<admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"tenant-a","pattern":"live/tenant-a/%","priority":10}'

# Create rule with metadata tag condition (higher precedence)
curl -X POST http://localhost:8090/api/v1/admin/tenants \
  -H "Authorization: Bearer plt_<admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"tenant-b","tag_key":"tenant","tag_value":"b","priority":20}'
```

> **Note:** examples use `http://localhost:8090` — substitute your production
> `https://` URL when running against a proxied deployment.

---

## Egress estimation method

Pulse uses the **bitrate x watch-time** method (disclosed on every report row
via the `egress_method` field):

```
egress_GB = viewer_minutes * avg_bitrate_kbps * 60 * 1000 / 8 / 1,000,000,000
```

Where `avg_bitrate_kbps` is the mean ingest bitrate observed during the session.

This is an **estimation**: actual CDN egress depends on CDN overhead, caching
efficiency, and multi-bitrate ladder weights. Pulse reports the `egress_method`
field on every row so downstream billing systems know which formula was applied.
The CSV export includes an `egress_method` column with value `bitrate_x_watch_time`.

For precision billing, use CDN access logs and treat Pulse egress as an indicator
only. The +/-1% reconciliation budget applies to rollup vs raw session data drift,
not CDN accuracy.

---

## Schedule setup

Scheduled reports accept both standard 5-field cron and Pulse's simplified 3-field format
(V3b VD-36: 5-field cron parser added).

### Cron expression format

**Standard 5-field cron (recommended — matches system cron syntax):**

```
MIN HOUR DOM MONTH WEEKDAY
```

**Pulse simplified 3-field cron (also accepted):**

```
MIN HOUR WEEKDAY
```

| Field | Values | Examples |
|---|---|---|
| `MIN` | 0-59 or `*` | `0` = on the hour, `30` = :30 |
| `HOUR` | 0-23 or `*` | `8` = 8 AM |
| `DOM` (5-field only) | 1-31 or `*` | `1` = 1st of month; use `*` for weekly/daily schedules |
| `MONTH` (5-field only) | 1-12 or `*` | `*` for all months |
| `WEEKDAY` | 0-6 (0=Sun) or `*`; ranges `lo-hi` supported | `1` = Monday, `1-5` = Mon-Fri |

Common presets:

| Schedule | 5-field expression | 3-field expression |
|---|---|---|
| Daily at midnight | `0 0 * * *` | `0 0 *` |
| Monthly on 1st at 6 AM | `0 6 1 * *` | *(not representable in 3-field)* |
| Weekly (Monday 6 AM) | `0 6 * * 1` | `0 6 1` |
| Weekdays at noon | `0 12 * * 1-5` | `0 12 1-5` |

> **Note:** When using 5-field cron, the `DOM` and `MONTH` fields are accepted
> but only `MIN`, `HOUR`, and `WEEKDAY` drive next-run time computation. For
> month-day scheduling, use the 5-field format and set `DOM=1` — the scheduler
> will compute next Monday/Wednesday/etc from the WEEKDAY field.

### Creating a schedule via API

Requires Business tier or higher (returns 403 for Free and Pro).

```sh
# Monthly report on the 1st at 06:00 (5-field cron)
curl -X POST http://localhost:8090/api/v1/reports/schedules \
  -H "Authorization: Bearer plt_<admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Monthly viewer-minutes report",
    "cron_expr": "0 6 1 * *",
    "format": "csv",
    "app_filter": "live",
    "tenant_filter": "tenant-a"
  }'

# Equivalent 3-field form (same schedule, Pulse-specific syntax)
curl -X POST http://localhost:8090/api/v1/reports/schedules \
  -H "Authorization: Bearer plt_<admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Weekly viewer-minutes report",
    "cron_expr": "0 6 1",
    "format": "csv"
  }'
```

### S3 upload

Add S3 config to enable automatic upload of generated reports:

```sh
export PULSE_S3_BUCKET=my-billing-reports
export PULSE_S3_REGION=us-east-1
export PULSE_S3_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export PULSE_S3_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG...
```

Or use indirect references (recommended for secrets management):
```sh
export PULSE_S3_ACCESS_KEY_ENV=MY_S3_KEY_ID    # name of env var holding the key ID
export PULSE_S3_SECRET_KEY_ENV=MY_S3_SECRET    # name of env var holding the secret
export MY_S3_KEY_ID=AKIAIOSFODNN7EXAMPLE
export MY_S3_SECRET=wJalrXUtnFEMI/K7MDENG...
```

The indirect reference pattern means S3 credentials are never stored in Pulse config
or the meta database. The credential env vars are read at upload time only.

Reports are uploaded to `s3://${PULSE_S3_BUCKET}/${PULSE_S3_PREFIX}${filename}`.
Default prefix: `reports/`.

### S3-compatible endpoints (MinIO, DigitalOcean Spaces, etc.)

```sh
export PULSE_S3_ENDPOINT=https://minio.internal:9000
export PULSE_S3_REGION=us-east-1    # required even for S3-compatible endpoints
```

---

## White-label config

The PDF statement header can show your company name and address (Enterprise tier only — `white_label` entitlement):

> **Phase-3 roadmap:** A dedicated `GET/PUT /api/v1/admin/whitelabel` endpoint
> for global brand config (company name, address, logo URL) is planned for Wave 3
> (CR-2, WO-205). In Wave 2, the PDF report header is minimal. White-label PDF polish
> is a Phase-3 item.

Note: **Business tier** enables report access (CSV, PDF, scheduling) but the white-label
PDF header requires **Enterprise tier** (`white_label: true` entitlement). Business-tier
PDF exports have a standard Pulse header.

---

## Reconciliation (`pulse diag --reconcile`)

Reconciliation checks that the rollup tables (`rollup_audience_1d`,
`rollup_audience_1h`) are within +/-1% of the raw `viewer_sessions` data.

```sh
/tmp/pulse diag --reconcile
```

Expected output:
```
pulse diag --reconcile:
  raw viewer-minutes    : 148900.0
  rollup viewer-minutes : 148901.2
  drift                 : 0.0008%
  tolerance <= 1.0%     : PASS
```

Exits non-zero when drift exceeds 1%.

**What the reconciliation checks:**
- `drift_pct = |rollup_viewer_minutes - raw_viewer_minutes| / raw_viewer_minutes * 100`
- Raw source: `SUM(watch_ms / 60000)` from `viewer_sessions`
- Rollup source: `sumMerge(watch_time_s) / 60` from `rollup_audience_1d`

Run reconciliation:
- After major viewer session ingestion events (e.g. large live events)
- Before generating billing statements for Enterprise customers
- After any ClickHouse cluster maintenance (merges, node additions)

D-W2-002 (wrong column names in `accounting.go`) was fixed in the D-009 fix-loop.
The correct column names (`watch_time_s`, `peak_concurrency`, `bucket`) are in place
and verified by `TestAccountant_CHIntegration` (live ClickHouse integration test).

---

## On-demand report generation

```sh
# Generate CSV for a date range
curl "http://localhost:8090/api/v1/reports/usage?from=2026-05-01&to=2026-06-01&format=csv" \
  -H "Authorization: Bearer plt_<admin_token>" \
  -o usage-may-2026.csv

# Generate PDF statement
curl "http://localhost:8090/api/v1/reports/usage?format=pdf" \
  -H "Authorization: Bearer plt_<admin_token>" \
  -o statement.pdf
```

The CSV includes these columns: `app`, `stream_id`, `tenant`, `viewer_minutes`,
`peak_concurrency`, `egress_gb`, `recording_gb`, `egress_method`.

**`peak_concurrency` data source (Wave-3-Plus):** Peak concurrent viewers per stream is
sourced from the `rollup_concurrency_1d` ClickHouse table — a true windowed maximum using
`maxState(viewer_count)` (AggregateFunction from `server_events`) per stream per day,
read back with `maxMerge`. This replaces the prior session-count proxy. The value
represents the highest instantaneous concurrent viewer count recorded in the day's
stream-stats events, regardless of session overlap. Verified by
`TestAccountant_CHIntegration`: overlapping viewer snapshots (peak=25, peak=5) produce
drift=0.0000% (VD-38 CLOSED).

---

## Known limitations

| Issue | Severity | Status |
|---|---|---|
| `/api/v1/admin/tenants` not in OpenAPI spec (D-004 freeze) | Minor | Added via CR-WO204-01 (implemented) |
| White-label `GET/PUT /api/v1/admin/whitelabel` endpoint not implemented | Minor | Phase-3 roadmap |
| D-W2-002: wrong column names in `accounting.go` | Major | **Fixed** (D-009 fix-loop) |
| `peak_concurrency` in billing = session count | Minor | **Fixed Wave-3-Plus** — true windowed max from `rollup_concurrency_1d` (VD-38 CLOSED) |
| Edge-origin viewer dedup | — | **Fixed V3a** — `IsEdgeStream()` implemented; aggregator dedup active (VD-03) |
