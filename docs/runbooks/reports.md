# Pulse — Usage Reports Runbook

**PRD ref:** F6 (usage/billing reports) · **Status: Shipped (Wave 2)**  
**Tier:** Enterprise tier required for reports, white-label PDF, and scheduled S3 exports. CSV generation and on-demand report download require Pro tier or higher..

---

## Overview

Pulse generates per-tenant viewer-minute and egress usage statements. Operators
create tenant mapping rules that associate stream-name patterns or stream metadata
tags with tenant identifiers. Reports are available as CSV (all tiers with access)
and PDF (Enterprise white-label). Scheduled exports can push reports to S3.

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

> **Note:** `/api/v1/admin/tenants` routes are implemented but not yet in the
> OpenAPI spec (contracts frozen per D-004). A contract change request (CR-WO204-01)
> has been filed for Wave 3.

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

Scheduled reports run on a 3-field cron schedule (`min hour weekday`).

### Cron expression format

```
MIN HOUR WEEKDAY
```

| Field | Values | Examples |
|---|---|---|
| `MIN` | 0-59 or `*` | `0` = on the hour, `30` = :30 |
| `HOUR` | 0-23 or `*` | `8` = 8 AM |
| `WEEKDAY` | 0-6 (0=Sun) or `*` | `1` = Monday, `1-5` = Mon-Fri |

Common presets:

| Schedule | Cron expression |
|---|---|
| Daily (midnight) | `0 0 *` |
| Weekly (Monday 6 AM) | `0 6 1` |
| Weekdays at noon | `0 12 1-5` |

> **Note:** The 3-field format is Pulse's own simplified parser, not standard
> 5-field cron. Month-day scheduling is not supported; use a daily schedule
> for month-relative scheduling.

### Creating a schedule via API

```sh
curl -X POST http://localhost:8090/api/v1/reports/schedules \
  -H "Authorization: Bearer plt_<admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Monthly viewer-minutes report",
    "cron_expr": "0 0 *",
    "format": "csv",
    "app_filter": "live",
    "tenant_filter": "tenant-a"
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

The PDF statement header can show your company name and address (Enterprise tier):

> **Phase-3 roadmap:** A dedicated `GET/PUT /api/v1/admin/whitelabel` endpoint
> for global brand config (company name, address, logo URL) is planned for Wave 3
> (CR-2, WO-205). In Wave 2, the PDF report header is minimal. White-label PDF polish
> is a Phase-3 item.

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

**Known limitation (D-W2-002):** In the Wave-2 initial release, `accounting.go`
contained wrong ClickHouse column names (`watch_s_state`, `peak_viewers_state`,
`bucket_ts`). This was reported as defect D-W2-002 (owner: BE-02). If
`pulse diag --reconcile` returns an error about unknown identifier, apply the
BE-02 patch for D-W2-002. The in-memory reconciliation unit test (n=10,000,
drift=0.0000%) is unaffected.

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

---

## Known limitations

| Issue | Severity | Status |
|---|---|---|
| `/api/v1/admin/tenants` not in OpenAPI spec (D-004 freeze) | Minor | Wave 3 CR-WO204-01 |
| White-label `GET/PUT /api/v1/admin/whitelabel` endpoint not implemented | Minor | Phase-3 roadmap |
| D-W2-002: live ClickHouse column names wrong in `accounting.go` | Major | BE-02 fix pending |
| Edge-origin viewer dedup not implemented in Wave 2 | Minor | Wave 3 GAP-2-002 |
