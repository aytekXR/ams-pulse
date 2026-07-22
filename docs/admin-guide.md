# Pulse Administrator Reference

**Audience:** operators who install, configure, and maintain Pulse deployments.
**Scope:** environment variables, token management, user management, license activation,
data retention, port reference, reverse-proxy setup, backup and upgrade pointers.

This document is generated from code (`server/cmd/pulse/config.go` + `server/internal/config/config.go`
+ `server/internal/config/secrets.go`). All defaults are code-verified against the live tree.

**Env var count:** The server production codebase reads **69 distinct `PULSE_*` environment variables**
(fact-ledger verified 2026-07-22). This reference table documents all 69: 6 `_FILE`-convention variants
(`PULSE_SECRET_KEY_FILE`, `PULSE_WEBHOOK_SECRET_FILE`, `PULSE_AMS_AUTH_TOKEN_FILE`,
`PULSE_AMS_LOGIN_PASSWORD_FILE`, `PULSE_METRICS_TOKEN_FILE`, `PULSE_OIDC_CLIENT_SECRET_FILE`) are
addressed via the `_FILE` convention section below rather than as separate table rows; the remaining 63
fixed-name variables each have a dedicated row, plus one dynamic-name row for `PULSE_AMS_<NAME>_TOKEN`.

---

## Contents

1. [Environment variables](#1-environment-variables)
2. [Token management](#2-token-management)
3. [User management](#3-user-management)
4. [License activation](#4-license-activation)
5. [Data retention](#5-data-retention)
6. [Ports](#6-ports)
7. [Reverse proxy](#7-reverse-proxy)
8. [Backup and restore](#8-backup-and-restore)
9. [Upgrade and rollback](#9-upgrade-and-rollback)

---

## 1. Environment variables

Pulse reads all configuration from `PULSE_*` environment variables. A YAML file
(`pulse.yaml` in the process working directory, or `--config=<path>`) can supply
base values; environment variables always override YAML when both are present.

### _FILE convention

Every **secret-bearing** variable listed below with "Yes" in the `_FILE support?`
column supports an alternate delivery path. When `<VAR>_FILE` is set, Pulse reads
the secret from that file path (trimming one trailing newline) and ignores the
plain `<VAR>` value. A missing or unreadable file causes a **hard startup failure**
(fail-closed). Use this for Docker secrets or secret-manager tmpfs mounts.

Variables with `_FILE` support (from `server/internal/config/secrets.go:GetSecret`):
`PULSE_SECRET_KEY`, `PULSE_WEBHOOK_SECRET`, `PULSE_AMS_AUTH_TOKEN`,
`PULSE_AMS_LOGIN_PASSWORD`, `PULSE_METRICS_TOKEN`, `PULSE_OIDC_CLIENT_SECRET`,
`PULSE_AMS_<NAME>_TOKEN` (per-source named tokens).

> **Exception:** `PULSE_LICENSE_KEY` is read directly via `os.Getenv` and does NOT
> support the `_FILE` convention. Use `PULSE_LICENSE_FILE` instead for file-based
> license delivery.

---

### 1.1 Core

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_LISTEN_ADDR` | `:8090` | No | No | HTTP listen address for the UI + API server |
| `PULSE_LOG_LEVEL` | `info` | No | No | Log verbosity: `debug`, `info`, `warn`, `error` |
| `PULSE_MIGRATIONS_DIR` | (empty) | No | No | Path to ClickHouse migration SQL files; embedded migrations are used when unset |
| `PULSE_WEB_DIR` | `/usr/share/pulse/web` | No | No | Filesystem path to the built React UI; the Docker image places the UI here automatically |
| `PULSE_BASE_URL` | (empty) | No | No | Full public URL used in alert deep-link payloads (e.g. `https://pulse.example.com`); takes effect only via the YAML config loader in the current build |

---

### 1.2 Networking

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_INGEST_LISTEN_ADDR` | (empty = main listener) | No | No | Dedicated listen address for the beacon ingest endpoint (e.g. `:8091`); when empty, beacon ingest is served on `PULSE_LISTEN_ADDR` |
| `PULSE_CORS_ALLOWED_ORIGINS` | (empty = disabled) | No | No | Comma-separated CORS origins permitted on `/api/v1/*`; empty = no CORS headers emitted (same-origin requests still work) |
| `PULSE_ALLOWED_WS_ORIGINS` | (empty = same-origin) | No | No | Comma-separated WebSocket origin patterns for `/live/ws`; empty = same-origin only |

---

### 1.3 AMS connection

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_AMS_URL` | `http://localhost:5080` | Yes (prod) | No | AMS REST API base URL |
| `PULSE_AMS_NODE_ID` | `standalone` | No | No | Node identifier emitted in events; used for labeling in multi-node cluster deployments |
| `PULSE_AMS_AUTH_TOKEN` | (empty) | No | Yes | AMS REST bearer token; mutually exclusive with `PULSE_AMS_LOGIN_EMAIL`/`PULSE_AMS_LOGIN_PASSWORD` |
| `PULSE_AMS_LOGIN_EMAIL` | (empty) | No | No | AMS console login email for cookie-session auth; used as a fallback when no bearer token is configured |
| `PULSE_AMS_LOGIN_PASSWORD` | (empty) | No | Yes | AMS console login password for cookie-session auth |
| `PULSE_AMS_APPLICATIONS` | (empty = all) | No | No | Comma-separated AMS application names to poll (e.g. `live,vod`); empty = poll all applications |
| `PULSE_AMS_<NAME>_TOKEN` | (empty) | No | Yes | Per-source bearer token for named AMS sources defined in `pulse.yaml`; `<NAME>` is the uppercase source `name` field (e.g. `PULSE_AMS_MAIN_TOKEN`) |
| `PULSE_POLL_INTERVAL` | `5s` | No | No | AMS REST API poll interval (Go duration string, e.g. `"5s"` or `"10s"`) |

---

### 1.4 Storage and retention

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_CLICKHOUSE_DSN` | `clickhouse://localhost:9000/pulse` | Yes (prod) | No | Full ClickHouse native-protocol DSN; overrides `PULSE_CLICKHOUSE_ADDR` in the `loadEnvConfig` path |
| `PULSE_CLICKHOUSE_ADDR` | (empty) | No | No | ClickHouse `host:port` alternative to a full DSN; takes precedence in the `internal/config` YAML path when set |
| `PULSE_CLICKHOUSE_DATABASE` | `pulse` | No | No | ClickHouse database name |
| `PULSE_META` | `sqlite` | No | No | Meta store backend: `sqlite` (default) or `postgres` |
| `PULSE_META_DSN` | `pulse_meta.db` | No | No | SQLite file path for the `sqlite` backend; ignored when `PULSE_POSTGRES_DSN` is set |
| `PULSE_META_DDL_PATH` | (empty) | No | No | Override path for SQLite schema DDL; embedded DDL is used when unset; has no effect with the `postgres` backend |
| `PULSE_POSTGRES_DSN` | (empty) | No | No | PostgreSQL DSN; when set, automatically selects the `postgres` meta backend and overrides `PULSE_META` |
| `PULSE_RETENTION_DAYS` | `90` | No | No | Raw event TTL in ClickHouse (days) |
| `PULSE_ROLLUP_TTL_DAYS` | `395` | No | No | Rollup table TTL in ClickHouse (days). **Note:** the env var path defaults to 395 days. The YAML config path defaults to `rollup_months: 13`, computed as `13 × 30 = 390 days` by `Config.RollupTTLDays()`. If neither is set, the effective TTL depends on which code path loaded the config. |

---

### 1.5 Collection and enrichment

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_KAFKA_BROKERS` | (empty = disabled) | No | No | Comma-separated Kafka broker addresses; empty = Kafka source disabled |
| `PULSE_KAFKA_GROUP_ID` | `pulse-collector` | No | No | Kafka consumer group ID |
| `PULSE_GEO_MMDB_PATH` | (empty = disabled) | No | No | Path to a MaxMind-format `.mmdb` file for geo enrichment; empty = geo lookup disabled |
| `PULSE_ANONYMIZE_IP` | `false` | No | No | Strip the last octet of viewer IPs before geo lookup and storage (GDPR/KVKK compliance); set `1` or `true` to enable |
| `PULSE_BEACON_SAMPLE_RATE` | `1.0` | No | No | Fraction of beacon events accepted at the ingest endpoint (0.0 = drop all, 1.0 = accept all) |
| `PULSE_SESSION_IDLE_TIMEOUT` | `5m` | No | No | Viewer session idle-close timeout (Go duration string) |
| `PULSE_CLUSTER_DISCOVERY_INTERVAL` | `30s` | No | No | How often Pulse polls AMS for cluster node membership; a new node becomes visible within ≤1 interval |
| `PULSE_INGEST_TARGET_BITRATE_KBPS` | `2000` | No | No | Expected healthy ingest bitrate in kbps; used by the health-score formula |
| `PULSE_INGEST_TARGET_FPS` | `30` | No | No | Expected healthy ingest frame rate; used by the health-score formula |
| `PULSE_ANOMALY_TICK_S` | `0` (= 60 s) | No | No | **Anomaly detector** Welford baseline-update interval in seconds. `0` or unset = 60 s. This controls ONLY the anomaly detector's statistics-update cycle, NOT the alert evaluator (which runs on a separate, hardcoded 5-second tick). Setting this to `5` (as CI does) makes baselines update faster but has no effect on how quickly threshold alerts evaluate. |

---

### 1.6 Webhook

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_WEBHOOK_ADDR` | (empty = disabled) | No | No | HTTP listen address for the AMS webhook receiver (e.g. `:8092`); empty = webhook listener disabled |
| `PULSE_WEBHOOK_SECRET` | (empty) | Cond. | Yes | HMAC-SHA256 shared secret for webhook validation; **required** when `PULSE_WEBHOOK_ADDR` is set; an empty secret with a configured address causes a hard startup failure (fail-closed) |
| `PULSE_WEBHOOK_REQUIRE_TIMESTAMP` | `false` | No | No | Enable `X-Ams-Timestamp` replay protection; enable ONLY after AMS is configured to send and sign the timestamp header; without AMS support every webhook request returns 401 |
| `PULSE_WEBHOOK_TIMESTAMP_SKEW` | `5m` (handler default) | No | No | Acceptance window for `X-Ams-Timestamp` when replay protection is active (Go duration string) |

---

### 1.7 Security

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_SECRET_KEY` | (none) | **Yes** | Yes | AES-GCM encryption key for HMAC token storage and OIDC state-cookie signing; minimum 16 bytes; required for all non-`:memory:` deployments. Generate: `openssl rand -hex 32`. **Rotating this key invalidates all existing HMAC-protected tokens — re-issue tokens after rotation.** See [SECURITY.md](../SECURITY.md). |
| `PULSE_METRICS_TOKEN` | (empty = open) | No | Yes | Bearer token required for `GET /metrics` (Prometheus scrape); empty = endpoint unauthenticated (rate-limited 10 rps / burst 20 per IP regardless) |

---

### 1.8 License

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_LICENSE_KEY` | (empty = Free) | No | **No** | License key string; empty = Free tier (1 node, 7-day retention). **No `_FILE` convention**; the variable is read via `os.Getenv` directly. |
| `PULSE_LICENSE_FILE` | (empty) | No | No | Path to a file containing the license key (for air-gapped/offline installs); read via `os.ReadFile` at startup |
| `PULSE_LICENSE_OFFLINE_FILE` | (empty) | No | No | Legacy config-path variable for offline license verification; present in the YAML config loader (`internal/config`) but has no effect in the production `pulse serve` command path. Use `PULSE_LICENSE_FILE` instead. |
| `PULSE_LICENSE_PUBKEY` | (embedded Pulse CA) | No | No | Hex-encoded ed25519 public key that overrides the embedded Pulse CA for verifying license signatures; leave unset to use the default embedded key |

---

### 1.9 OIDC/SSO

OIDC is disabled when `PULSE_OIDC_ISSUER` is empty. When enabled, `PULSE_OIDC_CLIENT_ID`,
`PULSE_OIDC_CLIENT_SECRET`, and `PULSE_OIDC_REDIRECT_URL` are all required; the server
refuses to start if any is missing.

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_OIDC_ISSUER` | (empty = disabled) | No | No | OIDC provider issuer URL (e.g. `https://accounts.google.com`); setting this enables OIDC login at `/auth/oidc/login` |
| `PULSE_OIDC_CLIENT_ID` | (empty) | Cond. | No | OAuth2 client ID registered with the OIDC provider; required when `PULSE_OIDC_ISSUER` is set |
| `PULSE_OIDC_CLIENT_SECRET` | (empty) | Cond. | Yes | OAuth2 client secret; required when `PULSE_OIDC_ISSUER` is set |
| `PULSE_OIDC_REDIRECT_URL` | (empty) | Cond. | No | Full callback URL registered with the provider, e.g. `https://pulse.example.com/auth/oidc/callback`; required when `PULSE_OIDC_ISSUER` is set |
| `PULSE_OIDC_GROUP_CLAIM` | `groups` | No | No | `id_token` claim name holding group membership for role mapping |
| `PULSE_OIDC_GROUP_ROLE_MAP` | (empty) | No | No | JSON object mapping OIDC group names to Pulse roles, e.g. `{"ops-admins":"admin","pulse-users":"viewer"}` |
| `PULSE_OIDC_DEFAULT_ROLE` | (empty = fail-closed) | No | No | Pulse role assigned when no group claim matches; empty = 403 `GROUP_DENIED` (fail-closed); set `viewer` to grant any authenticated OIDC user read-only access |
| `PULSE_OIDC_SESSION_TTL` | `24h` | No | No | OIDC session cookie/token lifetime (Go duration string, e.g. `"48h"`) |

---

### 1.10 Reports and S3

| Variable | Default | Required? | _FILE support? | What it does |
|---|---|---|---|---|
| `PULSE_REPORTS_DIR` | `pulse-reports` | No | No | Base directory for generated report artifacts (`pulse-usage-*.csv`, `pulse-usage-*.pdf`); relative to the process working directory |
| `PULSE_REPORT_ARTIFACT_RETENTION_DAYS` | `90` | No | No | Prune report artifacts older than this many days on each scheduler tick; `0` = keep forever (never prune); only `pulse-usage-*.{csv,pdf}` files inside `PULSE_REPORTS_DIR` are ever removed |
| `PULSE_REPORT_LOGO_PATH` | (empty = default) | No | No | Filesystem path to a PNG or JPEG logo embedded in generated PDF reports; empty = embedded default Pulse waveform |
| `PULSE_S3_ENDPOINT` | (empty = disabled) | No | No | S3-compatible endpoint URL (e.g. `https://s3.amazonaws.com`); empty = S3 report export disabled |
| `PULSE_S3_BUCKET` | (empty) | Cond. | No | Target S3 bucket; required when `PULSE_S3_ENDPOINT` is set. **Note:** `deploy/.env.example` incorrectly shows `PULSE_S3_EXPORT_BUCKET` — the correct variable name is `PULSE_S3_BUCKET`. |
| `PULSE_S3_PREFIX` | `reports/` | No | No | Object key prefix applied to every uploaded report |
| `PULSE_S3_REGION` | `us-east-1` | No | No | AWS/S3-compatible region. **Note:** `deploy/.env.example` incorrectly shows `PULSE_S3_EXPORT_REGION` — the correct variable name is `PULSE_S3_REGION`. |
| `PULSE_S3_ACCESS_KEY_ENV` | `PULSE_S3_ACCESS_KEY_ID` | No | No | Name of the environment variable that holds the S3 access key ID; indirection allows delivery via a secret manager. Change this only if your secret manager uses a different variable name. If the named variable is also unset at upload time, the uploader falls back to the standard `AWS_ACCESS_KEY_ID` environment variable (`server/internal/reports/s3.go:98`). |
| `PULSE_S3_SECRET_KEY_ENV` | `PULSE_S3_SECRET_ACCESS_KEY` | No | No | Name of the environment variable that holds the S3 secret access key; same indirection pattern as `PULSE_S3_ACCESS_KEY_ENV`. Falls back to `AWS_SECRET_ACCESS_KEY` when the named variable is unset. |
| `PULSE_S3_ACCESS_KEY_ID` | (empty) | Cond. | No | S3 access key ID read at upload time; required when S3 export is enabled. **Note:** `deploy/.env.example` incorrectly shows `PULSE_S3_EXPORT_KEY_ID` — the correct variable name is `PULSE_S3_ACCESS_KEY_ID` (verified in `server/cmd/pulse/config.go:365`). |
| `PULSE_S3_SECRET_ACCESS_KEY` | (empty) | Cond. | No | S3 secret access key read at upload time; required when S3 export is enabled. **Note:** `deploy/.env.example` incorrectly shows `PULSE_S3_EXPORT_SECRET_KEY` — the correct variable name is `PULSE_S3_SECRET_ACCESS_KEY`. |

---

### Corrections to previously-claimed defaults

Two defaults stated in earlier ledger entries were wrong. The table above reflects
the code-verified values; the corrections are:

**`PULSE_ANOMALY_TICK_S`:** previously described in some contexts as "the evaluation
tick." The real behavior: this variable controls **only the anomaly detector's Welford
baseline-update interval** (default 60 s when 0 or unset). It has no effect on the
alert evaluator, which runs on a **separate, hardcoded 5-second tick** that no
environment variable controls. CI sets `PULSE_ANOMALY_TICK_S=5` to make anomaly
baselines converge faster in integration tests; this does not speed up threshold alert
evaluation.

**`PULSE_ROLLUP_TTL_DAYS`:** the env var path (`cmd/pulse/config.go`) defaults to
`395` days. The YAML config path (`internal/config/config.go`) defaults to
`rollup_months: 13`, computed as `13 × 30 = 390 days` by `Config.RollupTTLDays()`.
These two code paths disagree by 5 days. When using the env var path (all Docker
Compose and env-only deployments), the effective default rollup TTL is **395 days**.
When using a `pulse.yaml` without an explicit `rollup_months` key (and without
`PULSE_ROLLUP_TTL_DAYS` set), the effective TTL is **390 days**.

---

## 2. Token management

### Token kinds

Pulse issues two distinct token kinds; they are not interchangeable:

| Kind | Created via | Accepted on | Prefix |
|---|---|---|---|
| `api` | `POST /api/v1/admin/tokens` or first-run bootstrap | `/api/v1/*` routes | `plt_` |
| `ingest` | `POST /api/v1/admin/tokens` | `/ingest/beacon` only | `plt_` |

An `ingest` token presented on an `/api/v1/*` route returns 401 with the message
`"this route requires an API token (kind=api)"`.

### Scopes

`api` tokens carry a `scopes` array. Two effective scopes exist:

| Scope value | Access level |
|---|---|
| `"admin"` | Full read/write access to all `/api/v1/*` endpoints |
| Any other value (e.g. `"viewer"`, `"read"`) | Read-only (GET/HEAD/OPTIONS pass; all mutating methods return 403) |
| Empty array (legacy) | Treated as `admin` for backward compatibility with tokens minted before scopes were enforced |

### First-run bootstrap token

If the meta store has no `api_tokens` rows at startup, `pulse serve` **prints a
one-time admin token to stderr** and stores its hash:

```
pulse: FIRST RUN — generated admin token: plt_<random>
       Save this token; it will not be shown again.
```

This token has `kind=api`, `scopes=["admin"]`. Save it immediately — it is never
shown again. Use it to create additional scoped tokens via the Settings → API Tokens
UI or `POST /api/v1/admin/tokens`.

### Creating tokens via the API

```sh
# Create a read-only API token
curl -s -X POST https://<pulse-host>/api/v1/admin/tokens \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"kind":"api","name":"grafana-readonly","scopes":["viewer"]}'

# Create an ingest token for the beacon SDK
curl -s -X POST https://<pulse-host>/api/v1/admin/tokens \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"kind":"ingest","name":"player-beacon"}'
```

### Rotation and PULSE_SECRET_KEY

API tokens are stored as `HMAC-SHA256(hmacKey, rawToken)` when `PULSE_SECRET_KEY`
is configured. **Rotating `PULSE_SECRET_KEY` invalidates all HMAC-protected tokens.**
After rotation, every token holder must obtain a new token. Plan rotation windows
carefully. See [SECURITY.md](../SECURITY.md) for the full token-storage security
design.

---

## 3. User management

User management is **API-only** as of v0.4.x. The Settings → Users tab in the web
UI shows "User management — coming in a future update" (LIM-25).

**Endpoints** (all require an admin-scoped `api` token):

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/admin/users` | List users (paginated) |
| `POST` | `/api/v1/admin/users` | Create a user |
| `PUT` | `/api/v1/admin/users/{userId}` | Update a user |
| `DELETE` | `/api/v1/admin/users/{userId}` | Delete a user |

Every mutating call is written to the audit log (`GET /api/v1/admin/audit-log`).

User roles are `"admin"` or `"viewer"`. OIDC/SSO user provisioning (Enterprise tier)
uses first-login provisioning via `PULSE_OIDC_GROUP_ROLE_MAP` and is not affected
by the UI gap.

**Workaround:** manage access via API tokens (Settings → API Tokens has a full UI)
instead of named users until the Users tab ships.

---

## 4. License activation

See [docs/guides/license-activation.md](guides/license-activation.md) for the full
step-by-step guide. A summary of the three activation paths:

**Route A — Environment variable at boot (recommended for containers):**
```sh
PULSE_LICENSE_KEY=<your-key>
```
Set before the container starts; takes effect at startup. Requires restart to change.

**Route B — Offline key file at boot:**
```sh
PULSE_LICENSE_FILE=/etc/pulse/license.key
```
Write the key string to a file (no trailing newline required; the server trims it).
Requires restart to take effect.

**Route C — Runtime API activation (no restart required):**
```sh
curl -s -X PUT https://<pulse-host>/api/v1/admin/license \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"key":"<PULSE_LICENSE_KEY value>"}'
```
The new tier takes effect immediately. Confirm with `GET /api/v1/admin/license`.

### Graceful expiry

When a license expires, Pulse **fails open**: the server keeps running and existing
data remains accessible. Paid-tier endpoints return `403 LICENSE_REQUIRED` until a
new key is activated. The tier reverts to Free for gate checks. Activate a renewed
key via Route C (no restart needed) before expiry to avoid a service gap.

---

## 5. Data retention

| Layer | Default TTL | Configured by |
|---|---|---|
| Raw events (ClickHouse) | 90 days | `PULSE_RETENTION_DAYS` |
| Rollup tables (ClickHouse) | 395 days (~13 months) | `PULSE_ROLLUP_TTL_DAYS` |
| Report artifacts (`pulse-usage-*.{csv,pdf}`) | 90 days | `PULSE_REPORT_ARTIFACT_RETENTION_DAYS` |
| Alert history rows (SQLite) | 1000 rows per rule cap | Hard limit in `store.ListAlertHistory` |

**Alert history cap:** the `GET /api/v1/alerts/history` endpoint caps results at 1000
rows per rule. Rows beyond 1000 are dropped from view but are not automatically
deleted from the meta store.

**Report artifact pruning:** only `pulse-usage-*.{csv,pdf}` files inside
`PULSE_REPORTS_DIR` are pruned. The SQLite metastore (`pulse_meta.db`) and ClickHouse
data are never touched by the pruning pass.

**Setting `PULSE_REPORT_ARTIFACT_RETENTION_DAYS=0` disables pruning** (keep forever).
Kubernetes `--from-file` secrets add a trailing newline — Pulse trims it correctly,
so `0\n` is read as `0` and correctly disables pruning.

---

## 6. Ports

| Port | Protocol | Purpose | Configured by |
|---|---|---|---|
| `8090` | HTTP | UI + API server (all `/api/v1/*`, `/healthz`, `/metrics`, static UI) | `PULSE_LISTEN_ADDR` |
| `8091` | HTTP | Beacon ingest (optional separate listener for DMZ deployments) | `PULSE_INGEST_LISTEN_ADDR` |
| `8092` | HTTP | AMS webhook receiver (optional; disabled by default) | `PULSE_WEBHOOK_ADDR` |
| `9000` | TCP | ClickHouse native protocol (internal; `expose:` only, not host-bound) | `PULSE_CLICKHOUSE_DSN` |

**ClickHouse ports 9000 (native) and 8123 (HTTP) are declared with `expose:` in
the base `docker-compose.yml`, not `ports:`.** They are cluster-internal only and are
never bound to the host network.

The meta store (SQLite) is a file on the `pulse-data` Docker volume and is not
network-accessible.

---

## 7. Reverse proxy

### Caddy (default — recommended)

The base stack ships with Caddy as the TLS-terminating reverse proxy. Two Caddyfiles
are provided:

| File | Use case | CSP |
|---|---|---|
| `deploy/config/Caddyfile` | Local development + base overlay | `connect-src 'self' wss://{$PULSE_DOMAIN:localhost} https://{$PULSE_DOMAIN:localhost}` |
| `deploy/config/Caddyfile.prod` | Production with Let's Encrypt TLS | `connect-src 'self' wss://{$PULSE_DOMAIN} wss://pulse.{$PULSE_DOMAIN} https://{$PULSE_DOMAIN} https://pulse.{$PULSE_DOMAIN}` |

Caddy handles WebSocket upgrades automatically. No additional proxy headers are
required for `/live/ws`.

### nginx edge (alternative)

For deployments that already run nginx on the host:

1. Load the shared WebSocket upgrade map (once per host):
   ```sh
   sudo cp deploy/nginx/00-beyondkaira-maps.conf /etc/nginx/conf.d/
   sudo nginx -t && sudo systemctl reload nginx
   ```

2. Copy the Pulse site config and reload nginx:
   ```sh
   sudo cp deploy/nginx/pulse.beyondkaira.com.conf /etc/nginx/sites-available/pulse.conf
   sudo ln -s /etc/nginx/sites-available/pulse.conf /etc/nginx/sites-enabled/
   sudo nginx -t && sudo systemctl reload nginx
   ```

3. Use the additive Docker Compose overlay to publish ports on loopback:
   ```sh
   docker compose \
     -f deploy/docker-compose.yml \
     -f deploy/docker-compose.nginx-edge.yml \
     up -d
   ```

#### WebSocket upgrade requirements

nginx does NOT handle WebSocket upgrades automatically (unlike Caddy). The site config
must include the `$connection_upgrade` map variable and pass the upgrade headers for
`/live/ws`:

```nginx
proxy_http_version 1.1;
proxy_set_header Upgrade  $http_upgrade;
proxy_set_header Connection $connection_upgrade;
```

The `00-beyondkaira-maps.conf` file defines the required `map $http_upgrade $connection_upgrade`
directive. This must be included in the `http {}` context exactly once.

### CSP and CORS notes

- The CSP is enforced by Caddy/nginx, not by Go middleware. Adjust `Caddyfile.prod`
  if you host Pulse on a subdomain other than `pulse.{$PULSE_DOMAIN}`.
- Set `PULSE_CORS_ALLOWED_ORIGINS` when the API is called from a different origin than
  the UI (e.g. Grafana or a custom dashboard).
- Set `PULSE_ALLOWED_WS_ORIGINS` when the live dashboard WebSocket is opened from a
  cross-origin page.

---

## 8. Backup and restore

See [deploy/runbooks/backup-restore.md](../deploy/runbooks/backup-restore.md) for the
full runbook.

**Summary:** the production stack includes a backup sidecar (`deploy/docker-compose.backup.yml`)
that runs on a 24-hour cycle, retaining 7 artifacts. Each backup includes:
- A ClickHouse data export (`.zip`)
- A copy of the SQLite metastore (`pulse_meta.db`)

**PostgreSQL users:** the backup sidecar handles SQLite only. PostgreSQL operators
must configure `pg_dump` separately.

---

## 9. Upgrade and rollback

See [deploy/runbooks/upgrade-rollback.md](../deploy/runbooks/upgrade-rollback.md) for
the full runbook, including the stamped-build pattern and `pre-dNNN` rollback tags.

**Compatibility stance:** ClickHouse migrations are forward-only and are applied
idempotently at startup (`pulse serve` runs `MIGRATE` on boot). No breaking config
changes have been made within the v0.4.x line. Check the `CHANGELOG.md` `[Unreleased]`
section before upgrading.
