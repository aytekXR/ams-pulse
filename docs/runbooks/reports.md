# Pulse — Usage Reports Runbook

**PRD ref:** F6 (usage/billing reports) · **Status: Shipped (Wave 2 + V3b + Wave-3-Plus)**

---

## Overview

Pulse generates per-tenant viewer-minute and egress usage statements. Operators
create tenant mapping rules that associate stream-name patterns or stream metadata
tags with tenant identifiers. Reports are available as CSV and PDF, with optional
white-label PDF headers. Scheduled exports can push reports to S3.

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

A schedule's `cron` says when its statement runs, in UTC. It accepts standard 5-field cron
and two short forms:

```
MIN HOUR DOM MONTH WEEKDAY     standard cron
MIN HOUR WEEKDAY               Pulse short form
MIN HOUR                       every day
```

| Field | Values |
|---|---|
| `MIN` | 0-59 |
| `HOUR` | 0-23 |
| `DOM` (5-field only) | 1-31 |
| `MONTH` (5-field only) | 1-12 |
| `WEEKDAY` | 0-7 (0 and 7 = Sunday) |

Each field takes `*`, a value, a range `a-b`, a step `*/n` or `a-b/n`, or a comma list
(`1,15`). As in Vixie cron (the cron most Linux systems run), when `DOM` and `WEEKDAY` are both
restricted a day matching either one runs — but a field that begins with `*`, a step like
`*/2` included, counts as unrestricted: `0 6 */2 * 1` runs on odd-numbered days that are
Mondays, not on every odd day plus every Monday. An expression that does not parse, is out of range, or can never fire
(`0 0 31 2 *`) is refused with `422 INVALID_SCHEDULE` — before v0.5.1 it was stored and run a
month later, ranges used only their first value (`1-5` meant Mondays), and `MONTH` was
ignored (a yearly `0 6 1 1 *` ran every month).

Common presets:

| Schedule | 5-field expression | Short form |
|---|---|---|
| Daily at midnight | `0 0 * * *` | `0 0` |
| Monthly on the 1st at 06:00 | `0 6 1 * *` | *(needs DOM — 5-field only)* |
| Weekly, Monday 06:00 | `0 6 * * 1` | `0 6 1` |
| Weekdays at noon | `0 12 * * 1-5` | `0 12 1-5` |
| Quarterly (1 Jan/Apr/Jul/Oct, 06:00) | `0 6 1 1,4,7,10 *` | — |

Every run covers the **previous calendar month** whatever the schedule, so a monthly
schedule on the 1st is the usual choice.

### Creating a schedule via API

The body takes `cron`, `format` (`csv` or `pdf`), an optional `scope` (`app`, `tenant`) and an
optional `whitelabel_header`. Any other field is refused with a 422 that names it — the
statement's scope is `scope`, not `app_filter`/`tenant_filter`, so a mistyped scope can no
longer produce an unscoped statement.

```sh
# Monthly PDF statement for one tenant, on the 1st at 06:00 UTC
curl -X POST http://localhost:8090/api/v1/reports/schedules \
  -H "Authorization: Bearer plt_<admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "cron": "0 6 1 * *",
    "format": "pdf",
    "scope": {"app": "live", "tenant": "tenant-a"},
    "whitelabel_header": {"name": "Acme Streaming", "address": "1 Main St\nSpringfield"}
  }'

# Weekly CSV for everything, Mondays 06:00 UTC (short form)
curl -X POST http://localhost:8090/api/v1/reports/schedules \
  -H "Authorization: Bearer plt_<admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"cron": "0 6 1", "format": "csv"}'
```

### S3 upload

Add S3 config to enable automatic upload of generated reports. `PULSE_S3_ENDPOINT` is what
turns upload on — without it nothing is uploaded, whatever else is set (Pulse logs a warning
at startup when a bucket is set without an endpoint). For AWS use the regional endpoint:

```sh
export PULSE_S3_ENDPOINT=https://s3.us-east-1.amazonaws.com
export PULSE_S3_BUCKET=my-billing-reports
export PULSE_S3_REGION=us-east-1
export PULSE_S3_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export PULSE_S3_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG...
```

Pulse addresses objects path-style: `PUT <endpoint>/<bucket>/<prefix><filename>`, signed with
SigV4.

Or use indirect references (recommended for secrets management):
```sh
export PULSE_S3_ACCESS_KEY_ENV=MY_S3_KEY_ID    # name of env var holding the key ID
export PULSE_S3_SECRET_KEY_ENV=MY_S3_SECRET    # name of env var holding the secret
export MY_S3_KEY_ID=AKIAIOSFODNN7EXAMPLE
export MY_S3_SECRET=wJalrXUtnFEMI/K7MDENG...
```

The indirect reference pattern means S3 credentials are never stored in Pulse config
or the meta database. The credential env vars are read at upload time only.

Default prefix: `reports/`.

### S3-compatible endpoints (MinIO, DigitalOcean Spaces, etc.)

```sh
export PULSE_S3_ENDPOINT=https://minio.internal:9000
export PULSE_S3_REGION=us-east-1    # required even for S3-compatible endpoints
```

---

## White-label config

A schedule's `whitelabel_header` replaces the Pulse header on its statements (PDF and CSV)
with your company name and address:

```json
"whitelabel_header": {"name": "Acme Streaming", "address": "1 Main St\nSpringfield"}
```

`name` is required; `address` is optional and each of its lines (newline-separated) is
printed as its own header line. Any other key is refused (422) — before v0.5.1 a header
written with other keys (`company`, …) was stored and the statement went out unbranded. The
PDF logo is server-wide, set with `PULSE_REPORT_LOGO_PATH`, not per schedule.

> A global `GET/PUT /api/v1/admin/whitelabel` endpoint (one brand config for all schedules)
> is not implemented; the header is set per schedule.

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
