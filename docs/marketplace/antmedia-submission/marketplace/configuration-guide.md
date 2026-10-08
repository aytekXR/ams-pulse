# Pulse — Configuration Guide

Pulse is configured with **environment variables** (`PULSE_*`). In the quickstart they live in
`quickstart/.env`. After editing, apply them with
`docker compose -f quickstart/docker-compose.quickstart.yml --env-file quickstart/.env up -d`.
A YAML file is **not** read by the binary.

Any variable holding a secret also accepts a `_FILE` form (e.g. `PULSE_AMS_LOGIN_PASSWORD_FILE=/run/secrets/ams_pw`)
for Docker/Kubernetes secrets. This guide covers the settings an AMS operator actually touches;
the complete reference (about 70 variables) is `docs/admin-guide.md` in the repository.

Defaults below were read from the code (`server/cmd/pulse/config.go`) on 2026-10-01.

## 1. Connecting to Ant Media Server

| Variable | Default | Purpose |
|---|---|---|
| `PULSE_AMS_URL` | `http://localhost:5080` | AMS REST base URL. Use `https://…:5443` when AMS is on another host. |
| `PULSE_AMS_LOGIN_EMAIL` / `PULSE_AMS_LOGIN_PASSWORD` | — | AMS 3.x console login (cookie session; re-login on expiry). Use a dedicated account. *Secret.* |
| `PULSE_AMS_AUTH_TOKEN` | — | Alternative for AMS with JWT REST security: a static bearer token. *Secret.* |
| `PULSE_AMS_APPLICATIONS` | all | Comma-separated application names to monitor (empty = every application). |
| `PULSE_AMS_NODE_ID` | `standalone` | Name shown for this AMS node in the UI and alerts. |
| `PULSE_POLL_INTERVAL` | `5s` | How often Pulse polls AMS. |
| `PULSE_CLUSTER_DISCOVERY_INTERVAL` | `30s` | How often Pulse asks AMS for cluster nodes. |

AMS side: each monitored application must allow the Pulse host's IP in its REST IP filter
(`remoteAllowedCIDR`), otherwise AMS answers HTTP 403 and that application is skipped.

## 2. Web UI, API and security

| Variable | Default | Purpose |
|---|---|---|
| `PULSE_LISTEN_ADDR` | `:8090` | Listener for UI + API + beacons (inside the container). |
| `PULSE_HOST_PORT` | `8090` | **Quickstart only** — host port the UI is published on. |
| `PULSE_BASE_URL` | — | Public URL of this Pulse (e.g. `https://pulse.example.com`); set it behind a proxy. |
| `PULSE_SECRET_KEY` | generated file | 64-hex-character key encrypting stored credentials (AES-256-GCM) and keying API-token hashes. Generate with `openssl rand -hex 32`; keep it — changing it invalidates tokens and stored secrets. *Secret.* |
| `PULSE_ALLOWED_WS_ORIGINS` | same origin | Extra origins allowed to open the live-dashboard WebSocket. |
| `PULSE_CORS_ALLOWED_ORIGINS` | none | Origins allowed to call `/api/v1` from a browser. |
| `PULSE_METRICS_TOKEN` | — | Bearer token for `/metrics`. **Unset = unauthenticated**; set it on any reachable host. *Secret.* |
| `PULSE_OIDC_ISSUER`, `_CLIENT_ID`, `_CLIENT_SECRET`, `_REDIRECT_URL`, `_GROUP_CLAIM`, `_GROUP_ROLE_MAP`, `_DEFAULT_ROLE` | — | SSO via OpenID Connect. |
| `PULSE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |

## 3. License

**Pulse is free.** Every feature is included on every install — no license key is needed, no
node or retention limits apply, and commercial use is included. **Settings → License** shows
"Pulse is free — every feature is included, with no node or retention limits. No license key
is needed." and limits read "Unlimited".

| Variable | Default | Purpose |
|---|---|---|
| `PULSE_LICENSE_KEY` | — | Accepted for backward compatibility; does not change behavior. |
| `PULSE_LICENSE_FILE` | — | Path to a license file; accepted for compatibility, not needed. |
| `PULSE_LICENSE_PUBKEY` | official key | Verification key (compatibility). |

License keys still load and verify (offline, ed25519 signature) but change nothing.

## 4. Data retention and storage

| Variable | Default | Purpose |
|---|---|---|
| `PULSE_CLICKHOUSE_DSN` | `clickhouse://localhost:9000/pulse` | ClickHouse connection (the quickstart points it at its own container). |
| `PULSE_RETENTION_DAYS` | `90` | How long raw events are kept (days). |
| `PULSE_ROLLUP_TTL_DAYS` | `395` | How long hourly/daily rollups are kept (≈ 13 months). |
| `PULSE_META_DSN` | `pulse_meta.db` | SQLite file for configuration (rules, channels, tokens, users, audit log). `PULSE_POSTGRES_DSN` switches to Postgres. |

Retention is configuration only — there is no query cap. You can query all data within your
retention window.

## 5. Ingest health and viewer QoE

| Variable | Default | Purpose |
|---|---|---|
| `PULSE_INGEST_TARGET_BITRATE_KBPS` | `2000` | "Healthy" ingest bitrate used by the 0–100 health score. Set it to your typical encoder bitrate. |
| `PULSE_INGEST_TARGET_FPS` | `30` | Healthy frame rate for the score (AMS 3.x REST does not report FPS; the score re-weights). |
| `PULSE_INGEST_LISTEN_ADDR` | — | Separate listener (e.g. `:8091`) exposing **only** beacon ingest, for internet-facing players. |
| `PULSE_SESSION_IDLE_TIMEOUT` | `5m` | A viewer session closes after this long without beacons. |
| `PULSE_GEO_MMDB_PATH` | — | GeoIP country/region lookups need a MaxMind GeoLite2 `.mmdb` you download yourself (not bundled). |
| `PULSE_ANONYMIZE_IP` | `false` | Zero the last IPv4 octet / last 80 IPv6 bits **before** the GeoIP lookup. Viewer IPs are never stored either way. |

The player side needs an **ingest token** (Settings → Ingest Tokens) and the beacon SDK in
your player — `docs/beacon-sdk.md` in the repository.

## 6. Alerting

Rules and channels are created in the UI (**Alerts**) or the API; see
[`alerting-guide.md`](alerting-guide.md). An e-mail channel needs an SMTP server (`host:port`)
in its form — without one Pulse tries `localhost:587`, which in the container is the container
itself. A rule notifies only the channels ticked under **Notify channels**.

## 7. Optional inputs and outputs

| Variable | Default | Purpose |
|---|---|---|
| `PULSE_WEBHOOK_ADDR` + `PULSE_WEBHOOK_SECRET` | — | Receiver (e.g. `:8092`) for **HMAC-signed** AMS lifecycle webhooks. AMS's own `listenerHookURL` sends unsigned hooks, which Pulse rejects by design; this input is for senders that sign. |
| `PULSE_KAFKA_BROKERS`, `PULSE_KAFKA_TOPICS`, `PULSE_KAFKA_GROUP_ID` | — | **Experimental** AMS Kafka input; not yet validated against a real AMS broker, plaintext only. |
| `PULSE_REPORTS_DIR` | `./pulse-reports` | Where scheduled CSV/PDF reports are written. |
| `PULSE_S3_ENDPOINT`, `PULSE_S3_BUCKET`, `PULSE_S3_REGION`, `PULSE_S3_PREFIX` | — / — / `us-east-1` / `reports/` | Upload reports to S3-compatible storage (keys via `PULSE_S3_ACCESS_KEY_ID` / `PULSE_S3_SECRET_ACCESS_KEY`). |
| `PULSE_REPORT_LOGO_PATH` | built-in | Logo for PDF reports (white-label). |

## 8. Kubernetes (Helm)

The chart maps the same settings to `values.yaml` (`pulse.ams.url`, `pulse.listenAddr`,
`pulse.retentionDays`, …) and reads secrets from the Secret named in `pulse.secretRef.name`.
Settings without a dedicated value go in `pulse.extraEnv` — see
[`installation-guide.md`](installation-guide.md) §6.
