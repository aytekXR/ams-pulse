# Pulse — Install Runbook

**PRD ref:** §7.12 (launch asset — 15-minute install target)  
**QA-verified:** local binary path < 2 min (Wave-1 gate); wave-2 build verified 2026-06-14

---

## Overview

Pulse installs beside your Ant Media Server (AMS) and never modifies it.
Two components are required: the **Pulse binary** (collector + API + UI) and **ClickHouse**
(event store). Configuration is via environment variables; AMS credentials never go
in a config file or image.

Three install paths are available:

| Path | Status | Recommended for |
|---|---|---|
| **Path A: Docker Compose** | Authored; unexecuted per D-002 | Single-server production |
| **Path B: Local binary** | QA-verified (< 2 min) | Dev, bare-metal, ClickHouse managed separately |
| **Path C: Helm** | Authored and lint-verified; cluster validation deferred D-002 | Kubernetes / clustered AMS |

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

All wave-1 and wave-2 variables are listed below. Omit any wave-2 variable to get the
noted default; the binary runs correctly without it.

**Wave-1 variables (collector + API core):**

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
| `PULSE_WEBHOOK_ADDR` | — | Address for the AMS webhook receiver (optional) |
| `PULSE_WEBHOOK_SECRET` | — | HMAC shared secret for webhook validation |
| `PULSE_LICENSE_KEY` | — | License key (empty = Free tier) |
| `PULSE_LICENSE_FILE` | — | Path to offline license file (air-gapped Enterprise) |
| `PULSE_RETENTION_DAYS` | `90` | Raw event retention in ClickHouse (days) |
| `PULSE_ROLLUP_TTL_DAYS` | `395` | Rollup table TTL in ClickHouse (days; 395 ≈ 13 months) |
| `PULSE_MIGRATIONS_DIR` | auto from source tree | Override path to ClickHouse migration SQL files |
| `PULSE_META_DDL_PATH` | embedded in binary | Optional override: path to a custom meta DDL SQL file |

**Wave-2 variables (beacon ingest listener, geo, Kafka, metrics, reports, S3):**

| Variable | Default | Description |
|---|---|---|
| `PULSE_INGEST_LISTEN_ADDR` | — (main listener) | Dedicated beacon ingest address, e.g. `:8091`. Set to expose beacon on a separate port for DMZ routing. |
| `PULSE_METRICS_TOKEN` | — (401 without) | Prometheus scrape token. Set to enable `/metrics` with token auth. See [Prometheus guide](../guides/prometheus.md). |
| `PULSE_ANONYMIZE_IP` | `false` | Set `true` to zero last IPv4 octet / last 80 IPv6 bits before geo lookup and ClickHouse storage (GDPR/KVKK posture). |
| `PULSE_GEO_MMDB_PATH` | — (no-op) | Path to a MaxMind GeoLite2 `.mmdb` file for geo enrichment. Absent = no-op, one WARN logged. Register at maxmind.com for the free GeoLite2 download (D-007.4). |
| `PULSE_KAFKA_BROKERS` | — (disabled) | Comma-separated Kafka broker addresses, e.g. `kafka1:9092,kafka2:9092`. Empty = Kafka source disabled. |
| `PULSE_KAFKA_GROUP_ID` | `pulse-collector` | Kafka consumer group ID. |
| `PULSE_SESSION_IDLE_TIMEOUT` | `5m` | Viewer session idle close timeout (Go duration, e.g. `3m`, `10m`). |
| `PULSE_CLUSTER_DISCOVERY_INTERVAL` | `30s` | How often to poll AMS for cluster nodes. New node visible within 1 poll cycle (≤ 2 min budget). |
| `PULSE_INGEST_TARGET_BITRATE_KBPS` | `2000` | Expected healthy ingest bitrate for the health score formula. |
| `PULSE_INGEST_TARGET_FPS` | `30` | Expected healthy ingest frame rate for the health score formula. |
| `PULSE_REPORTS_DIR` | `pulse-reports` | Local directory for generated CSV/PDF report files. |
| `PULSE_S3_ENDPOINT` | — (AWS) | S3-compatible endpoint URL (for MinIO, DigitalOcean Spaces, etc.). Empty = AWS. |
| `PULSE_S3_BUCKET` | — | S3 bucket name for report uploads. |
| `PULSE_S3_PREFIX` | `reports/` | S3 key prefix for uploaded reports. |
| `PULSE_S3_REGION` | `us-east-1` | S3 region. |
| `PULSE_S3_ACCESS_KEY_ENV` | `PULSE_S3_ACCESS_KEY_ID` | Name of the env var holding the S3 access key ID (indirect reference — never store the key in this var). |
| `PULSE_S3_SECRET_KEY_ENV` | `PULSE_S3_SECRET_ACCESS_KEY` | Name of the env var holding the S3 secret access key. |

### YAML config file

> **Note:** the shipped binary reads configuration from **environment variables only** —
> `pulse serve|migrate|diag` all use the env loader; a YAML config file is **not consumed**
> (a `--config`/`pulse.yaml` parser exists in `server/internal/config` but is not wired into
> the binary entry point, `server/cmd/pulse/main.go` HOOK(BE-02)). If you create `pulse.yaml`,
> it is silently ignored. The schema below documents `deploy/config/pulse.example.yaml` as a
> reference for the env-var equivalents above.

```yaml
server:
  listen: ":8090"
  ingest_listen: ":8091"      # beacon ingest (separate DMZ port)
  base_url: https://pulse.example.com   # used in alert deep-links
  metrics_token: ""           # Prometheus scrape token (see guides/prometheus.md)

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
    rollup_months: 13     # 395 days rollup TTL

beacon:
  sample_rate: 1.0
  # anonymize_ip: true   # GDPR/KVKK posture — zeros last IPv4 octet / last 80 IPv6 bits

geo:
  # mmdb_path: /data/GeoLite2-City.mmdb   # user-supplied; register at maxmind.com

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

### Diagnostic commands

```sh
# Print resolved config (secrets redacted), CH connectivity, meta store status:
/tmp/pulse diag

# Check billing reconciliation (Wave 2) — requires live ClickHouse:
/tmp/pulse diag --reconcile
# Output: drift%, tolerance verdict (exits non-zero if > 1%)
```

`pulse diag` is the first tool to run when diagnosing connectivity issues.
`pulse diag --reconcile` compares rollup-derived viewer-minutes against raw
`viewer_sessions` and reports the drift percentage (budget: ≤ 1%).

---

## Upgrading

1. Stop Pulse (`docker compose stop pulse` or kill the process).
2. Replace the binary or update the Docker image tag.
3. Run `pulse migrate` to apply any new ClickHouse DDL.
4. Start Pulse. Meta migrations run automatically on startup.

The meta store schema is backwards-compatible within a major version.
ClickHouse DDL migrations are append-only (no destructive changes).

---

## Path C: Helm (Kubernetes)

> **EXPERIMENTAL — do not use in production yet.**
> The chart has not been deployed to a real cluster (D-002 waiver). Template-render
> and lint pass locally (`helm lint`, `helm template` golden-file tests).
> Validate on a clean cluster before production use.
>
> **S6 parity batch — shipped in this session (chart now has):**
> - ClickHouse auth via `clickhouse.auth.existingSecret` — wire `CLICKHOUSE_USER` /
>   `CLICKHOUSE_PASSWORD` / `PULSE_CLICKHOUSE_DSN` through a K8s Secret (parity with
>   `docker-compose.hardened.yml`). Empty `existingSecret` = unauthenticated default
>   user (dev/test only; must be protected by NetworkPolicy).
> - Webhook Service (`webhookService.enabled`) + Ingress (`ingressWebhook.enabled`)
>   for AMS webhook callbacks on port 8092.
>   Routes: `/webhook/ams` and `/webhook/ams/{source_name}` (B7 per-source webhooks).
>   Per-source secret rotation requires a pod restart (no live reload).
> - Backup CronJob (`backup.enabled=false` by default, same opt-in posture as compose).
>   Mirrors `docker-compose.backup.yml` / `pulse-backup.sh`: ClickHouse BACKUP SQL +
>   SQLite meta store file copy; retention=7; digest-pinned CH image.
> - `PULSE_SECRET_KEY` wired with `optional: false` — pod fails to schedule (not
>   crash at runtime) if the Secret key is absent.
> - Digest pinning via `pulse.image.digest` (recommended for production).
> - `NOTES.txt` post-install smoke test in `helm install` output.
>
> **Remaining gap (chart still EXPERIMENTAL):**
> - `helm install` / `helm upgrade` not run against a real cluster (D-002).
>   QA-01 must validate on a clean cluster before removing the EXPERIMENTAL marker.
> - S3 push in the backup CronJob requires a custom sidecar image (aws-cli not in the
>   ClickHouse base image); documented in `backup.extraEnv` and README.
>
> **Canonical image:** `ghcr.io/aytekxr/ams-pulse` (cosign-signed from `v0.1.0` onward).
> Earlier tags do not exist on this registry. Pin to digest in production via
> `pulse.image.digest`.
>
> **Cluster-unvalidated (D-002):** `helm install` / `helm upgrade` have not been run
> against a real cluster. `helm lint` passes and all three template variants render
> without error. Validate on a clean cluster before production use.
> See `deploy/helm/pulse/README.md` for the full values table and HA configuration.

### Prerequisites

- Kubernetes 1.25+, Helm 3.12+
- A Kubernetes Secret with sensitive values (create before install)

### Steps

**1. Create the secrets**

```sh
kubectl create secret generic pulse-secrets \
  --from-literal=PULSE_AMS_AUTH_TOKEN=<your-ams-token> \
  --from-literal=PULSE_SECRET_KEY=$(openssl rand -hex 32) \
  --from-literal=PULSE_METRICS_TOKEN=<prometheus-scrape-token>
```

Never store sensitive values in `values.yaml`. The chart reads all secrets from
the Secret named in `pulse.secretRef.name`.

**Note (GAP-206-02):** If you enable the bundled Postgres StatefulSet
(`postgres.enabled=true`), you must also create a `pulse-postgres-secret` Secret
with a `POSTGRES_PASSWORD` key before install. The chart does not auto-generate
passwords.

**2. Install the chart**

```sh
helm install pulse ./deploy/helm/pulse \
  --set pulse.ams.url=http://your-ams:5080 \
  --set pulse.ams.nodeId=node-01 \
  --set pulse.secretRef.name=pulse-secrets
```

Bundled ClickHouse StatefulSet starts automatically. The chart uses a minimal
low-footprint ClickHouse config (512 MB memory cap; 2-vCPU tier default).

**3. Apply migrations**

```sh
kubectl exec deploy/pulse -- pulse migrate
```

**4. Access the UI**

```sh
kubectl port-forward svc/pulse 8090:8090
# Open http://localhost:8090
```

For production access configure an Ingress:
```yaml
pulse:
  secretRef:
    name: pulse-secrets
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: pulse.example.com
      paths:
        - path: /
          pathType: Prefix
```

**Beacon ingest (internet-facing):** Configure a separate Ingress or LoadBalancer
for the ingest port. See `deploy/helm/pulse/README.md §ingressIngest`.

**5. Enable geo enrichment (optional)**

Pulse uses a user-supplied MaxMind GeoLite2 database for country/city-level geo
enrichment (D-007.4). MaxMind requires a free registration to download GeoLite2
files.

1. Register at [https://www.maxmind.com/en/geolite2/signup](https://www.maxmind.com/en/geolite2/signup)
2. Download `GeoLite2-City.mmdb`
3. Mount it in your deployment and set the path:

```yaml
pulse:
  extraEnv:
    - name: PULSE_GEO_MMDB_PATH
      value: /data/GeoLite2-City.mmdb
  extraVolumeMounts:
    - name: mmdb
      mountPath: /data
  extraVolumes:
    - name: mmdb
      configMap:           # or a PVC / hostPath
        name: mmdb-data
```

Or set `mmdb.enabled=true` in values.yaml for the built-in mmdb volume mount stub.

When `PULSE_GEO_MMDB_PATH` is absent or the file cannot be read, Pulse logs one
WARN and falls back to no-op geo enrichment — existing collection continues normally.

**6. Upgrade**

```sh
helm upgrade pulse ./deploy/helm/pulse -f my-values.yaml
kubectl exec deploy/pulse -- pulse migrate  # apply new DDL if any
```

---

## Free tier limits

Pulse starts in Free tier when no license key is configured:

| Limit | Free | Pro | Business | Enterprise |
|---|---|---|---|---|
| AMS source nodes | 1 | 10 | 5 | Unlimited |
| Notification channels | Email only | Email, Slack, Telegram | Email, Slack, Telegram, PagerDuty, Webhook | All |
| Data retention | 7 days | 90 days | 13 months | Unlimited |
| Data API, CSV export | No | Yes | Yes | Yes |
| Usage reports + scheduled exports | No | No | Yes | Yes |
| White-label PDF | No | No | No | Yes |
| Beacon ingest (QoE) | No (403 LICENSE_REQUIRED) | Yes | Yes | Yes |
| Prometheus `/metrics` endpoint | No (403 LICENSE_REQUIRED) | No (403 LICENSE_REQUIRED) | Yes | Yes |

Upgrading: set `PULSE_LICENSE_KEY` in your environment or YAML config.
The license check **fails open for reads** — you can always read already-collected
data even if the key is invalid. Tier-gated features fail closed (return 403).
