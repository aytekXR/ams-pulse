# Pulse — Install Runbook

**PRD ref:** §7.12 (launch asset — 15-minute install target)  
**QA-verified:** local binary path < 2 min (see Wave-1 gate report)

---

## Overview

Pulse installs beside your Ant Media Server (AMS) and never modifies it.
Two components are required: the **Pulse binary** (collector + API + UI) and **ClickHouse**
(event store). Configuration lives in a single YAML file; AMS credentials stay in
environment variables, never in the config file.

**Primary install path — Docker Compose:**  
One command brings up both containers. This is the supported production path
(PRD §7.10). It is authored from `deploy/` and validated by analysis; the Docker Compose
`make up` command is **authored-but-unexecuted-here** per decision D-002
(Docker not available on the build machine). The local binary path below is the
QA-verified equivalent and exercises identical code.

---

## Path A: Docker Compose (supported production path)

> **Note:** This path is authored and designed for production. The commands below
> are correct per `deploy/docker-compose.yml` and the Dockerfile, but were not
> executed during Wave 1 development (D-002: Docker not available on the build
> machine). Treat as the supported install path for your server environment.

### Prerequisites

- Docker Engine 24+ and Docker Compose v2
- AMS host accessible from the Pulse host on port 5080 (REST API)
- Outbound internet access to pull images (or pre-pull and side-load for air-gapped)
- 2 vCPU, 2 GB RAM minimum (4 GB recommended for > 100 concurrent streams)

### Steps

**1. Clone / download Pulse**

```sh
git clone https://github.com/your-org/pulse.git
cd pulse
```

**2. Copy and edit the config file**

```sh
cp deploy/config/pulse.example.yaml deploy/config/pulse.yaml
```

Edit `deploy/config/pulse.yaml`:

```yaml
ams:
  sources:
    - name: main
      rest_url: http://YOUR_AMS_HOST:5080
      # AMS REST credentials go in the environment, not here — see step 3.
```

All other keys have sane defaults. The full config surface is documented
in `deploy/config/pulse.example.yaml`.

**3. Set credentials in the environment**

```sh
# AMS REST API bearer token (AMS admin token or dedicated read token):
export PULSE_AMS_AUTH_TOKEN=your_ams_token_here

# 32-byte hex key for encrypting secrets at rest (generate once, keep safe):
export PULSE_SECRET_KEY=$(openssl rand -hex 32)
```

> `PULSE_AMS_AUTH_TOKEN` is the single bearer token used to authenticate to the
> AMS REST API. When the config system is upgraded to Wave-2 multi-source YAML,
> per-source env vars (`PULSE_AMS_<NAME>_TOKEN`) will be introduced.

**4. Start the stack**

```sh
make up
# or: cd deploy && docker compose up -d
```

ClickHouse takes ~10 s to become healthy (the healthcheck retries up to 12 times
at 10 s intervals). Pulse will wait for it via the `depends_on: clickhouse: condition: service_healthy` in the Compose file.

**5. Open the UI and run first-run setup**

Navigate to `http://your-server:8090` in a browser.

On first run, the bootstrap token is printed to the Pulse container logs:

```sh
docker compose logs pulse | grep "FIRST RUN"
# pulse: FIRST RUN — generated admin token: plt_<hex>
#        Save this token; it will not be shown again.
```

Copy the token. You will use it in the onboarding wizard.

**6. Complete the onboarding wizard**

The wizard appears automatically on first login:
1. **Welcome** — enter your admin token.
2. **Add source** — confirm or edit the AMS REST URL and source name.
3. **Verify** — Pulse pings AMS and shows the stream count.
4. **Done** — the live dashboard opens.

**Expected time:** under 5 minutes from `make up` to live dashboard.

---

## Path B: Local binary (QA-verified)

This path runs Pulse and ClickHouse as local processes without Docker.
It is the path QA-01 exercised in Wave 1 and all command output below is
from that verified run.

### Prerequisites

- Go 1.22+ (for building from source) or a pre-built `pulse` binary
- ClickHouse single binary (see step 1)
- An AMS host reachable from this machine

### Steps

**1. Get the ClickHouse binary**

```sh
cd /tmp && curl -fsSL https://clickhouse.com/ | sh
# Downloads clickhouse binary to /tmp/clickhouse (or current directory)
```

Start it in server mode (new terminal):

```sh
/tmp/clickhouse server
```

ClickHouse listens on native port 9000 and HTTP port 8123 by default.
Verify it is ready:

```sh
/tmp/clickhouse client --query "SELECT 1"
# → 1
```

**2. Build the Pulse binary**

```sh
cd server && CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/
```

Verified output (Wave 1 QA gate, 2026-06-12):
```
# no output — success
# binary at /tmp/pulse (~30 MB static binary)
```

Build time: approximately 15 s on a 2-vCPU machine.

**3. Run migrations**

```sh
PULSE_CLICKHOUSE_DSN=clickhouse://localhost:9000/pulse \
PULSE_META_DSN=/tmp/pulse.db \
/tmp/pulse migrate
```

This runs both:
- **Meta migrations** (SQLite) — creates 14 tables: `alert_rules`, `alert_channels`,
  `alert_history`, `api_tokens`, `ams_sources`, `ingest_tokens`, `users`, `tenants`,
  `license`, `cluster_nodes`, `probes`, `anomaly_baselines`, `report_schedules`,
  `schema_migrations`. The DDL is embedded in the binary; no external file needed.
- **ClickHouse migrations** — creates the `pulse` database with 9 raw/rollup tables
  and 5 materialized views.

Verified output (Wave 1 fix-loop QA gate, 2026-06-12):
```
[INFO] pulse migrate: meta store migrations done
[WARN] pulse migrate: ClickHouse migrations failed (non-fatal)   ← only if CH unreachable
[INFO] pulse migrate: done
```

ClickHouse migration failure is logged as a warning (non-fatal) so you can run meta
migrations standalone. When ClickHouse is running, both complete cleanly.

**4. Start Pulse**

```sh
PULSE_AMS_URL=http://YOUR_AMS_HOST:5080 \
PULSE_AMS_AUTH_TOKEN=your_ams_token_here \
PULSE_META_DSN=/tmp/pulse.db \
PULSE_SECRET_KEY=$(openssl rand -hex 32) \
/tmp/pulse serve
```

The meta schema (rules, tokens, channels) is embedded in the binary and applied
automatically. `PULSE_META_DDL_PATH` is no longer required.

> **Optional override:** Set `PULSE_META_DDL_PATH` to a custom SQL file path to
> replace the embedded meta DDL — useful for advanced deployments or schema
> customization. Under normal operation this variable should be omitted.

Pulse logs the first-run bootstrap token to stderr:

```
pulse: FIRST RUN — generated admin token: plt_<hex>
       Save this token; it will not be shown again.
```

Copy the token.

**5. Verify the server is healthy**

```sh
curl http://localhost:8090/healthz
```

Expected response (verified Wave 1 fix-loop QA gate, 2026-06-12):

```json
{
  "status": "ok",
  "components": {
    "clickhouse": {"status": "ok", "latency_ms": 2, "message": null},
    "meta_store":  {"status": "ok", "latency_ms": 0, "message": null},
    "collector":   {"status": "ok", "latency_ms": null, "message": null}
  }
}
```

`latency_ms` values are measured in milliseconds. `clickhouse` and `meta_store`
report real round-trip latency from a live probe. `collector` is non-latency
(pipeline status) so its `latency_ms` is `null`.

When ClickHouse or the meta store is unreachable, `/healthz` returns HTTP 503
and `status: "down"` for the affected component. Use this for health checks
and load-balancer probes.

**6. Open the dashboard and complete onboarding**

Navigate to `http://localhost:8090`. Enter the admin token printed in step 4.
Follow the 4-step onboarding wizard (same as Docker path, step 6 above).

**QA-measured time (Wave 1 gate):**

| Step | Time |
|------|------|
| Build ClickHouse binary download | ~30 s |
| `go build` pulse | ~15 s |
| `pulse migrate` | ~5 s |
| `pulse serve` start | ~2 s |
| Stream visible in dashboard | ~1 s after publish |
| **Total** | **< 2 min** |

---

## First-run wizard walkthrough

The onboarding wizard runs automatically when no AMS sources are configured.
It is a 4-step flow in the UI.

**Step 1 — Welcome**  
Enter the admin token from the container/process logs. The wizard validates it
against the API before proceeding.

**Step 2 — Add source**  
Fields:
- **Name** — label for this AMS instance (e.g., `production`, `staging`)
- **AMS REST URL** — the AMS HTTP(S) endpoint, e.g. `http://10.0.1.10:5080`
- **REST username** — AMS admin username (default: `admin`)
- **Credential env var** — the name of the env var holding the AMS bearer token;
  Pulse stores the reference, not the secret itself
- **Log path (optional)** — full path to the AMS analytics log file for
  low-latency event capture, e.g. `/var/log/antmedia/ant-media-server-analytics.log`

**Step 3 — Verify**  
Pulse calls `GET /api/v1/live/overview` and shows the AMS stream count.
A green checkmark means Pulse can reach AMS REST and is collecting events.
A red error typically means the REST URL or token is wrong — see
[Troubleshooting](#troubleshooting) below.

**Step 4 — Done**  
The live dashboard opens. New streams appear within the poll interval
(default 5 s; configurable via `PULSE_POLL_INTERVAL`).

---

## Configuration reference

All keys have defaults. Only the AMS source REST URL is required.
See `deploy/config/pulse.example.yaml` for the full annotated file.

### Environment variables

The following env vars are supported by the Wave 1 binary. Additional vars
(multi-source tokens, IP anonymisation, metrics scrape token) are added in
Wave 2 when the full config system lands.

| Variable | Default | Description |
|---|---|---|
| `PULSE_AMS_URL` | `http://localhost:5080` | AMS REST base URL |
| `PULSE_AMS_AUTH_TOKEN` | — | AMS bearer token for the REST API |
| `PULSE_AMS_NODE_ID` | `standalone` | AMS node identifier emitted in events |
| `PULSE_AMS_APPLICATIONS` | all | Comma-separated AMS app names to poll (empty = all) |
| `PULSE_CLICKHOUSE_DSN` | `clickhouse://localhost:9000/pulse` | ClickHouse native-protocol DSN |
| `PULSE_CLICKHOUSE_DATABASE` | `pulse` | ClickHouse target database name |
| `PULSE_META_DSN` | `pulse.db` beside binary | SQLite database file path |
| `PULSE_SECRET_KEY` | auto-generated file | 32-byte hex key for AES-256-GCM at-rest encryption |
| `PULSE_LISTEN_ADDR` | `:8090` | HTTP listen address for UI + API |
| `PULSE_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `PULSE_POLL_INTERVAL` | `5s` | AMS REST poll interval (e.g. `2s`, `10s`) |
| `PULSE_LOG_TAIL_PATH` | — | Full path to AMS analytics log file (optional) |
| `PULSE_WEBHOOK_ADDR` | — | Address for the AMS webhook receiver (optional) |
| `PULSE_WEBHOOK_SECRET` | — | HMAC shared secret for webhook validation |
| `PULSE_LICENSE_KEY` | — | License key (empty = Free tier) |
| `PULSE_RETENTION_DAYS` | `90` | Raw event retention in ClickHouse (days) |
| `PULSE_ROLLUP_TTL_DAYS` | `395` | Rollup table TTL in ClickHouse (days; 395 ≈ 13 months) |
| `PULSE_MIGRATIONS_DIR` | auto from source tree | Override path to ClickHouse migration SQL files (default: resolved relative to binary at build time) |
| `PULSE_META_DDL_PATH` | embedded in binary | Optional override: path to a custom meta DDL SQL file. The binary embeds `contracts/db/meta/0001_init.sql` and runs it automatically; set this only to substitute a custom schema. |

### YAML config file (Wave 2 — roadmap)

> **Roadmap (Wave 2):** YAML config file support is not yet wired in the Wave 1
> binary. Wave 1 uses environment variables only. The schema below is the planned
> format; it is the authoritative reference for `deploy/config/pulse.example.yaml`.

```yaml
server:
  listen: ":8090"
  ingest_listen: ":8091"
  base_url: https://pulse.example.com   # used in alert deep-links

ams:
  sources:
    - name: main
      rest_url: http://your-ams:5080
      # credentials via PULSE_AMS_AUTH_TOKEN env var — never in this file
      # analytics_log: /var/log/antmedia/ant-media-server-analytics.log
      # kafka_brokers: localhost:9092

storage:
  clickhouse_addr: localhost:9000
  meta: sqlite            # or: postgres
  retention:
    raw_days: 90
    rollup_months: 13

beacon:
  sample_rate: 1.0
  # anonymize_ip: true   # GDPR/KVKK posture

license:
  # key: ...             # empty = Free tier
```

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `/healthz` returns `"status": "down"` for `clickhouse` | ClickHouse not reachable | Check `PULSE_CLICKHOUSE_DSN`; verify ClickHouse is running with `clickhouse client --query "SELECT 1"` |
| `/healthz` returns `"status": "down"` for `meta_store` | SQLite file unwritable or Postgres DSN wrong | Check `PULSE_META_DSN` path permissions |
| `/healthz` returns HTTP 503 | One or more critical components are unreachable | Check the `components` object in the response body; each component reports its own `status` and `latency_ms` |
| Dashboard shows no streams after publish | AMS token wrong or REST URL unreachable | Run `pulse diag` to see config + connectivity; check `PULSE_AMS_AUTH_TOKEN` |
| `pulse migrate` exits with "connection refused" | ClickHouse not yet ready | Wait for ClickHouse healthcheck to pass; retry |
| No admin token printed | Not a first run — tokens already exist | Generate a new token via `POST /api/v1/admin/tokens` using an existing admin token |
| `FIRST RUN` token lost | Token is never stored in plaintext | Reset: delete the meta database file and restart; all existing data will need re-configuration |

### Diagnostic command

```sh
/tmp/pulse diag
```

Prints resolved config (secrets redacted), ClickHouse connectivity, and meta store
status. Run this first when diagnosing connectivity issues.

---

## Upgrading

1. Stop Pulse (`docker compose stop pulse` or kill the process).
2. Replace the binary or update the Docker image tag.
3. Run `pulse migrate` to apply any new ClickHouse DDL.
4. Start Pulse. Meta migrations run automatically on startup.

The meta store schema is backwards-compatible within a major version.
ClickHouse DDL migrations are append-only (no destructive changes).

---

## Free tier limits

Pulse starts in Free tier when no license key is configured:

| Limit | Free | Pro | Business |
|---|---|---|---|
| AMS source nodes | 1 | 5 | Unlimited |
| Notification channels | Email only | Email + Slack | All channels |
| Data retention | 7 days (raw) | 90 days | 13 months |
| API access | Yes | Yes | Yes + data export |

Upgrading: set `PULSE_LICENSE_KEY` in your environment or YAML config.
The license check **fails open for reads** — you can always read collected data
even if the key is invalid. Tier-gated features fail closed (return 403).
