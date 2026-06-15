# Pulse — Productionize Runbook

**PRD ref:** §7.10 (single Go binary, stateless), §7.12 (15-minute install target)

This runbook covers the steps to harden a working Pulse MVP deployment into a
production-grade installation. Work through the sections in order; each section
is independent once secrets and TLS are in place.

---

## 1. TLS and a real domain

Pulse ships a Caddy reverse proxy in the hardened override
(`deploy/docker-compose.hardened.yml`). Caddy auto-provisions TLS via Let's
Encrypt when a real domain is pointed at the server.

### Steps

**1a. Point DNS to the server**

Create an A record (and AAAA if IPv6 is available) for your domain:

```
pulse.example.com  A  <server-public-ip>
```

Propagation takes seconds to a few minutes.

**1b. Set PULSE_DOMAIN in deploy/.env**

```sh
# deploy/.env
PULSE_DOMAIN=pulse.example.com
```

Caddy reads `PULSE_DOMAIN` and requests a Let's Encrypt certificate automatically
on first startup. Ports 80 and 443 must be reachable from the internet (Let's
Encrypt HTTP-01 challenge).

**1c. Open ports 80 and 443**

```sh
# ufw — if active
ufw allow 80/tcp
ufw allow 443/tcp

# Or via the cloud provider firewall / security group: TCP 80, 443 inbound.
```

Note: Docker DNAT bypass means `ufw` rules may not apply to published ports;
check your cloud provider's external firewall as well.

**1d. Remove plain-HTTP host exposure**

The demo override (`docker-compose.override.yml`) publishes pulse on `:80`.
In production, Caddy is the public entry point and pulse should only be
reachable cluster-internally. The hardened override removes the `:80` binding
and restricts pulse to `expose` only (no host ports).

**1e. Start the hardened stack**

```sh
cd /path/to/pulse
docker compose \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  up -d
```

Verify TLS within 30 seconds:

```sh
curl -m 10 https://pulse.example.com/healthz
# Expected: {"status":"ok",...}
```

---

## 2. ClickHouse authentication

The base compose file and the demo override skip ClickHouse authentication
(`CLICKHOUSE_SKIP_USER_SETUP=1`). In production, enable auth so the ClickHouse
native port is not open to unauthenticated connections.

### Steps

**2a. Add credentials to deploy/.env**

Copy the example template and fill in values:

```sh
cp deploy/.env.example deploy/.env   # if not already done
```

Edit `deploy/.env`:

```sh
CLICKHOUSE_USER=pulse
CLICKHOUSE_PASSWORD=<strong-random-password>
```

Generate a password:

```sh
openssl rand -hex 24
```

**2b. Update PULSE_CLICKHOUSE_DSN to include credentials**

In `deploy/.env`, set the DSN with userinfo for both `pulse` and `pulse-migrate`:

```sh
PULSE_CLICKHOUSE_DSN=clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@clickhouse:9000/pulse
```

The hardened override passes these vars to both `pulse` and `pulse-migrate`
services. The `clickhouse-go` driver parses userinfo from the DSN directly.

**2c. Update the ClickHouse healthcheck**

When auth is enabled, the base `SELECT 1` healthcheck fails because it connects
as the anonymous user. The hardened override overrides the healthcheck to pass
credentials:

```yaml
clickhouse:
  healthcheck:
    test: ["CMD", "clickhouse-client",
           "--user", "${CLICKHOUSE_USER}",
           "--password", "${CLICKHOUSE_PASSWORD}",
           "--query", "SELECT 1"]
```

This is already handled by `deploy/docker-compose.hardened.yml`.

**2d. Remove CLICKHOUSE_SKIP_USER_SETUP**

The hardened override does NOT set `CLICKHOUSE_SKIP_USER_SETUP=1`, so the
ClickHouse image enforces the credentials from `CLICKHOUSE_USER` and
`CLICKHOUSE_PASSWORD`.

**2e. Verify migrations and serve**

Run migrations against the authenticated instance:

```sh
docker compose \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  run --rm pulse-migrate
```

Then start the full stack:

```sh
docker compose \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  up -d
```

Check connectivity:

```sh
docker compose logs pulse | grep -E "clickhouse|error|warn"
curl -m 5 https://pulse.example.com/healthz | python3 -m json.tool
# clickhouse.status must be "ok"
```

---

## 3. Secrets management

### PULSE_SECRET_KEY

The secret key encrypts AMS tokens and sensitive config at rest in the SQLite
meta store (AES-256-GCM). Generate it once per installation.

```sh
# Generate:
openssl rand -hex 32
# → e.g. 4a3f...c9b2  (64 hex chars = 32 bytes)
```

Store in `deploy/.env`:

```sh
PULSE_SECRET_KEY=<output-of-openssl>
```

**Never commit `deploy/.env` to git.** It is listed in `.gitignore`.
A `deploy/.env.example` template with placeholder values is the committable form.

### Rotating the AMS token

When an AMS REST token is rotated:

1. Generate the new token in the AMS admin panel.
2. Update `PULSE_AMS_AUTH_TOKEN` in `deploy/.env`.
3. Restart the pulse service to pick up the new value:

```sh
docker compose \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.real-ams.yml \
  up -d pulse
```

No other services need restarting. The meta store token (for the Pulse API) is
a separate credential stored in the meta database; rotate it via:

```sh
# POST with your current admin token:
curl -X POST https://pulse.example.com/api/v1/admin/tokens \
  -H "Authorization: Bearer plt_<current-token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"admin-rotated","role":"admin"}'
# Revoke old token afterward via DELETE /api/v1/admin/tokens/<id>
```

### deploy/.env.example

A committable template belongs at `deploy/.env.example`. It documents all
required variables without real values:

```sh
# deploy/.env.example — copy to deploy/.env and fill in real values.
# This file is safe to commit; deploy/.env is gitignored.

PULSE_SECRET_KEY=replace-with-openssl-rand-hex-32

CLICKHOUSE_USER=pulse
CLICKHOUSE_PASSWORD=replace-with-strong-password
PULSE_CLICKHOUSE_DSN=clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@clickhouse:9000/pulse

PULSE_DOMAIN=pulse.example.com

PULSE_AMS_URL=http://your-ams-host:5080
PULSE_AMS_AUTH_TOKEN=replace-with-ams-rest-token
PULSE_AMS_NODE_ID=standalone
PULSE_AMS_APPLICATIONS=

# Optional: Prometheus scrape token (leave blank to disable /metrics auth)
PULSE_METRICS_TOKEN=
```

---

## 4. Real AMS wiring

Replace the mock-AMS stub with a connection to your production Ant Media Server.

### Steps

**4a. Fill in AMS env vars in deploy/.env**

```sh
PULSE_AMS_URL=http://your-ams-host:5080    # AMS REST base URL (no trailing slash)
PULSE_AMS_AUTH_TOKEN=<ams-rest-token>      # AMS admin or dedicated read token
PULSE_AMS_NODE_ID=node-01                  # Label for this node in Pulse events
PULSE_AMS_APPLICATIONS=live,vod            # Comma-separated; empty = all apps
```

**4b. Start with the real-ams override**

```sh
docker compose \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.real-ams.yml \
  up -d
```

`deploy/docker-compose.real-ams.yml` assigns `mock-ams` to an unused profile so
it does not start. The pulse service receives the four AMS env vars from your
`.env` file.

**4c. Test the AMS source connectivity**

Use `pulse diag` to verify connectivity from inside the container:

```sh
docker compose exec pulse pulse diag
# AMS URL line must show your real host, not mock-ams
# "=== Connectivity ===" section must show AMS reachable
```

Alternatively, test a specific source via the REST API (replace with your
source ID from the onboarding wizard):

```sh
curl -m 10 https://pulse.example.com/api/v1/admin/sources/<source-id>/test \
  -X POST \
  -H "Authorization: Bearer plt_<your-admin-token>"
# Expected: {"status":"ok","stream_count":<n>}
```

**4d. Verify live data collection**

Publish a test stream to AMS, then check the overview endpoint:

```sh
curl -m 10 https://pulse.example.com/api/v1/live/overview \
  -H "Authorization: Bearer plt_<your-admin-token>"
# streams.active should be > 0 within one poll interval (default 5 s)
```

---

## 5. Backups and data retention

### ClickHouse backups

**Backup** (requires ClickHouse running):

```sh
docker compose exec clickhouse \
  clickhouse-client \
    --user "${CLICKHOUSE_USER}" \
    --password "${CLICKHOUSE_PASSWORD}" \
    --query "BACKUP DATABASE pulse TO Disk('backups', 'pulse-$(date +%Y%m%d).zip')"
```

For simpler volume-level snapshots, stop ClickHouse first to ensure a
consistent state:

```sh
docker compose stop clickhouse
# Snapshot the clickhouse-data Docker volume or the underlying host path
# (typically /var/lib/docker/volumes/<project>_clickhouse-data/_data)
docker compose start clickhouse
```

**Retention policy** (already configured in `contracts/db/clickhouse/`):

| Table type | TTL env var | Default |
|---|---|---|
| Raw events (`viewer_sessions`, etc.) | `PULSE_RETENTION_DAYS` | 90 days |
| Rollup tables | `PULSE_ROLLUP_TTL_DAYS` | 395 days (~13 months) |

Adjust these in `deploy/.env` and re-run `pulse migrate` to apply new TTL
values to the DDL.

### SQLite meta store backups

The meta store lives at `/var/lib/pulse/pulse_meta.db` inside the `pulse-data`
volume. It holds alert rules, API tokens, probe config, and user data — not
time-series events.

**File-copy backup** (safe while pulse is running — SQLite WAL mode):

```sh
docker compose exec pulse \
  sqlite3 /var/lib/pulse/pulse_meta.db ".backup /tmp/pulse_meta_backup.db"
docker compose cp pulse:/tmp/pulse_meta_backup.db ./pulse_meta_backup_$(date +%Y%m%d).db
```

**Volume snapshot** (consistent, no sqlite3 required):

```sh
docker compose stop pulse
# Snapshot the pulse-data Docker volume
docker compose start pulse
```

**Recommended schedule:** daily meta backup, daily or weekly ClickHouse backup.
Store backups off-host (S3, NFS, or remote rsync).

---

## 6. Resource limits and metrics scraping

### Resource limits

Add resource limits to prevent a runaway ClickHouse or pulse container from
exhausting host memory. In `deploy/docker-compose.hardened.yml` or a local
override:

```yaml
services:
  pulse:
    deploy:
      resources:
        limits:
          cpus: "2"
          memory: 512M
        reservations:
          memory: 128M

  clickhouse:
    deploy:
      resources:
        limits:
          cpus: "4"
          memory: 2G
        reservations:
          memory: 512M
```

Minimum recommended: pulse 512 MB, ClickHouse 2 GB (for < 100 concurrent streams).
Increase ClickHouse memory for higher stream counts or longer retention windows.

For Docker Compose v2 (standalone, non-Swarm), `deploy.resources` limits are
honored when Docker is running without Swarm mode enabled. If running in Swarm
mode use `mem_limit` + `cpus` at the service level.

### Prometheus metrics scraping

Pulse exposes `/metrics` in Prometheus text format on the main API port (`:8090`).

**Enable scrape authentication** by setting `PULSE_METRICS_TOKEN` in `deploy/.env`:

```sh
PULSE_METRICS_TOKEN=<random-token>  # generate: openssl rand -hex 24
```

When set, Pulse requires `Authorization: Bearer <token>` on `GET /metrics`.

**Example Prometheus scrape config:**

```yaml
# prometheus.yml
scrape_configs:
  - job_name: pulse
    scheme: https
    static_configs:
      - targets: ['pulse.example.com']
    metrics_path: /metrics
    authorization:
      credentials: <PULSE_METRICS_TOKEN value>
    tls_config:
      insecure_skip_verify: false  # Caddy provisions a valid cert
```

**Key metrics to alert on:**

| Metric | Description |
|---|---|
| `pulse_collector_errors_total` | AMS poll errors (spike = AMS unreachable) |
| `pulse_clickhouse_write_errors_total` | CH write failures |
| `pulse_active_streams` | Current active stream count |
| `pulse_ingest_health_score` | Per-stream health score (0–100) |

---

## Quick reference — full production startup

```sh
# From the repo root, after filling in deploy/.env:

docker compose \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.real-ams.yml \
  up -d

# Watch logs:
docker compose logs -f pulse

# Verify:
curl -m 10 https://pulse.example.com/healthz
```

The three-file compose ordering is canonical:
1. `docker-compose.yml` — base (services, volumes, healthchecks; no host ports)
2. `docker-compose.hardened.yml` — TLS via Caddy, CH auth, no plain-HTTP exposure
3. `docker-compose.real-ams.yml` — disables mock-ams, wires real AMS credentials

For upgrades:

```sh
docker compose \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.real-ams.yml \
  pull

docker compose \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.real-ams.yml \
  up -d

# Run migrations after image update:
docker compose \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  run --rm pulse-migrate
```
