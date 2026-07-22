# Pulse API Quickstart Guide

**Interactive reference:** [docs/api/index.html](api/index.html) (self-contained ReDoc, generated from `contracts/openapi/pulse-api.yaml`)

---

## Table of contents

1. [Authentication](#1-authentication)
2. [Base URL and versioning](#2-base-url-and-versioning)
3. [API surface — 42 paths by tag](#3-api-surface--42-paths-by-tag)
4. [WebSocket live feed](#4-websocket-live-feed)
5. [Beacon ingest endpoint](#5-beacon-ingest-endpoint)
6. [Prometheus metrics endpoint](#6-prometheus-metrics-endpoint)
7. [Rate limits](#7-rate-limits)
8. [Error envelope](#8-error-envelope)
9. [Tier gates and LICENSE\_REQUIRED errors](#9-tier-gates-and-license_required-errors)
10. [Curl examples](#10-curl-examples)

---

## 1. Authentication

### Bootstrap admin token (first run)

On the very first `pulse serve` invocation, when no tokens exist in the
meta store, Pulse auto-generates a random admin token (`plt_<16 hex bytes>`),
stores its SHA-256 hash, and prints it **once** to stderr:

```
[pulse] bootstrap token (one-time): plt_3a9f7c2e1b0d4852...
```

Copy it immediately — the raw value is never stored and cannot be retrieved
again. This token has the `admin` scope and is used to bootstrap all other
configuration.

### Creating scoped tokens

Use the bootstrap token to mint long-lived API tokens for your applications:

```
POST /api/v1/admin/tokens
Authorization: Bearer plt_3a9f7c2e1b0d4852...
Content-Type: application/json

{
  "kind": "api",
  "name": "my-dashboard-token",
  "scopes": ["read"]
}
```

The response body is a `TokenCreated` object. The `token` field contains the
raw token value — it is returned **only on creation**:

```json
{
  "id": "abc123",
  "kind": "api",
  "name": "my-dashboard-token",
  "scopes": ["read"],
  "created_at": 1700000000000,
  "token": "plt_..."
}
```

### Token kinds

| Kind | Header | Used for |
|------|--------|----------|
| `api` | `Authorization: Bearer <token>` | All `/api/v1/*` endpoints |
| `ingest` | `X-Pulse-Ingest-Token: <token>` | `POST /ingest/beacon` only |

An ingest token presented to an `/api/v1/*` route returns `403 WRONG_TOKEN_KIND`.
An API token presented to `POST /ingest/beacon` returns `401`.

### Scopes

API tokens carry a `scopes` array. Two values are recognised:

| Scope | Access |
|-------|--------|
| `admin` | Full read and write access to all admin endpoints |
| `read` (or any other value, or `viewer`) | Read-only — mutating endpoints return `403 FORBIDDEN` |

### Using the Authorization header

Every `/api/v1/*` request must include:

```
Authorization: Bearer <your-api-token>
```

OIDC/SSO browser sessions use a `pulse_session` cookie instead of the header;
the API accepts both.

---

## 2. Base URL and versioning

All REST API paths are prefixed with `/api/v1`:

```
https://pulse.example.com/api/v1/live/overview
```

The default port is `8090`. A `servers` block in the OpenAPI spec declares `/api/v1`
as the base path. No version negotiation header is required. Non-versioned paths
(`/healthz`, `/metrics`, `/ingest/beacon`, `/auth/*`) sit outside the `/api/v1`
prefix deliberately.

Breaking changes will increment the version segment (`/api/v2`). The current surface
is `v1`; no `v2` exists.

---

## 3. API surface — 42 paths by tag

The spec defines 42 URL paths across 59 HTTP operations, grouped into 12 tags.
The counts below are operations (method + path combinations), matching the OpenAPI
`tags` grouping.

### live — 3 operations

Real-time dashboard data from in-memory aggregates (≤10 s lag, never from ClickHouse).

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/live/overview` | Snapshot: total viewers, publishers, protocol mix, node health |
| GET | `/live/streams` | Paginated active-stream list with per-stream viewer counts and health score |
| GET | `/live/ws` | WebSocket upgrade for server-push dashboard events |

### analytics — 3 operations

Historical audience analytics from hourly/daily ClickHouse rollups (≤3 s for ≤13-month spans).

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/analytics/audience` | Views, uniques, watch-time and peak concurrency by time bucket |
| GET | `/analytics/geo` | Country (and optional region) breakdown |
| GET | `/analytics/devices` | Device / OS / browser / protocol breakdown |

### qoe — 2 operations

Viewer-side Quality of Experience and publisher ingest health.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/qoe/summary` | Startup time (p50/p95), rebuffer ratio, error rate; sliceable by country and device |
| GET | `/qoe/ingest` | Server-side ingest health: bitrate, fps, packet loss, health score, drop events |

### alerts — 10 operations

Alert rules, notification channels, test fire, and history.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/alerts/rules` | List alert rules (paginated) |
| POST | `/alerts/rules` | Create an alert rule |
| PUT | `/alerts/rules/{ruleId}` | Update an alert rule |
| DELETE | `/alerts/rules/{ruleId}` | Delete an alert rule |
| GET | `/alerts/channels` | List notification channels |
| POST | `/alerts/channels` | Create a notification channel (email, slack, telegram, pagerduty, webhook) |
| PUT | `/alerts/channels/{channelId}` | Update a notification channel |
| DELETE | `/alerts/channels/{channelId}` | Delete a notification channel |
| POST | `/alerts/channels/{channelId}/test` | Fire a test notification to a channel |
| GET | `/alerts/history` | Alert firing history (filterable by rule, state, and time range) |

### reports — 6 operations

Usage and billing reports.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/reports/usage` | Viewer-minutes, peak concurrency, egress GB, recording GB |
| GET | `/reports/export` | Download usage report as CSV attachment (Business+) |
| GET | `/reports/schedules` | List scheduled report exports |
| POST | `/reports/schedules` | Create a scheduled CSV/PDF export |
| PUT | `/reports/schedules/{scheduleId}` | Update a scheduled report |
| DELETE | `/reports/schedules/{scheduleId}` | Delete a scheduled report |

### admin — 20 operations

Configuration: data sources, license, tokens, users, tenants, audit log.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/admin/sources` | List configured AMS data sources |
| POST | `/admin/sources` | Add an AMS data source |
| PUT | `/admin/sources/{sourceId}` | Update a data source |
| DELETE | `/admin/sources/{sourceId}` | Delete a data source |
| POST | `/admin/sources/{sourceId}/test` | Test connectivity to an AMS source |
| GET | `/admin/license` | License status, tier, limits, and validity |
| PUT | `/admin/license` | Activate or update license key |
| GET | `/admin/audit-log` | Append-only audit trail (newest first) |
| GET | `/admin/tokens` | List API and ingest tokens |
| POST | `/admin/tokens` | Create an API or ingest token |
| DELETE | `/admin/tokens/{tokenId}` | Revoke a token (idempotent — 204 even if already revoked) |
| GET | `/admin/users` | List local users |
| POST | `/admin/users` | Create a local user |
| PUT | `/admin/users/{userId}` | Update a local user |
| DELETE | `/admin/users/{userId}` | Delete a local user (idempotent) |
| GET | `/admin/tenants` | List tenant definitions (Business+) |
| POST | `/admin/tenants` | Create a tenant definition |
| GET | `/admin/tenants/{tenantId}` | Get a tenant definition |
| PUT | `/admin/tenants/{tenantId}` | Update a tenant definition |
| DELETE | `/admin/tenants/{tenantId}` | Delete a tenant definition |

### fleet — 1 operation

Cluster topology.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/fleet/nodes` | Discovered AMS nodes with role (origin/edge/standalone), health, version, and load |

### anomalies — 1 operation

Baseline-deviation anomaly flags (Enterprise tier).

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/anomalies` | Anomaly flags where a metric exceeded `min_sigma` deviations from its baseline |

### probes — 5 operations

Synthetic stream probes (F10).

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/probes` | List synthetic probe configurations |
| POST | `/probes` | Create a synthetic stream probe |
| PUT | `/probes/{probeId}` | Update a probe |
| DELETE | `/probes/{probeId}` | Delete a probe |
| GET | `/probes/{probeId}/results` | Time-range query of probe results |

### operational — 2 operations

Unauthenticated infrastructure endpoints.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/healthz` | Component liveness check (ClickHouse, meta store, collector, Kafka) |
| GET | `/metrics` | Prometheus text exposition (Business+, optional scrape token) |

### ingest — 1 operation

Beacon QoE ingest (internet-facing, separate token).

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/ingest/beacon` | Receive a batch of player QoE beacon events |

### auth — 5 operations

OIDC/SSO login flow.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/auth/oidc/login` | Initiate OIDC authorization code flow (PKCE S256) |
| GET | `/auth/oidc/callback` | Receive authorization code, set `pulse_session` cookie |
| POST | `/auth/oidc/logout` | Revoke OIDC session and clear cookie (idempotent) |
| GET | `/auth/oidc/status` | Discover whether OIDC SSO is configured (unauthenticated) |
| GET | `/auth/me` | Return the current authenticated identity (name, role, auth\_method) |

---

## 4. WebSocket live feed

**Path:** `GET /api/v1/live/ws`

Upgrades to a WebSocket connection. The server pushes typed JSON messages:

| Type | Frequency | Payload |
|------|-----------|---------|
| `snapshot` | On connect (initial state, sent once) | Full `LiveOverview` object |
| `delta` | On any live-feed state change | Partial `LiveOverview` with only changed keys |
| `heartbeat` | Every 30 s | None (empty payload) |

**Message envelope:**

```json
{
  "type": "snapshot",
  "ts": 1700000000000,
  "payload": { ... }
}
```

For `snapshot` and `delta`, `payload` conforms to the `LiveOverview` schema.
For `heartbeat`, `payload` is absent.

### Authentication options

The OpenAPI spec documents three authentication mechanisms for this endpoint
(security schemes `bearerAuth`, `wsTokenQuery`, `cookieAuth`). The server
implementation additionally recognises a fourth browser-native mechanism not
captured in the OpenAPI spec:

| Mechanism | How | Documented in |
|-----------|-----|---------------|
| Bearer header | `Authorization: Bearer <token>` in the upgrade request | OpenAPI spec |
| Query parameter | `?token=<token>` appended to the WebSocket URL | OpenAPI spec |
| Session cookie | `pulse_session` cookie (set by OIDC callback) | OpenAPI spec |
| Subprotocol header | `Sec-WebSocket-Protocol: pulse.v1, <token>` — the server negotiates the `pulse.v1` marker and extracts the token from the offered subprotocol list (S73/D-140); the raw token never appears in the URL or proxy access logs | Implementation only |

All four mechanisms are validated identically via `downloadAuthMiddleware`
(`server/internal/api/server.go`). The subprotocol mechanism is the recommended
approach for browser clients that cannot set `Authorization` headers and need to
keep the token out of the URL.

Example WebSocket URL with query-param auth:

```
wss://pulse.example.com/api/v1/live/ws?token=plt_abc123
```

---

## 5. Beacon ingest endpoint

**Path:** `POST /ingest/beacon`

The internet-facing QoE beacon receiver. Used exclusively by the browser/player SDK.
It is deliberately outside the `/api/v1` prefix.

### Authentication

```
X-Pulse-Ingest-Token: <ingest-token>
```

Ingest tokens are created via `POST /api/v1/admin/tokens` with `"kind": "ingest"`.
They are revocable per-stream and listed with `?kind=ingest`.

### Constraints

| Parameter | Value |
|-----------|-------|
| Max body size | 64 KB (enforced by handler; returns `413` on excess) |
| Max events per batch | 100 |
| Rate limit (dedicated ingest port) | 100 req/s per token, burst 200 |
| Rate limit (main port `/ingest/beacon`) | 100 req/s per token, burst 200 |
| Tier gate | Pro+ (`LICENSE_REQUIRED` on Free tier) |

### Request body (`BeaconBatch`)

```json
{
  "events": [
    {
      "version": 1,
      "session_id": "550e8400-e29b-41d4-a716-446655440000",
      "stream_id": "my-stream",
      "app": "live",
      "meta": { "tenant": "acme-corp" },
      "player": { "kind": "hls.js", "sdk_version": "1.5.4" },
      "events": [
        { "type": "session_start", "ts": 1700000000000 },
        { "type": "startup_complete", "ts": 1700000001200 }
      ]
    }
  ]
}
```

Valid event types: `session_start`, `startup_complete`, `heartbeat`,
`rebuffer_start`, `rebuffer_end`, `error`, `bitrate_change`,
`resolution_change`, `session_end`.

### Response

```
HTTP 202 Accepted
```

```json
{
  "accepted": 10,
  "rejected": 1,
  "errors": [{ "index": 5, "reason": "missing required field: session_id" }]
}
```

Schema-invalid events within a batch are dropped and reported; the batch as
a whole still returns `202`. A `422` is only returned when every event fails
validation.

---

## 6. Prometheus metrics endpoint

**Path:** `GET /metrics`  
**Tier gate:** Business+ (`403 LICENSE_REQUIRED` on Free and Pro)  
**Full guide:** [docs/guides/prometheus.md](guides/prometheus.md)

Returns Prometheus text exposition format (`text/plain; version=0.0.4`).

### Authentication

Unauthenticated by default. Set `PULSE_METRICS_TOKEN` to require a scrape token:

```
Authorization: Bearer <scrape-token>
```

The scrape token is independent from API tokens and scoped only to `/metrics`.
Rate limit: 10 req/s per IP, burst 20.

### Gauge metrics (7 total)

| Metric | Labels | Description |
|--------|--------|-------------|
| `pulse_live_viewers` | — | Total concurrent viewers across all active streams |
| `pulse_live_streams` | — | Number of streams with an active publisher |
| `pulse_live_publishers` | — | Number of active publisher connections |
| `pulse_ingest_bitrate_kbps` | — | Aggregate ingest bitrate (Kbps) across all streams |
| `pulse_node_cpu_pct` | `node` | CPU utilization % per AMS node |
| `pulse_node_mem_pct` | `node` | Memory utilization % per AMS node |
| `pulse_alerts_firing` | — | Count of `firing`-state rows in alert history (cumulative — not a live incident gauge) |

---

## 7. Rate limits

| Endpoint / surface | Limit | Burst | Notes |
|--------------------|-------|-------|-------|
| `POST /ingest/beacon` (dedicated ingest port) | 100 req/s per token | 200 | Token-bucket, per ingest token |
| `POST /ingest/beacon` (main port) | 100 req/s per token | 200 | Same token-bucket logic |
| `GET /metrics` | 10 req/s per IP | 20 | IP-based; returns 429 on excess |
| All other `/api/v1/*` | No hard-coded global limit | — | Individual endpoints may be bounded by upstream ClickHouse/meta store throughput |

Exceeded limits return `429 Too Many Requests` with the standard error envelope.

---

## 8. Error envelope

All `4xx` and `5xx` responses use a consistent `Error` schema:

```json
{
  "code": "INVALID_PARAM",
  "message": "parameter 'from' must be a valid timestamp",
  "details": {
    "field": "from",
    "received": "not-a-date"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `code` | string | Machine-readable error code (e.g. `INVALID_PARAM`, `RATE_LIMITED`, `LICENSE_REQUIRED`, `WRONG_TOKEN_KIND`, `NOT_FOUND`) |
| `message` | string | Human-readable description |
| `details` | object? | Optional structured context: field errors, received values, etc. |

Common HTTP status codes:

| Status | Meaning |
|--------|---------|
| `400` | Invalid request parameters |
| `401` | Missing or invalid authentication |
| `403` | Authenticated but not authorized (scope, tier, or token kind) |
| `404` | Resource not found |
| `409` | Conflict — duplicate name or resource already exists |
| `413` | Beacon body exceeds 64 KB |
| `422` | Well-formed request but semantically invalid |
| `429` | Rate limit exceeded |
| `500` | Internal server error |
| `503` | Health check: one or more components degraded |

---

## 9. Tier gates and LICENSE\_REQUIRED errors

Pulse enforces feature access by license tier. Calling a tier-gated endpoint on an
insufficient tier returns:

```
HTTP 403 Forbidden
```

```json
{
  "code": "LICENSE_REQUIRED",
  "message": "Prometheus endpoint (F8) requires Business tier or higher (current: \"pro\")"
}
```

| Feature | Minimum tier |
|---------|-------------|
| Beacon ingest (`/ingest/beacon`) | Pro |
| CSV export (`/reports/export`) | Business |
| Prometheus (`/metrics`) | Business |
| Multi-tenant (`/admin/tenants`) | Business |
| Data API (public `/api/v1` with ingest token) | Pro |
| White-label PDF reports | Enterprise |
| Anomaly detection (`/anomalies`, anomaly alert rules) | Enterprise |
| SSO/OIDC (`/auth/oidc/*`) | Enterprise |

Free tier requires no license key and never phones home. Upgrade by calling
`PUT /api/v1/admin/license` with your license key, or by setting
`PULSE_LICENSE_KEY` before starting Pulse.

---

## 10. Curl examples

All examples assume:
- Pulse is running at `https://pulse.example.com`  
- Replace `$TOKEN` with your API bearer token  
- Replace `$INGEST_TOKEN` with your ingest token

### 1. Get the live dashboard snapshot

```bash
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  https://pulse.example.com/api/v1/live/overview | jq .
```

**Expected response (200):** `LiveOverview` object with `total_viewers`,
`total_publishers`, `protocol_mix`, per-app breakdown, and node health array.

---

### 2. Create a scoped read-only API token

```bash
curl -s -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"api","name":"grafana-readonly","scopes":["read"]}' \
  https://pulse.example.com/api/v1/admin/tokens | jq '{id:.id, token:.token}'
```

**Expected response (201):** `TokenCreated` — copy the `token` field immediately;
it is returned only once.

---

### 3. Query audience analytics for the last 7 days (daily buckets)

```bash
# Unix epoch ms timestamps
FROM=$(date -d '7 days ago' +%s)000
TO=$(date +%s)000

curl -s \
  -H "Authorization: Bearer $TOKEN" \
  "https://pulse.example.com/api/v1/analytics/audience?from=${FROM}&to=${TO}&interval=day" \
  | jq '{total_views:.totals.views, peak:.totals.peak_concurrency}'
```

**Expected response (200):** `AudienceResponse` with `totals` and a `timeseries`
array of daily `AudienceBucket` objects.

---

### 4. Create a threshold alert rule (fires when viewers drop below 10)

```bash
curl -s -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Low viewer count",
    "metric": "viewer_count",
    "operator": "lt",
    "threshold": 10,
    "window_s": 300,
    "severity": "warning",
    "enabled": true,
    "channel_ids": []
  }' \
  https://pulse.example.com/api/v1/alerts/rules | jq '{id:.id, name:.name}'
```

**Expected response (201):** `AlertRule` object with the generated `id`.

---

### 5. Post a beacon event batch from a player (using ingest token)

```bash
curl -s -X POST \
  -H "X-Pulse-Ingest-Token: $INGEST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "version": 1,
        "session_id": "550e8400-e29b-41d4-a716-446655440000",
        "stream_id": "my-stream",
        "app": "live",
        "player": {"kind": "hls.js", "sdk_version": "1.5.4"},
        "events": [
          {"type": "session_start", "ts": 1700000000000},
          {"type": "startup_complete", "ts": 1700000001200}
        ]
      }
    ]
  }' \
  https://pulse.example.com/ingest/beacon | jq '{accepted:.accepted, rejected:.rejected}'
```

**Expected response (202):** `{"accepted":2,"rejected":0}`. Note this path is
`/ingest/beacon` without the `/api/v1` prefix.

---

*For the complete schema reference, request/response examples, and parameter
details, open the interactive reference: [docs/api/index.html](api/index.html)*
