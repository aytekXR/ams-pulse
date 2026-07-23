# Pulse — Productionize Runbook

**PRD ref:** §7.10 (single Go binary, stateless), §7.12 (15-minute install target)

This runbook covers the steps to harden a working Pulse MVP deployment into a
production-grade installation. Work through the sections in order; each section
is independent once secrets and TLS are in place.

---

## 1. TLS and a real domain

In production, TLS is terminated by **nginx on the host** — not by anything in
the compose stack. The stack (`deploy/docker-compose.prod.yml`) publishes the
app on the loopback interface only (`127.0.0.1:8090/8091/8092`); host nginx
proxies the public `:80/:443` to those ports. Reference vhosts live in
`deploy/nginx/` (one `server` file per subdomain); the reference deployment's
cert lives at `/etc/letsencrypt/live/beyondkaira.com/` and is renewed by
certbot's systemd timer.

> The canonical compose ordering is: **prod + real-ams + backup**; see step 1e
> and the Quick Reference for the full command. **Omitting
> `-f deploy/docker-compose.backup.yml` on `up -d` silently removes the backup
> sidecar** (real-ams overlay: §4; backup overlay: §5).
> Stop anything holding host `:80` first (e.g. the demo: `docker compose -p pulse down`).

### Steps

**1a. Point DNS to the server**

Create an A record (and AAAA if IPv6 is available) for your domain:

```
pulse.example.com  A  <server-public-ip>
```

Propagation takes seconds to a few minutes.

**1b. Install nginx, a vhost, and a certificate**

```sh
apt-get install -y nginx certbot python3-certbot-nginx
# Adapt the reference vhosts (FQDNs are hard-set per file — edit to your domain):
cp deploy/nginx/00-beyondkaira-maps.conf /etc/nginx/conf.d/   # shared WebSocket map, once per host
cp deploy/nginx/pulse.beyondkaira.com.conf /etc/nginx/sites-available/pulse.example.com.conf
ln -s /etc/nginx/sites-available/pulse.example.com.conf /etc/nginx/sites-enabled/
# Obtain a certificate (HTTP-01 webroot; certbot's systemd timer auto-renews):
certbot certonly --webroot -w /var/www/html -d pulse.example.com
nginx -t && systemctl reload nginx
```

nginx does **not** read a domain from env — the FQDNs and cert paths are set in
each `.conf`. Ports 80 and 443 must be reachable from the internet for the
HTTP-01 challenge.

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
In production, host nginx is the public entry point and pulse binds
`127.0.0.1` only — `deploy/docker-compose.prod.yml` publishes no public port.

**1e. Start the production stack**

```sh
cd /path/to/pulse
docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
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
docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
  run --rm pulse-migrate
```

Then start the full stack:

```sh
docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
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

### Docker secrets / `_FILE` variants (D-052)

Several secret variables are resolved via `config.GetSecret()`, which checks for a
`${NAME}_FILE` companion first: if that env var exists, Pulse reads the secret value
from the file path it contains (compatible with Docker secrets mounted at
`/run/secrets/<name>`). Variables with `_FILE` support:

| Variable | `_FILE` supported? |
|---|---|
| `PULSE_SECRET_KEY` | Yes |
| `PULSE_AMS_AUTH_TOKEN` | Yes |
| `PULSE_AMS_LOGIN_PASSWORD` | Yes |
| `PULSE_WEBHOOK_SECRET` | Yes |
| `PULSE_METRICS_TOKEN` | Yes |
| `PULSE_AMS_<NAME>_TOKEN` (per-source) | Yes |
| **`PULSE_LICENSE_KEY`** | **No** — read via plain `os.Getenv`; no `_FILE` variant is honoured |

For `PULSE_LICENSE_KEY`, set the value directly in `deploy/.env` or pass it as a
plain environment variable. Docker secrets via a `_FILE` path will **not** work for
this variable.

### Rotating the AMS token

When an AMS REST token is rotated:

1. Generate the new token in the AMS admin panel.
2. Update `PULSE_AMS_AUTH_TOKEN` in `deploy/.env`.
3. Restart the pulse service to pick up the new value:

```sh
docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
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

To expose the AMS web console publicly, host nginx proxies `ams.<your-domain>`
to the AMS host (see `deploy/nginx/ams.beyondkaira.com.conf` — upstream
`127.0.0.1:5080` for an AMS on the same host; edit the `.conf` if yours differs).

**4b. Start with the real-ams override**

```sh
docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
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

> **Automated backups are now handled by `deploy/docker-compose.backup.yml`.**
> Add that overlay to your compose command (see below). Manual steps are still
> documented here for reference and for environments without the backup sidecar.
> Full operator doc: `deploy/runbooks/backup-restore.md`.

### Enabling the backup overlay

Add `-f deploy/docker-compose.backup.yml` to your compose command:

```sh
docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
  up -d
```

This adds a `backup` sidecar that runs every 24 h and keeps the 7 most-recent
artifacts for each store. Artifacts land in the `pulse-backups` named volume.

### ClickHouse backups

The backup overlay configures a ClickHouse disk named `backups` at `/backups/`
via `deploy/config/clickhouse-backups.xml`. The `BACKUP DATABASE` SQL command
requires this disk to be configured — it does NOT work without the overlay.

**Manual one-shot backup** (overlay must be active):

```sh
docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.backup.yml \
  exec backup /scripts/pulse-backup.sh once
```

**Manual restore** (see `deploy/runbooks/backup-restore.md` for full steps):

```sh
docker compose -p pulse-prod exec backup \
  clickhouse-client \
    --host clickhouse \
    --user "${CLICKHOUSE_USER}" \
    --password "${CLICKHOUSE_PASSWORD}" \
    --query "RESTORE DATABASE pulse FROM Disk('backups', 'ch/pulse-YYYYMMDD-HHMMSS.zip')"
```

**Data-retention TTL** (configured in `contracts/db/clickhouse/`):

| Table type | TTL env var | Default |
|---|---|---|
| Raw events (`viewer_sessions`, etc.) | `PULSE_RETENTION_DAYS` | 90 days |
| Rollup tables | `PULSE_ROLLUP_TTL_DAYS` | 395 days (~13 months) |

Adjust these in `deploy/.env` and re-run `pulse migrate` to apply new TTL values.

### SQLite meta store backups

The meta store lives at `/var/lib/pulse/pulse_meta.db` inside the `pulse-data`
volume. It holds alert rules, API tokens, probe config, and user data — not
time-series events. It runs in WAL journal mode.

The backup sidecar copies `pulse_meta.db` + `.db-wal` + `.db-shm` to the
`pulse-backups` volume each cycle (no `sqlite3` binary is needed).

**Manual restore** (stop pulse first to prevent write conflicts):

```sh
docker compose -p pulse-prod stop pulse

# IMPORTANT: clear the live WAL and SHM *before* overwriting the db file.
# A crashed pulse may have left post-backup WAL frames on disk; if not removed
# first, SQLite replays them onto the restored db and produces a state beyond
# the backup point. (rm -f is idempotent — safe if files are absent.)
docker run --rm \
  -v pulse-prod_pulse-backups:/src:ro \
  -v pulse-prod_pulse-data:/dst \
  busybox sh -c "
    rm -f /dst/pulse_meta.db-wal /dst/pulse_meta.db-shm
    cp /src/meta/pulse_meta-YYYYMMDD-HHMMSS.db /dst/pulse_meta.db
    [ -f /src/meta/pulse_meta-YYYYMMDD-HHMMSS.db-wal ] && \
      cp /src/meta/pulse_meta-YYYYMMDD-HHMMSS.db-wal /dst/pulse_meta.db-wal || true
    echo done
  "

docker compose -p pulse-prod start pulse
```

**Recommended schedule:** daily (handled automatically by the backup overlay).
For off-host storage (S3, NFS, rsync) see `deploy/runbooks/backup-restore.md §S3`.

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
      insecure_skip_verify: false  # host nginx serves a valid certbot cert
```

**Key metrics to alert on** (the 7 metrics registered by `/metrics` — see `deploy/runbooks/monitoring.md` for full details):

| Metric | Labels | Description |
|---|---|---|
| `pulse_live_viewers` | — | Current live viewer count |
| `pulse_live_streams` | — | Current active stream count |
| `pulse_live_publishers` | — | Current publishing stream count |
| `pulse_ingest_bitrate_kbps` | — | Aggregate ingest bitrate (kbps) |
| `pulse_node_cpu_pct` | `node` | Node CPU utilization percent |
| `pulse_node_mem_pct` | `node` | Node memory utilization percent |
| `pulse_alerts_firing` | — | Total firing alert count |

---

## Quick reference — full production startup

```sh
# From the repo root, after filling in deploy/.env:

docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
  up -d

# Watch logs:
docker compose -p pulse-prod logs -f pulse

# Verify:
curl -m 10 https://pulse.example.com/healthz
```

The three-file compose ordering is canonical (all files under `deploy/`):
1. `docker-compose.prod.yml` — consolidated prod stack (pulse + ClickHouse with auth,
   container hardening, resource limits, loopback-only `127.0.0.1` publishes for host nginx)
2. `docker-compose.real-ams.yml` — disables mock-ams, wires real AMS credentials
3. `docker-compose.backup.yml` — backup sidecar (24 h cycle, 7-copy retention)

TLS is terminated by host nginx (vhosts in `deploy/nginx/`, certbot-managed cert) —
nothing in the compose stack binds a public port.

**Omitting `-f deploy/docker-compose.backup.yml` on `up -d` silently removes the
backup sidecar.**

For upgrades — stamped-build procedure (D-058): build with explicit ARGs first,
then `up -d` WITHOUT `--build`. Mixing `--build` into `up -d` rebuilds in-place and
loses the `VERSION`/`COMMIT`/`BUILD_DATE` stamps.

```sh
# 1. Build the pulse image with explicit version stamps:
docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
  build \
  --build-arg VERSION=$(git describe --tags --always) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  pulse

# 2. Start WITHOUT --build — uses the pre-built stamped image:
docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
  up -d

# 3. Run migrations after image update:
docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
  run --rm pulse-migrate
```
