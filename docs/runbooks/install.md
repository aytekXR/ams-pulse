# Pulse — Install Runbook

**PRD ref:** §7.12 (launch asset — 15-minute install target)  
**QA-verified:** local binary path < 2 min (Wave-1 gate); wave-2 build verified 2026-06-14

---

## Overview

Pulse installs beside your Ant Media Server (AMS) and never modifies it.
Two components are required: the **Pulse binary** (collector + API + UI) and **ClickHouse**
(event store). Configuration is via environment variables; AMS credentials never go
in a config file or image.

Four install paths are available:

| Path | Status | Recommended for |
|---|---|---|
| **Path A0: One-command quickstart** | Supported — live clean-install verification scheduled (D-089 V2) | Evaluators, first install, single-server |
| **Path A: Docker Compose** | Supported production path — runs the live production deployment; CI `docker-build` is a required merge context and staging smokes exercise it every deploy session | Single-server production |
| **Path B: Local binary** | QA-verified (< 2 min) | Dev, bare-metal, ClickHouse managed separately |
| **Path C: Helm** | Authored, lint- and golden-file-verified in CI; marked experimental until a cluster install is validated | Kubernetes / clustered AMS |

**Primary install path — Docker Compose:**  
One command brings up the stack. This is the supported production path (PRD §7.10) and
what the live production deployment runs (consolidated prod + real-AMS + backup
overlays, behind host-nginx TLS; see `productionize.md`). Released images: `ghcr.io/aytekxr/ams-pulse`
(cosign-signed, multi-arch, from `v0.1.0` onward). The historical D-002
"Docker unavailable on the build machine" waiver is retired.

---

## Path A0 — One-command quickstart

The quickstart path uses a pre-built released image (`ghcr.io/aytekxr/ams-pulse`) with
ClickHouse migration SQL **baked in** — no repo clone is needed. A single script handles
Docker preflight, credential collection, `.env` writing, stack start, healthz polling,
and bootstrap-token extraction.

> **GHCR visibility (pending):** `ghcr.io/aytekxr/ams-pulse` is pending public visibility.
> Until the operator flips the package to Public you must authenticate first:
> `docker login ghcr.io` (GitHub PAT with `read:packages` scope).
> This note will be removed once the package is public.

> **Image tag format:** Pulse image tags have **no `v` prefix**. The git release tag
> `v0.4.0` is published as image tag `0.4.0` (not `v0.4.0`). Always omit the `v`
> when specifying an image tag (e.g. `ghcr.io/aytekxr/ams-pulse:0.4.0`).

### Prerequisites

- Docker Engine 24+ and Docker Compose v2 (`docker compose version` must succeed)
- AMS host accessible from this host on port 5080

### Option 1 — curl|bash (one command)

```sh
curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/install.sh \
  | bash -s -- \
      --ams-url http://YOUR_AMS_HOST:5080 \
      --email your-ams-admin@example.com \
      --password your-ams-password
```

The script prompts interactively for any missing required flags when a TTY is attached.
Append `--license-key <key>` to activate a Pro/Business/Enterprise license on first boot.

### Option 2 — manual (3 commands)

```sh
mkdir quickstart && cd quickstart

# Download the compose file and env template
curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/docker-compose.quickstart.yml \
  -o docker-compose.quickstart.yml
curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/.env.example \
  -o .env && chmod 600 .env

# Edit .env: fill in PULSE_AMS_URL, PULSE_AMS_LOGIN_EMAIL, PULSE_AMS_LOGIN_PASSWORD,
# generate PULSE_SECRET_KEY with `openssl rand -hex 32`, and optionally PULSE_LICENSE_KEY.
${EDITOR:-nano} .env

docker compose -f docker-compose.quickstart.yml --env-file .env up -d
```

After the stack starts, extract the bootstrap admin token (first run only):

```sh
docker compose -f docker-compose.quickstart.yml --env-file .env logs pulse \
  | grep -oE 'plt_[a-f0-9]+' | head -1
```

Open `http://localhost:8090` and enter the token on the **Pulse login screen** (the
first screen you see — not a step inside the onboarding wizard). The onboarding wizard
starts automatically after you sign in, when no AMS sources are configured yet.

---

## Path A: Docker Compose (supported production path)

> **Note:** This is the supported production path — it is what the live production
> deployment runs. A full clean-install verification of the step-by-step below (fresh
> machine, released image, real AMS) is scheduled as a SESSION-11 work order (D-069);
> any step that diverges will be corrected there.

### Prerequisites

- Docker Engine 24+ and Docker Compose v2
- AMS host accessible from the Pulse host on port 5080 (REST API)
- Outbound internet access to pull images (or pre-pull and side-load for air-gapped)
- 2 vCPU, 2 GB RAM minimum (4 GB recommended for > 100 concurrent streams)

### Steps

**1. Clone / download Pulse**

```sh
git clone https://github.com/aytekXR/ams-pulse.git
cd ams-pulse
```

**2. Review the configuration reference (optional)**

`deploy/config/pulse.example.yaml` is a fully annotated reference for every
configuration option and its environment-variable equivalent.

> **Important:** `deploy/config/pulse.example.yaml` is reference documentation only —
> it is **not consumed at runtime**. The shipped binary reads configuration from
> **environment variables only**. A `--config` YAML parser exists in
> `server/internal/config` but is not yet wired into the binary entry point
> (`HOOK(BE-02)` in `server/cmd/pulse/main.go`). If you create or edit `pulse.yaml`,
> the binary silently ignores it. Set your AMS URL and credentials in the environment
> (step 3 below).

**3. Set required environment variables**

```sh
# AMS REST base URL — required (the default http://localhost:5080 only works if
# AMS runs on the same host as Pulse):
export PULSE_AMS_URL=http://YOUR_AMS_HOST:5080

# AMS credentials for cookie-session auth (AMS Enterprise Edition uses
# email + password login; these are the operative credentials):
export PULSE_AMS_LOGIN_EMAIL=your-ams-admin@example.com
export PULSE_AMS_LOGIN_PASSWORD=your-ams-password

# AMS bearer token — optional; only needed if your AMS uses token-based auth
# instead of cookie-session. Leave unset for AMS Enterprise Edition:
# export PULSE_AMS_AUTH_TOKEN=

# 32-byte hex key for encrypting secrets at rest (generate once, keep safe):
export PULSE_SECRET_KEY=$(openssl rand -hex 32)

# License key — optional; empty = Free tier (1 node, 7-day retention):
# export PULSE_LICENSE_KEY=

# Official Pulse license verification key — do not change unless self-signing:
export PULSE_LICENSE_PUBKEY=6403d7b49951d0220c7219e491b6525971edf10f0e64616b17023eab002ab4ba
```

> AMS Enterprise Edition authenticates via cookie-session (email + password login).
> `PULSE_AMS_LOGIN_EMAIL` and `PULSE_AMS_LOGIN_PASSWORD` are the operative credentials.
> `PULSE_AMS_AUTH_TOKEN` is optional and only needed when bearer-token auth is used
> instead of cookie-session.

**4. Start the stack**

```sh
make up
# or: cd deploy && docker compose up -d
```

> **Warning — `docker-compose.override.yml` auto-loads from `deploy/`:** Docker Compose
> v2 automatically merges `docker-compose.override.yml` when you run `docker compose`
> from the `deploy/` directory without explicit `-f` flags. This dev/CI overlay binds
> `0.0.0.0:80:8090` on the host. If another service (Nginx, Caddy, an existing Pulse
> stack, etc.) already holds port 80, `compose up` will fail with a port conflict.
>
> To suppress the override and avoid the port 80 binding, pass compose files explicitly
> (run from the repo root):
>
> ```sh
> docker compose \
>   -f deploy/docker-compose.yml \
>   -f deploy/docker-compose.real-ams.yml \
>   up -d
> ```
>
> When using explicit `-f` flags without the override, the one-shot `pulse-migrate`
> container is not included. Apply schema migrations before the first start.
> Three gotchas (all verified empirically, D-072 release test):
> the image `ENTRYPOINT` is `pulse serve`, so `compose run` must override the
> entrypoint (`run --rm pulse pulse migrate` would execute `pulse serve pulse
> migrate` and fail); `pulse migrate` validates `PULSE_SECRET_KEY` at startup;
> and the ClickHouse migration SQL files are **not** baked into the runtime
> image — mount the repo's `contracts/` directory and point
> `PULSE_MIGRATIONS_DIR` at it:
>
> ```sh
> # from the repo root; PULSE_SECRET_KEY exported in step 3
> docker compose \
>   -f deploy/docker-compose.yml \
>   -f deploy/docker-compose.real-ams.yml \
>   run --rm --entrypoint pulse \
>   -e PULSE_SECRET_KEY \
>   -e PULSE_MIGRATIONS_DIR=/contracts/db/clickhouse \
>   -v "$(pwd)/contracts:/contracts:ro" \
>   pulse migrate
> docker compose \
>   -f deploy/docker-compose.yml \
>   -f deploy/docker-compose.real-ams.yml \
>   up -d
> ```

> **Note — base compose builds from source:** The base `docker-compose.yml` has no
> `image:` key for the Pulse service; it builds the binary from the local source tree
> on every `compose up`. To use a pre-built released image instead, pull it first and
> create a small image-pin override file:
>
> > **GHCR visibility (pending):** `ghcr.io/aytekxr/ams-pulse` is pending public visibility.
> > Until the package is made public you must authenticate first:
> > `docker login ghcr.io` (GitHub PAT with `read:packages` scope).
>
> ```sh
> # Pull the released image. ⚠ Image tags have NO `v` prefix: the git tag
> # `v0.2.0` publishes the image tag `0.2.0` (also `0.2`, `0`, `latest`).
> docker pull ghcr.io/aytekxr/ams-pulse:0.2.0
> ```
>
> Create `deploy/docker-compose.image-pin.yml` (do not commit this file).
> This full example is the one that passed the clean-install release test
> (D-072, ~3 min to verified-healthy): when running with explicit `-f` flags
> the override's conveniences are absent, so the pin file must also publish
> the API port, define the one-shot `pulse-migrate` service completely (the
> image `ENTRYPOINT` is `pulse serve` and the migration SQL is not baked into
> the runtime image), pass `PULSE_SECRET_KEY`, and keep ClickHouse's
> permissive default user:
> ```yaml
> services:
>   pulse:
>     image: ghcr.io/aytekxr/ams-pulse:0.2.0
>     ports:
>       - "127.0.0.1:8090:8090"   # loopback only; front with a reverse proxy for remote access
>     environment:
>       PULSE_SECRET_KEY: "${PULSE_SECRET_KEY}"
>     depends_on:
>       pulse-migrate:
>         condition: service_completed_successfully
>
>   pulse-migrate:
>     image: ghcr.io/aytekxr/ams-pulse:0.2.0
>     entrypoint: ["pulse", "migrate"]   # image ENTRYPOINT is `pulse serve`
>     restart: "no"
>     depends_on:
>       clickhouse:
>         condition: service_healthy
>     environment:
>       PULSE_CLICKHOUSE_DSN: "clickhouse://clickhouse:9000/pulse"
>       PULSE_CLICKHOUSE_DATABASE: "pulse"
>       PULSE_MIGRATIONS_DIR: "/contracts/db/clickhouse"   # SQL not in the runtime image
>       PULSE_META_DSN: "/var/lib/pulse/pulse_meta.db"
>       PULSE_SECRET_KEY: "${PULSE_SECRET_KEY}"            # migrate validates it too
>     volumes:
>       - pulse-data:/var/lib/pulse
>       - ../contracts:/contracts:ro
>
>   clickhouse:
>     environment:
>       # ClickHouse ≥24.8 disables the default user for network access unless
>       # credentials are set; the dev override sets this — explicit -f paths must too.
>       CLICKHOUSE_SKIP_USER_SETUP: "1"
> ```
>
> Then start the stack **without** `--build`:
> ```sh
> docker compose \
>   -f deploy/docker-compose.yml \
>   -f deploy/docker-compose.real-ams.yml \
>   -f deploy/docker-compose.image-pin.yml \
>   up -d
> ```

**Schema migrations:** When using the default `make up` / `cd deploy && docker compose
up -d` path, the stack includes a one-shot **`pulse-migrate`** container (from
`docker-compose.override.yml`) that automatically applies ClickHouse and meta-store
schema before Pulse starts. No separate migration step is needed for the default path.

ClickHouse takes ~10 s to become healthy (the healthcheck retries up to 12 times
at 10 s intervals). Pulse will wait for it via the `depends_on: clickhouse: condition: service_healthy` in the Compose file.

**5. Open the UI and run first-run setup**

The entry point depends on which compose path you ran:

| Path | URL |
|---|---|
| **Plain path** (`make up` / `cd deploy && docker compose up -d`) | `http://your-server` (port 80, published on all interfaces by the dev override) or `http://localhost:8090` (loopback-only debug binding) |
| **Production path** (README Quick Start — `docker-compose.prod.yml` + real-ams overlay) | **`http://127.0.0.1:8090`** (loopback only). The stack terminates no TLS itself; put a host reverse proxy in front for public exposure — the reference edge is host nginx with a certbot cert (see `productionize.md`). |

> With explicit `-f` flags (no override) the base compose only `expose`s 8090 inside the
> Docker network — nothing is reachable from the host unless you added a `ports:` binding
> (the image-pin example above binds `127.0.0.1:8090`) or fronted the stack with a
> reverse proxy (see `productionize.md`).

On first run, the bootstrap token is printed to the Pulse container logs:

```sh
# Run from the deploy/ directory when using the default compose path:
docker compose logs pulse | grep "FIRST RUN"
# If you ran with a custom project name (-p flag) or from outside deploy/,
# pass the same -p flag and/or -f flags used in step 4:
# docker compose -p <project-name> logs pulse | grep "FIRST RUN"
# pulse: FIRST RUN — generated admin token: plt_<hex>
#        Save this token; it will not be shown again.
```

> **WARNING — save this token before closing the terminal.** The FIRST RUN bootstrap
> token is printed exactly once and is never stored in plaintext anywhere. There is no
> CLI command to recover or regenerate it (`pulse` subcommands are `serve`, `migrate`,
> `version`, `diag`, `help` — there is no `token` or `reset-admin` command). If the
> token is lost, the only recovery is to **delete the entire meta database** (Docker
> default: `/var/lib/pulse/pulse_meta.db`; bare-metal default: `pulse.db` beside the
> binary) and restart Pulse. Deleting the meta database wipes every configured source,
> alert rule, API token, and system setting — full reconfiguration is required.
> See [Troubleshooting](#troubleshooting) for the exact recovery steps.

Copy the token. Open the UI and enter it on the **Pulse login screen** — the first
screen you see when you navigate to the UI, before the onboarding wizard appears.

**6. Complete the onboarding wizard**

Open the UI (see URL table above). You will be greeted by the **Pulse login screen**
(not the wizard). Enter the admin token from step 5 and click **Sign in**.

The wizard starts automatically after login when no AMS sources are configured:
1. **Welcome** — click "Get started".
2. **Add source** — enter the AMS REST URL and source name.
3. **Verify** — click "Test connection"; Pulse calls `POST /admin/sources/{id}/test`.
4. **Done** — click "Go to live dashboard".

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

Navigate to `http://localhost:8090`. You will see the **Pulse login screen** — enter
the admin token printed in step 4 and click **Sign in**. The onboarding wizard
starts automatically after login (same 4-step flow as Docker path step 6 above).

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

Before the wizard is reachable you must sign in on the **Pulse login screen**
(`web/src/components/AuthGate.tsx`). Open the UI, enter the admin token from the
container or process logs (see the relevant path's step 5/6 above), and click
**Sign in**. The login screen has a hint that reads "Generate a token in Settings →
API Tokens" — this is circular for a brand-new install; ignore it. The token comes
from the container logs as shown above.

The onboarding wizard opens automatically after login when no AMS sources are
configured. It is a 4-step flow in the UI.

**Step 1 — Welcome**  
A welcome card listing the setup steps. Click **"Get started"** to proceed.
There is no token input on this screen — the token was already accepted by the
login screen that preceded it.

**Step 2 — Add source**  
Fields:
- **Name** — label for this AMS instance (e.g., `production`, `staging`)
- **AMS REST URL** — the AMS HTTP(S) endpoint, e.g. `http://10.0.1.10:5080`
- **REST username** — AMS admin username (optional; default: `admin`)
- **Credential env var** — the name of the environment variable holding the AMS
  password; Pulse stores the variable name, not the secret itself
- **Log path (optional)** — full path to the AMS analytics log file for
  low-latency event capture, e.g. `/var/log/antmedia/ant-media-server-analytics.log`

**Step 3 — Verify**  
Click **"Test connection"**. Pulse calls `POST /admin/sources/{id}/test`
(`web/src/features/settings/OnboardingWizard.tsx` `handleTest()`; see also
`web/src/api/client.ts` `testSource`). A success response with `reachable: true`
shows the latency and AMS version; a failure shows the error reason.
A red error typically means the REST URL or credentials are wrong — see
[Troubleshooting](#troubleshooting) below.

**Step 4 — Done**  
Click **"Go to live dashboard"**. New streams appear within the poll interval
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
| `PULSE_AMS_LOGIN_EMAIL` | — | AMS admin email for cookie-session auth (AMS Enterprise Edition) |
| `PULSE_AMS_LOGIN_PASSWORD` | — | AMS admin password for cookie-session auth (AMS Enterprise Edition) |
| `PULSE_AMS_AUTH_TOKEN` | — | AMS bearer token for the REST API (optional for cookie-session setups) |
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
| `FIRST RUN` token lost | Token is printed once and never stored in plaintext; `pulse` has no token-recovery subcommand | **All configuration is destroyed in recovery.** Delete the meta database (`/var/lib/pulse/pulse_meta.db` in Docker; `pulse.db` beside the binary for bare-metal), then restart Pulse. A new FIRST RUN token is printed. Every source, alert rule, API token, and setting must be re-entered. See the WARNING block in step 5 above. |

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
