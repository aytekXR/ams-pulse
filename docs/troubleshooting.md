# Pulse — Troubleshooting Index

Symptom-first. Each entry states the exact UI text or log prefix the operator sees,
then **Cause / Check / Fix**. Where a runbook or guide covers the topic in depth,
this file links there rather than duplicating it.

---

## UI shows "Polling" instead of "Live" on the dashboard

**Symptom:** The connection-mode badge in the live dashboard shows _Polling_ instead
of _Live_. The dashboard still updates but on a REST-polling cadence rather than via
the WebSocket push path.

**Cause:** The browser's `WebSocket` connection to `/live/ws` is being rejected.
`nhooyr.io/websocket` compares the request `Origin` header against
`PULSE_ALLOWED_WS_ORIGINS`. When that variable is empty the library restricts
connections to the same origin it derives from the HTTP `Host` header. A reverse
proxy (Nginx, Caddy) typically presents the public domain as the `Origin` but
forwards to `pulse:8090`, so the origin check fails and the upgrade is dropped.

**Check:**

```bash
# Inspect the upgrade failure in browser DevTools Network tab — look for
# a WebSocket handshake to /live/ws returning HTTP 403 or a missing
# 101 Switching Protocols response.

# From the server, confirm the env var is absent or wrong:
docker exec pulse-prod-pulse-1 env | grep PULSE_ALLOWED_WS_ORIGINS
```

**Fix:**

Set `PULSE_ALLOWED_WS_ORIGINS` to the public origin (scheme + host, no trailing
slash). Comma-separate multiple origins:

```env
PULSE_ALLOWED_WS_ORIGINS=https://pulse.example.com
```

When fronted by Nginx, also verify the proxy passes the WebSocket upgrade headers.
The required directives are:

```nginx
proxy_http_version 1.1;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

Caddy handles the upgrade automatically for `reverse_proxy` directives; no
additional configuration is required there.

See `docs/AMS-INTEGRATION.md` §3 for the standard Caddy route layout and
`deploy/nginx/pulse.beyondkaira.com.conf` for a reference Nginx configuration.

---

## Dashboard empty / no streams visible after AMS is connected

**Symptom:** The live dashboard shows zero streams, zero viewers, and zero publishers
even though AMS has active broadcasts.

### Cause A — AMS URL unreachable from the Pulse container

**Check:**

```bash
# Verify Pulse can open a TCP connection to AMS:
docker exec pulse-prod-pulse-1 \
  wget -qO- "${PULSE_AMS_URL}/rest/v2/version"

# Look for recurring poll errors:
docker logs pulse-prod-pulse-1 2>&1 | grep -i 'poll error\|connection refused\|dial tcp'
```

**Fix:** Ensure `PULSE_AMS_URL` is set to an address reachable from inside the
Docker network (typically a LAN IP or Docker service name, not `localhost`).
See `docs/AMS-INTEGRATION.md` §2 (Prerequisites) and §3.3 (confirm startup).

---

### Cause B — 403 from AMS REST API (CIDR restriction)

AMS Enterprise Edition restricts REST API access by remote IP by default. If the
Pulse container's IP is not in the AMS allow-list, every REST call returns HTTP 403.

**Check:**

```bash
docker logs pulse-prod-pulse-1 2>&1 | grep 'HTTP 403'
```

**Fix:** In the AMS Management Console go to Settings → Security → REST API IP
Filter and add the Pulse container's Docker bridge IP (or CIDR range). Alternatively,
use the source-test endpoint to get the real error:

```bash
curl -s -X POST https://your-domain/api/v1/admin/sources/${SOURCE_ID}/test \
  -H "Authorization: Bearer plt_<admin-token>"
```

The response `message` field contains the exact HTTP status and body returned by
AMS, making it the fastest diagnostic. See `docs/AMS-INTEGRATION.md` §3.5.

---

### Cause C — Wrong or missing AMS credentials

**Check:** The source-test endpoint (above) returns `"status": "error"` and the
`message` field says `HTTP 401` or `HTTP 403`.

**Fix:**

- AMS 3.x Enterprise uses cookie-session auth (email + password), not a bearer
  token. Set `PULSE_AMS_LOGIN_EMAIL` and `PULSE_AMS_LOGIN_PASSWORD`.
- If you are using `PULSE_AMS_AUTH_TOKEN`, verify the token in AMS Management
  Console → Settings → Security.
- Note: `reachable: true` with `status: "error"` means the stored credential is
  genuinely wrong — the source-test endpoint decrypts and replays the stored
  credential before making the request. See `docs/AMS-INTEGRATION.md` §3.5.

---

### Cause D — Wrong app name in `PULSE_AMS_APPLICATIONS`

**Check:** Streams are on app `WebRTCApp` but `PULSE_AMS_APPLICATIONS=live` is
set. The poller silently skips apps not in the list.

**Fix:** Either set `PULSE_AMS_APPLICATIONS` to the correct comma-separated
app names, or remove it entirely so Pulse auto-discovers all apps via
`GET /rest/v2/applications`. See `docs/AMS-INTEGRATION.md` §3.1.

---

## `/healthz` component degraded — meanings

`GET /healthz` is unauthenticated and returns the full component map. The overall
`status` field and HTTP status code are determined by the component states:

| Component | `status: "down"` meaning | `status: "degraded"` meaning | HTTP code |
|-----------|--------------------------|-------------------------------|-----------|
| `clickhouse` | ClickHouse ping failed within 3 s | — | **503** |
| `meta_store` | SQLite Ping failed (file unwritable, Postgres DSN wrong) | — | **503** |
| `collector` | — | `live.CurrentSnapshot()` returns nil (poller has not yet produced a snapshot) | **200** |
| `kafka` | — | `parse_errors > 0` or consumer lag > 10 000 messages; only present when a Kafka source is configured | **200** |

`collector: degraded` is expected in the first few seconds after startup while the
REST poller completes its first cycle. If it persists beyond 30 s, the poller is
not running — check for AMS connectivity errors in the logs.

`ams_env_configured: false` in the response body means `PULSE_AMS_URL` is not set
in the environment. Pulse is using the default `http://localhost:5080`. Set the
variable to point at a real AMS host and restart.

**Reference:** `docs/runbooks/install.md` Troubleshooting table; `deploy/runbooks/monitoring.md` WARN log taxonomy.

---

## Beacon events not arriving / QoE data absent

**Symptom:** The QoE page is empty or shows no viewer sessions. Beacon POSTs return
HTTP 403 or 401.

### Cause A — License tier gate

Beacon ingest (F3 / QoE) requires **Pro tier or higher**. On Free tier every POST
to `/ingest/beacon` returns HTTP 403 with body `{"code":"LICENSE_REQUIRED","message":"..."}`.

**Check:**

```bash
curl -s https://your-domain/api/v1/admin/license \
  -H "Authorization: Bearer plt_<admin-token>" | jq .tier
```

**Fix:** Apply a Pro or higher license key in Settings → License, or set
`PULSE_LICENSE_KEY` and restart. See `docs/runbooks/install.md` Free tier limits.

---

### Cause B — Wrong token kind

Beacon ingest requires a token of **kind `ingest`**. Posting with an admin token
(`plt_...`) or any token that is not kind=ingest returns HTTP 401
`{"code":"UNAUTHORIZED","message":"invalid ingest token"}`.

**Check:** In Settings → API Tokens confirm the token's kind is `ingest`. The
token must be sent in the `X-Pulse-Ingest-Token` header (not
`Authorization: Bearer`).

**Fix:** Create a new token: Settings → API Tokens → New token → Kind: Ingest.
See `docs/guides/beacon-sdk.md` for the SDK configuration.

---

### Cause C — Dedicated ingest port not exposed

`PULSE_INGEST_LISTEN_ADDR` defaults to empty; when empty, beacon ingest is served
on the main API port (`:8090`). The Docker Compose file sets `:8091` as the
convention. When the variable is set, the beacon endpoint moves to that dedicated
port. If that port is not published in Docker Compose or not forwarded by the
reverse proxy, browser clients cannot reach it.

**Check:**

```bash
docker port pulse-prod-pulse-1 8091
# Empty output = port not published.
```

**Fix:** Either expose port 8091 in `docker-compose.yml` / Compose override, or
remove `PULSE_INGEST_LISTEN_ADDR` so beacon ingest falls back to the main API
port (`:8090`).

---

### Cause D — CORS preflight blocked

`/ingest/beacon` is always CORS-permissive for cross-origin browser SDKs
(`PULSE_CORS_ALLOWED_ORIGINS` does not apply to it). If CORS errors appear in the
browser console on `/ingest/beacon`, the block is most likely at the reverse-proxy
level (Nginx stripping `Access-Control-Allow-Origin`). Check that the proxy does
not override Pulse's CORS headers.

---

## Alerts not firing

### Cause A — Default rule pack ships muted

All four default rules seeded on first run (`stream_offline`, `viewer_drop_pct`,
`node_cpu`, `ingest_bitrate_floor`) are seeded with `muted: true`. They evaluate
and record history but send no notifications until unmuted.

**Check:** In Settings → Alerts → Rules, inspect the muted column. A muted rule
shows evaluations in the History tab but produces no channel notifications.

**Fix:** Assign a channel to the rule and set `muted: false`:

```bash
curl -X PUT https://your-domain/api/v1/alerts/rules/<rule_id> \
  -H "Authorization: Bearer plt_<admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"muted": false, "channel_ids": ["<channel_id>"]}'
```

See `docs/runbooks/alerting.md` Default rule pack and enabled vs muted semantics.

---

### Cause B — Maintenance window is active

A rule with a `maintenance_window` cron expression suppresses notifications
during the configured window. History is still written; notifications are not sent.
The behavior is identical to `muted: true` during the window.

**Check:** Review the rule's `maintenance_window` field in the API or UI.

**Fix:** To override immediately, set `muted: false` and remove the
`maintenance_window` from the rule, then re-add it when ready. See
`docs/runbooks/alerting.md` Maintenance windows.

---

### Cause C — Channel tier gate

| Channel type | Minimum tier |
|---|---|
| Email | Free |
| Slack, Telegram | Pro |
| PagerDuty, Webhook | Business |

Sending a test notification to a Slack channel on a Free-tier install will fail
silently (the evaluator skips the channel). The channel test button in the UI
returns an error if the tier check fails.

**Fix:** Upgrade the license tier or use email for the current tier. See
`docs/runbooks/alerting.md` Channel setup.

---

### Cause D — Rule disabled entirely

`enabled: false` means the rule is not evaluated at all — no history, no
notifications. Distinct from `muted: true`.

**Fix:** Re-enable the rule:

```bash
curl -X PUT https://your-domain/api/v1/alerts/rules/<rule_id> \
  -H "Authorization: Bearer plt_<admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}'
```

---

## Email / Slack test notifications fail

### Email (SMTP)

**Check:**

- `smtp_addr` defaults to `localhost:587` — change to your real SMTP server.
- `starttls` defaults to `false`. If your provider requires STARTTLS, set it
  explicitly in the channel config.
- Test via UI: Settings → Alerts → Channels → Test button.
- Via API: `POST /api/v1/alerts/channels/<channel_id>/test`

**Fix:** Update the channel config with correct `smtp_addr`, `username`,
`password`, and `starttls` values. See `docs/runbooks/alerting.md` Email (SMTP).

---

### Slack (incoming webhook)

**Check:**

- Confirm the license tier is Pro or higher (Slack requires Pro).
- Verify the incoming webhook URL in the Slack App configuration matches what
  is stored in the Pulse channel config.

**Fix:** Re-create the Slack channel with the correct webhook URL. The stored
`slack_webhook_url` is encrypted at rest; if the URL was rotated in Slack, update
the Pulse channel config. See `docs/runbooks/alerting.md` Slack.

---

## Reports are empty

**Symptom:** `GET /api/v1/reports/usage` returns HTTP 403, or returns empty rows.

### Cause A — Tier gate

All report endpoints (on-demand, CSV, PDF, scheduled S3 exports) require
**Business tier or higher**. Free and Pro tier callers receive
`{"code":"LICENSE_REQUIRED",...}`.

**Fix:** Apply a Business or Enterprise license. See `docs/runbooks/reports.md`.

---

### Cause B — No tenant mapping configured

Sessions not matched by any tenant rule appear with a blank `tenant` field.
If no rules exist, all rows show `tenant: ""`. This is correct behaviour, not a bug.

**Fix:** Create tenant mapping rules in Settings → Reports → Tenant Mapping, or via
`POST /api/v1/admin/tenants`. See `docs/runbooks/reports.md` Tenant mapping.

---

### Cause C — No data in the time range

Reports read ClickHouse rollup tables. If no streams were active in the requested
date range, the report is legitimately empty.

**Check:**

```bash
curl "https://your-domain/api/v1/reports/usage?from=2026-01-01&to=2026-02-01&format=csv" \
  -H "Authorization: Bearer plt_<admin-token>"
```

---

## Synthetic probe failures vs real stream failures

Pulse keeps probe data strictly separate from organic QoE data.

- **Probe results** appear only under `/probes` in the dashboard, each row
  labeled with a _Synthetic_ badge. They are never injected into the QoE charts.
- **Probe failures** (TTFB timeout, DNS, HTTP 4xx/5xx, ICE failure) reflect the
  Pulse server's outbound reachability to the stream URL — not a viewer's
  experience.
- A probe `success: false` with `error_code: timeout` means the Pulse server
  could not fetch the manifest within `timeout_s`. It does not mean viewers are
  affected.
- Probes require **Pro tier or higher**. On Free tier, all probe CRUD endpoints
  return HTTP 403 `LICENSE_REQUIRED`.

See `docs/runbooks/probes.md` for the full error code table and coverage matrix.

---

## ClickHouse disk growing unexpectedly

**Check volume size:**

```bash
# See deploy/runbooks/monitoring.md — Disk watch section:
sg docker -c "docker run --rm \
  -v pulse-prod_clickhouse-data:/data:ro \
  busybox du -sh /data"

df -h /    # host filesystem
```

**Causes and fixes:**

- **Raw event TTL** defaults to `PULSE_RETENTION_DAYS=90` (days). Reduce this
  value and restart to apply a shorter retention. ClickHouse applies TTLs
  asynchronously during merges; volume reclamation is not immediate.
- **Rollup TTL** defaults to `PULSE_ROLLUP_TTL_DAYS=395` (approximately 13 months).
- **Memory pressure** can cause failed inserts at peak load. The hardened overlay
  sets a 2 GiB limit on ClickHouse; if `Memory limit (total) exceeded` appears in
  ClickHouse logs, follow the guidance in `deploy/runbooks/monitoring.md`
  ClickHouse memory-pressure WATCH.

**Reference:** `deploy/runbooks/monitoring.md` Disk watch; `docs/runbooks/install.md`
environment variables `PULSE_RETENTION_DAYS` / `PULSE_ROLLUP_TTL_DAYS`.

---

## Migration failures at startup

### Cause A — `PULSE_SECRET_KEY` missing or too short

`pulse serve` (and `pulse migrate`) validate `PULSE_SECRET_KEY` at startup.
When the variable is absent or shorter than 16 bytes the process exits with:

```
PULSE_SECRET_KEY must be set (min 16 bytes); generate with: openssl rand -hex 32
```

This is the most common cause of a `pulse` container that exits immediately in a
fresh Docker Compose deployment.

**Fix:** Generate a key and add it to `deploy/.env`:

```bash
echo "PULSE_SECRET_KEY=$(openssl rand -hex 32)" >> deploy/.env
```

Then restart: `docker compose up -d pulse`. See `docs/runbooks/install.md` Path A
step 3 and `docs/AMS-INTEGRATION.md` §5.3.

---

### Cause B — ClickHouse not yet ready when `pulse-migrate` runs

The one-shot `pulse-migrate` container retries ClickHouse connection up to 10 times
(2 s apart). If ClickHouse takes longer than 20 s to pass its healthcheck the
migrate container exits and Pulse may start against an unmigrated schema.

**Check:**

```bash
docker compose logs pulse-migrate
# Look for: migrate: clickhouse connect failed, retrying
```

**Fix:** Confirm ClickHouse is healthy first, then re-run the migrate container:

```bash
docker compose run --rm --entrypoint pulse \
  -e PULSE_SECRET_KEY \
  -e PULSE_MIGRATIONS_DIR=/contracts/db/clickhouse \
  -v "$(pwd)/contracts:/contracts:ro" \
  pulse migrate
```

See `docs/runbooks/install.md` Path A step 4 (compose override notes).

---

### Cause C — `SYNTAX_ERROR` in ClickHouse startup logs

ClickHouse logs `Code: 62` entries during startup if raw DDL with `{db}`
placeholders was fed directly to ClickHouse (the `/docker-entrypoint-initdb.d/`
anti-pattern). No tables are created.

**Fix:** Never apply Pulse DDL via `initdb.d`. Use only the `pulse-migrate`
container. See `deploy/runbooks/monitoring.md` Known benign ClickHouse startup
messages.

---

## Quickstart `install.sh` failure modes

### Docker cannot be reached (not in group or daemon stopped)

```
Error: Docker is installed but cannot be reached.
       Possible causes:
         • You are not in the "docker" group.
           Fix: sudo usermod -aG docker $USER && newgrp docker
         • The Docker daemon is not running.
           Fix: sudo systemctl start docker
       Re-run this installer after fixing the issue.
```

Re-run the installer after the group change or daemon restart takes effect.

---

### Docker Compose v2 missing

```
Error: Docker Compose v2 not found.
       Install the Compose plugin: https://docs.docker.com/compose/install/
```

`docker compose version` must succeed (the `compose` subcommand, not the legacy
`docker-compose` binary).

---

### GHCR 401 / image not accessible

```
ERROR: The Pulse container image is not accessible from the registry.
  Image : ghcr.io/aytekxr/ams-pulse:0.4.0
```

The GHCR package is currently private. Authenticate first:

```bash
# Create a GitHub PAT with read:packages scope at:
# https://github.com/settings/tokens/new

docker login ghcr.io -u YOUR_GITHUB_USERNAME
# Paste your PAT when prompted for a password.
```

Then re-run `install.sh`. The installer pre-pulls the image before writing any
credentials to disk, so a 401 at this stage is safe to retry.

---

### Healthz 90 s timeout

```
ERROR: Pulse did not report healthy within 90s.
       Last /healthz response: <no response>
```

The installer polls `GET /healthz` for up to 90 s and never claims success without
a `{"status":"ok"}` body. Possible sub-causes:

- **ClickHouse slow to start:** check container logs for `clickhouse: connect failed,
  retrying`. ClickHouse can take up to 30–40 s on a cold VPS before it becomes
  healthy.
- **`PULSE_SECRET_KEY` issue:** the pulse container exited before the healthz
  listener even started. Check `docker compose logs pulse | tail -20`.
- **Port conflict:** another process holds port 8090. Check with
  `ss -tlnp | grep 8090`.

**Fix:** After resolving the underlying cause, re-run `install.sh`. The installer
removes the generated `.env` on failure; the next run generates a fresh
`PULSE_SECRET_KEY`.

---

## License key rejected — Pulse falls back to Free tier

**Symptom:** The Pulse startup log contains:

```
license: init failed, using free tier
```

**Cause:** The value of `PULSE_LICENSE_KEY` (or the file referenced by
`PULSE_LICENSE_FILE`) is invalid, malformed, or expired. Pulse fails open for reads
— already-collected data remains visible — but tier-gated features (Slack alerts,
beacon ingest, probes, reports) fail closed with HTTP 403 until a valid key is set.

**Check:**

```bash
docker logs pulse-prod-pulse-1 2>&1 | grep 'license:'
# Also check the UI: Settings → License shows current tier and expiry.

curl -s https://your-domain/api/v1/admin/license \
  -H "Authorization: Bearer plt_<admin-token>"
```

**Fix:**

1. Obtain a valid key.
2. Set `PULSE_LICENSE_KEY=<key>` in `deploy/.env`.
3. Restart the pulse container: `docker compose up -d pulse`.

The `license_expiry` alert metric can warn you before a key downgrades the
instance. See `docs/runbooks/alerting.md` Supported metrics and
`docs/guides/license-activation.md` for key activation details.

---

*For AMS REST / webhook integration issues see `docs/AMS-INTEGRATION.md` §10.*  
*For stack-level production concerns see `deploy/runbooks/monitoring.md`.*  
*For install path details see `docs/runbooks/install.md`.*
