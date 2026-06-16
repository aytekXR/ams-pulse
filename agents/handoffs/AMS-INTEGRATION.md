# Pulse — AMS Integration Guide (Operator + Next-Session)

> **Audience:** an operator connecting Pulse to a real Ant Media Server, and any
> developer or agent picking up the real-AMS integration in a future session.
>
> **Accuracy note:** every file reference, endpoint path, env var name, and code
> fact below was read directly from the source files cited. Nothing is inferred
> from documentation or memory.

---

## 1. How Pulse ingests from AMS

Pulse has three ingest paths for server-side data, and one client-side SDK:

### 1.1 REST polling (primary — always active)

`server/pkg/amsclient/client.go` implements a typed, read-only REST client.
`server/internal/collector/restpoller` polls AMS on `PULSE_POLL_INTERVAL`
(default 5 s) and is always wired regardless of other source config
(`serve.go:169–181`).

AMS endpoints polled per cycle:

| amsclient method | HTTP path | Purpose |
|---|---|---|
| `ListApplications` | `GET /rest/v2/applications` | Discover app names |
| `ListBroadcastsPaged` | `GET /rest/v2/broadcasts/{app}/list?offset=N&size=200` | All broadcasts, paginated 200/page |
| `BroadcastStatistics` | `GET /rest/v2/broadcasts/{app}/{streamId}/statistics` | Per-stream watch-time totals |
| `WebRTCClientStats` | `GET /rest/v2/broadcasts/{app}/{streamId}/webrtc-client-stats/0/100` | Per-peer WebRTC QoE |
| `ClusterNodes` | `GET /rest/v2/cluster/nodes` | Node list (cluster deployments) |
| `NodeInfo` | `GET /rest/v2/cluster/nodes/{nodeId}` | Single-node detail |
| `SystemStats` | `GET /rest/v2/system/stats` | System resource counters |

Auth: every request sends `Authorization: Bearer <PULSE_AMS_AUTH_TOKEN>` when the
token is non-empty (`client.go:133–143, 162`). Auth is optional — some on-premise
AMS installs run with auth disabled.

TLS: the amsclient uses a plain `http.Client` with no TLS enforcement beyond what
the Go stdlib provides. If `PULSE_AMS_URL` begins with `https://`, the stdlib TLS
stack verifies the server cert against system roots. There is no `InsecureSkipVerify`
option; use `http://` only on trusted private networks.

Body safety: every response is capped at 10 MB (`client.go:179`). Individual
request timeout defaults to 10 s (`serve.go:134`).

AMS version tolerance: the client was hardened in W2c (D-025) to tolerate
v2.10/v2.14/v3.0 wire variance — unknown JSON fields are silently ignored,
missing fields zero. The `speed` field (v2.10) is used as a bitrate fallback when
`bitrate` is 0. The `version` field on `ClusterNodeDTO` is preserved so the Fleet
page can render it. Empty `streamId` in list responses is now guarded.

### 1.2 Webhook source (low-latency, now wired)

`server/internal/collector/webhook/webhook.go` receives AMS lifecycle events by
HTTP POST. Lower latency than polling for publish-start visibility (F1 criterion:
10 s) and alert detection (F5 criterion: 30 s). Activated when both
`PULSE_WEBHOOK_ADDR` and `PULSE_WEBHOOK_SECRET` are set (`serve.go:193–205`).

**Fail-closed** (B2): `serve.go:195–196` logs an error and skips starting the
listener when `PULSE_WEBHOOK_ADDR` is set but `PULSE_WEBHOOK_SECRET` is empty.
`validateHMAC` in `webhook.go:217–225` independently returns `false` when the
secret is empty, so even if the listener were somehow started without a secret,
every request would be rejected.

### 1.3 Kafka source (optional, high-throughput)

Activated when `PULSE_KAFKA_BROKERS` is non-empty (`serve.go:210–221`). Not
covered further here; see `server/internal/collector/kafka`.

### 1.4 Beacon-JS SDK (client QoE)

`sdk/beacon-js` collects player-side QoE (startup time, stall ratio, bitrate,
packet loss) from browser viewer sessions. Beacons POST to `/ingest/beacon` on
the main API port (`:8090`) or the dedicated ingest listener
(`PULSE_INGEST_LISTEN_ADDR`). Requires an ingest token (kind=ingest). Populates
`qoe/summary` with viewer-perceived quality data that REST polling cannot provide.
A Pro+ license lifts the ingest gate (see `internal/license/license.go`).

---

## 2. Prerequisites

Before connecting Pulse to a real AMS node:

1. **AMS REST API reachable** — the AMS node must expose its REST API at a URL
   accessible from inside the Pulse Docker network (typically the Docker bridge
   or a private LAN IP). Default AMS REST port is **5080** (HTTP) or **5443**
   (HTTPS).

2. **Bearer token** — log into AMS Management Console > Settings > Security to
   generate or retrieve the REST API token. Some self-hosted AMS installs have
   auth disabled; in that case leave `PULSE_AMS_AUTH_TOKEN` empty.

3. **Network path** — Pulse's `pulse` container must be able to open a TCP
   connection to `PULSE_AMS_URL`. Verify with:
   ```
   sg docker -c "docker exec pulse curl -s http://<ams-host>:5080/rest/v2/version"
   ```

4. **AMS version** — v2.8 and above are supported. The amsclient tolerates field
   variance across v2.10, v2.14, and v3.0 (W2c, D-025). Earlier versions have
   not been tested; the primary risk is an unknown `/rest/v2/applications`
   envelope shape.

5. **TLS recommendation** — use `https://` for the AMS URL in production so that
   the bearer token travels encrypted. See section 5 (Security).

---

## 3. Step-by-step: connect a real AMS

### 3.1 Set env vars in `deploy/.env`

Add or update these lines in `deploy/.env` (gitignored):

```env
PULSE_AMS_URL=https://your-ams-host:5443
PULSE_AMS_AUTH_TOKEN=your-ams-bearer-token
PULSE_AMS_NODE_ID=ams-node-01
PULSE_AMS_APPLICATIONS=live,vod
```

`PULSE_AMS_APPLICATIONS` is optional. When omitted or empty, Pulse calls
`/rest/v2/applications` on each poll cycle and monitors all discovered apps
(`config.go:178–185`). Set it explicitly to narrow polling to specific apps and
reduce load on AMS.

### 3.2 Bring up the production stack with the real-AMS overlay

The overlay file `deploy/docker-compose.real-ams.yml` reassigns the `mock-ams`
service to an unused profile (so it never starts) and overrides the `pulse`
service environment with the `PULSE_AMS_*` vars from `.env`.

Define the compose project alias (run from the repo root):

```bash
DC="-p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.prod-tls.yml \
  -f deploy/docker-compose.real-ams.yml \
  --env-file deploy/.env"
```

Bring up (or restart with the new overlay):

```bash
sg docker -c "docker compose $DC up -d --build pulse"
```

Using `--build pulse` rebuilds only the Pulse image (not Caddy/ClickHouse) and
restarts it with the new environment. Omit `--build` if the image has not changed.

### 3.3 Confirm Pulse started against the real AMS

```bash
sg docker -c "docker compose $DC logs pulse --tail=50"
```

Look for:
- `pulse: all services started`
- `webhook: listening addr=:8091` (if webhook is enabled)
- No `amsclient: GET /rest/v2/applications: HTTP 401` errors

### 3.4 Register the source via the admin sources API

The admin sources API (at `/api/v1/admin/sources`) stores AMS source metadata in
the meta store (SQLite) for the UI and the source-test endpoint. Registering a
source here is **separate** from the env-var-driven REST poller; it gives you the
connectivity test and future multi-source support.

First obtain your admin token:

```bash
sg docker -c "docker compose $DC logs pulse" | grep plt_
```

Then register the source:

```bash
curl -s -X POST https://your-domain/api/v1/admin/sources \
  -H "Authorization: Bearer plt_<your-admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production AMS",
    "type": "antmedia",
    "rest_url": "https://your-ams-host:5443",
    "rest_user": "admin",
    "rest_password": "your-ams-rest-password"
  }'
```

Request fields accepted by `amsSourceFromAPI` (`server.go:1857–1893`):

| Field | Purpose | Required |
|---|---|---|
| `name` | Display name | Yes |
| `type` | Source type (use `"antmedia"`) | Yes |
| `rest_url` | AMS REST base URL — must be `http://` or `https://` (scheme-validated, file/ftp/gopher rejected) | No |
| `rest_user` | Basic-auth username for AMS REST (optional) | No |
| `rest_password` | Basic-auth password — stored AES-GCM encrypted in the meta store | No |
| `log_path` | Path to AMS analytics log file (logtail source, not yet wired) | No |
| `credential_env_ref` | Name of an env var holding the credential (alternative to `rest_password`) | No |

The response includes an `id` field. Save it as `SOURCE_ID`.

### 3.5 Verify connectivity with the test endpoint

```bash
curl -s -X POST https://your-domain/api/v1/admin/sources/${SOURCE_ID}/test \
  -H "Authorization: Bearer plt_<your-admin-token>"
```

The test handler (`server.go:1328–1420`) GETs `{rest_url}/rest/v2/version` with a
5-second timeout. It uses a redirect-blocking client (no SSRF via redirect chains).
Response shape:

```json
{
  "reachable": true,
  "status": "ok",
  "message": "HTTP 200 from https://your-ams-host:5443/rest/v2/version",
  "latency_ms": 42
}
```

`reachable: true` means any HTTP response was received (including 4xx/5xx).
`status: "error"` means a 4xx/5xx was returned — check AMS auth config.
`reachable: false` means a network error (timeout, DNS failure, TLS cert mismatch).

Note (B6): when `rest_user` is set, the test sends an empty password string
(`server.go:1379`). This is a known limitation — the stored encrypted credential
is not decrypted for the connectivity test. A reachable-but-auth-failed result
from the test endpoint does not mean polling will fail; the REST poller uses the
`PULSE_AMS_AUTH_TOKEN` bearer token, not the stored basic-auth credential.

### 3.6 Confirm live data

```bash
curl -s https://your-domain/api/v1/live/overview \
  -H "Authorization: Bearer plt_<your-admin-token>"
```

When AMS has active streams, `total_viewers` and `total_publishers` will be
non-zero. The REST poller runs every `PULSE_POLL_INTERVAL` (default 5 s), so
allow up to 10 s after startup.

---

## 4. Webhook setup over HTTPS

### 4.1 Set webhook env vars

In `deploy/.env`:

```env
PULSE_WEBHOOK_ADDR=:8091
PULSE_WEBHOOK_SECRET=your-strong-random-secret
```

`PULSE_WEBHOOK_ADDR` is the listen address for the webhook HTTP server
(`config.go:157`). `PULSE_WEBHOOK_SECRET` is the shared HMAC-SHA256 secret
(`config.go:158`).

### 4.2 The webhook listener and its path

The webhook handler registers at path `/webhook/ams` on the webhook listener port
(`webhook.go:54`). With `PULSE_WEBHOOK_ADDR=:8091`, AMS should POST events to:

```
http://<pulse-host>:8091/webhook/ams
```

When fronted by Caddy over HTTPS (see below), the public URL becomes:

```
https://your-domain/webhook/ams
```

### 4.3 HMAC signature validation

Pulse reads the `X-Ams-Signature` request header (`webhook.go:116`) and validates
it as `sha256=<hex(HMAC-SHA256(body, secret))>` (`webhook.go:217–225`). AMS must
be configured to send this header with the same secret. If the signature is missing
or wrong, the handler returns HTTP 401 and logs a warning.

Fail-closed: `validateHMAC` returns `false` when `secret == ""` so a misconfigured
instance cannot accidentally accept unsigned webhooks even if `serve.go`'s own
guard were bypassed.

### 4.4 Add a Caddy route for the webhook path

The current `deploy/config/Caddyfile.prod` does **not** proxy `/webhook/*` to the
webhook port. The webhook listener (`:8091`) is not proxied by Caddy, so AMS
cannot reach it over HTTPS without an explicit route. Add the following `handle`
block to `Caddyfile.prod` inside the `{$PULSE_DOMAIN} { ... }` block, **before**
the catch-all `handle { ... }` block:

```caddyfile
# AMS webhook — proxy /webhook/* to the webhook listener port.
# Required to receive AMS lifecycle events over HTTPS.
handle /webhook/* {
    reverse_proxy pulse:8091 {
        header_up X-Forwarded-For {remote_host}
        header_up X-Real-IP {remote_host}
    }
}
```

After editing `Caddyfile.prod`, restart Caddy (not just reload — a restart is
needed to pick up new `handle` blocks on the same hostname):

```bash
sg docker -c "docker compose $DC restart caddy"
```

### 4.5 Configure AMS to POST webhooks

In AMS Management Console > Settings > Webhooks:
- **Webhook URL:** `https://your-domain/webhook/ams`
- **Webhook secret:** the value of `PULSE_WEBHOOK_SECRET`
- **Header name:** `X-Ams-Signature`

AMS sends events for stream start, stream stop, and recording-ready. The webhook
handler maps them to domain events via the `action`, `event`, and `type` fields
(`webhook.go:159–210`). Supported action strings: `liveStreamStarted`,
`startBroadcast`, `publish_started`, `liveStreamEnded`, `stopBroadcast`,
`publish_ended`, `vodReady`, `recording_ready`.

### 4.6 Expose the webhook port in Docker Compose (if needed)

If Caddy is not involved and you want AMS to POST directly to the Docker host on
port 8091, add an override:

```yaml
# deploy/docker-compose.webhook-port.yml (create if needed)
services:
  pulse:
    ports:
      - "8091:8091"
```

For production behind Caddy, the Caddy route above is preferred (single exposed
port, TLS, no direct host port binding).

---

## 5. Security

### 5.1 Use HTTPS for the AMS URL

The bearer token in `Authorization: Bearer <token>` travels in cleartext over
`http://`. Use `https://` in `PULSE_AMS_URL` for any non-loopback connection so
the token is protected in transit. The amsclient does not emit a startup warning
for `http://` — this is a known gap noted for future improvement.

### 5.2 Inject secrets via Docker secrets (deferred — B3 backlog)

Currently `PULSE_AMS_AUTH_TOKEN` and `PULSE_WEBHOOK_SECRET` are injected as
environment variables from `deploy/.env`. Environment variables are visible in
`docker inspect` output and in `/proc/<pid>/environ`. The production-hardened
pattern uses Docker Compose secrets:

```yaml
# In docker-compose.hardened.yml (pattern to apply — B3 backlog item):
secrets:
  ams_token:
    environment: PULSE_AMS_AUTH_TOKEN
  webhook_secret:
    environment: PULSE_WEBHOOK_SECRET

services:
  pulse:
    secrets:
      - ams_token
      - webhook_secret
```

With mounted secrets, the value is read from a tmpfs file (e.g.
`/run/secrets/ams_token`) rather than from the process environment. Pulse's
current `EnvConfig` reads from `os.Getenv` and does not yet support secret-file
paths. Until B3 is completed, mitigate by restricting `deploy/.env` permissions
(`chmod 600 deploy/.env`) and ensuring the `.env` file is in `.gitignore` (it is).

### 5.3 Stored credentials

AMS REST passwords registered via `POST /api/v1/admin/sources` are encrypted with
AES-GCM using the key derived from `PULSE_SECRET_KEY` before being stored in the
meta SQLite store (`server.go:1885–1890`). Protect `PULSE_SECRET_KEY` — loss
of this key means stored credentials cannot be decrypted.

### 5.4 CORS and CSP

CORS for `/api/v1/*` routes is allowlist-controlled via `PULSE_CORS_ALLOWED_ORIGINS`
(comma-separated). The beacon ingest route (`/ingest/beacon`) is always permissive
(needed for cross-origin browser beacons). CSP is enforced by Caddy headers in
`Caddyfile.prod`.

---

## 6. Env surface

Complete table of `PULSE_*` variables relevant to AMS integration, read from
`server/cmd/pulse/config.go` (`loadEnvConfig`, lines 147–268):

| Variable | Purpose | Default | Required |
|---|---|---|---|
| `PULSE_AMS_URL` | AMS REST API base URL | `http://localhost:5080` | Yes (for real AMS) |
| `PULSE_AMS_AUTH_TOKEN` | Bearer token for AMS REST API | _(empty = no auth)_ | Conditional |
| `PULSE_AMS_NODE_ID` | Node identifier stamped on events | `standalone` | Recommended |
| `PULSE_AMS_APPLICATIONS` | Comma-separated app names to poll; empty = all | _(empty)_ | No |
| `PULSE_POLL_INTERVAL` | REST poll interval (Go duration, e.g. `5s`, `10s`) | `5s` | No |
| `PULSE_WEBHOOK_ADDR` | Webhook HTTP listen address (e.g. `:8091`); empty = disabled | _(empty)_ | No |
| `PULSE_WEBHOOK_SECRET` | HMAC-SHA256 secret for webhook signature validation | _(empty)_ | Required if PULSE_WEBHOOK_ADDR is set |
| `PULSE_KAFKA_BROKERS` | Comma-separated Kafka broker addresses; empty = disabled | _(empty)_ | No |
| `PULSE_KAFKA_GROUP_ID` | Kafka consumer group ID | `pulse-collector` | No |
| `PULSE_LISTEN_ADDR` | Main API listen address | `:8090` | No |
| `PULSE_INGEST_LISTEN_ADDR` | Dedicated beacon ingest listener; empty = use main port | _(empty)_ | No |
| `PULSE_CLICKHOUSE_DSN` | ClickHouse native protocol DSN | `clickhouse://localhost:9000/pulse` | Yes |
| `PULSE_CLICKHOUSE_DATABASE` | ClickHouse database name | `pulse` | No |
| `PULSE_MIGRATIONS_DIR` | Path to ClickHouse migration SQL files | _(empty)_ | No |
| `PULSE_META_DSN` | SQLite meta store path | `pulse_meta.db` | No |
| `PULSE_SECRET_KEY` | AES-GCM key for encrypting stored credentials | _(empty = no encryption)_ | Yes for production |
| `PULSE_RETENTION_DAYS` | Raw event TTL in days | `90` | No |
| `PULSE_ROLLUP_TTL_DAYS` | Rollup table TTL in days | `395` | No |
| `PULSE_METRICS_TOKEN` | Bearer token required on `GET /metrics`; empty = open | _(empty)_ | Recommended |
| `PULSE_CORS_ALLOWED_ORIGINS` | Comma-separated CORS origins for `/api/v1/*` | _(empty)_ | No |
| `PULSE_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `info` | No |
| `PULSE_GEO_MMDB_PATH` | Path to MaxMind .mmdb for geo enrichment; empty = disabled | _(empty)_ | No |
| `PULSE_ANONYMIZE_IP` | Set `true` to anonymize IPs before geo lookup and storage | `false` | No |
| `PULSE_SESSION_IDLE_TIMEOUT` | Viewer session idle close timeout (Go duration) | `5m` | No |
| `PULSE_CLUSTER_DISCOVERY_INTERVAL` | AMS cluster node poll interval | `30s` | No |
| `PULSE_INGEST_TARGET_BITRATE_KBPS` | Expected healthy ingest bitrate for health score | `2000` | No |
| `PULSE_INGEST_TARGET_FPS` | Expected healthy ingest FPS for health score | `30` | No |
| `PULSE_S3_ENDPOINT` | S3-compatible endpoint for report exports; empty = disabled | _(empty)_ | No |
| `PULSE_S3_BUCKET` | S3 bucket for report uploads | _(empty)_ | No |
| `PULSE_S3_PREFIX` | S3 key prefix | `reports/` | No |
| `PULSE_S3_REGION` | S3 region | `us-east-1` | No |
| `PULSE_REPORTS_DIR` | Local directory for generated report artifacts | `pulse-reports` | No |
| `PULSE_LICENSE_KEY` | License key string | _(empty = free tier)_ | No |
| `PULSE_LICENSE_FILE` | Path to license file | _(empty)_ | No |
| `PULSE_LOG_TAIL_PATH` | Path to AMS analytics log file (logtail source, not yet wired in serve.go) | _(empty)_ | No |

---

## 7. Deferred hardening to complete during real-AMS integration

These items are open backlog and directly affect real-AMS production quality.
All are non-blocking for initial connectivity but should be completed before
treating the integration as production-hardened.

### B6 — Source test does not decrypt stored credential (medium, server)

**File:** `server/internal/api/server.go:1379`

```go
req.SetBasicAuth(src.RestUser.String, "") // password from encrypted store; skip for connectivity check
```

The `/admin/sources/{id}/test` endpoint sends an empty password string even when
a `rest_password` was stored. The encrypted credential exists in
`src.CredentialEnc` but is not decrypted before the test request. A source that
requires basic auth will show `reachable: true, status: "error", HTTP 401` from
the test even though the stored credential is correct. Fix: decrypt
`src.CredentialEnc` using `store.Decrypt(src.CredentialEnc.String)` and pass it
to `SetBasicAuth`. This is a straightforward change within the existing
`handleTestSource` function; no contract change required.

### B7 — Per-source webhook HMAC secret (contract change required — needs CR via ORCH)

Currently there is one global webhook secret (`PULSE_WEBHOOK_SECRET`) shared across
all AMS sources. A multi-source deployment needs per-source HMAC secrets so that
one compromised AMS node cannot inject events on behalf of another. This requires:
- A `webhook_secret` column on the `ams_sources` meta table
- A new field in the `POST /api/v1/admin/sources` and `PUT .../admin/sources/{id}`
  request body (contract change)
- The webhook handler selecting the secret by matching the incoming `nodeId` or
  `app` field to a registered source

**This is a contract change.** It must be submitted to ORCH as a CR and applied by
INT-01 before implementation (`contracts/openapi/pulse-api.yaml` + migration SQL).

### B3 — Secrets via Docker secrets rather than env vars (medium, deploy)

`PULSE_AMS_AUTH_TOKEN` and `PULSE_WEBHOOK_SECRET` are currently plaintext env vars
in `deploy/.env`. Migrate to Docker Compose `secrets:` (see section 5.2 pattern).
Requires adding secret-file reading to `loadEnvConfig` or a thin shim in
`server/cmd/pulse/main.go`.

### A2 — Rate-limit beacon ingest on the main API port (medium, server)

The dedicated beacon server (`PULSE_INGEST_LISTEN_ADDR`) has a per-token rate
limiter (100 req/s, burst 200 — `serve.go:326`). The main-port `/ingest/beacon`
handler (`server.go:288`) does not apply this rate limiter, leaving the main port
open to beacon flooding without the dedicated listener active. Fix: wire the same
token-bucket limiter on the main-port handler.

### A7 — Rate-limit `GET /metrics` (low, server)

`GET /metrics` is unauthenticated by default (or token-gated via
`PULSE_METRICS_TOKEN`). There is no rate limit; a Prometheus scraper misconfigured
to scrape every second at high parallelism could exert sustained load. Fix: add a
simple per-IP token bucket (e.g. 10 req/s) to `handleMetrics` in `server.go`.

---

## 8. Ready-to-paste next-session prompt

Drop this into a fresh Claude Code session at the repo root
(`/home/aytek/repo/ams-pulse`):

---

```
You are picking up the Pulse real-AMS integration. Read CLAUDE.md, then
agents/handoffs/RESUME-PROMPT.md, then agents/handoffs/AMS-INTEGRATION.md
before doing anything else. Those three files are the ground truth for state,
operating protocol, and integration facts.

GOAL: connect Pulse to a real Ant Media Server and adversarially validate the
amsclient against live captures.

INPUTS FROM OPERATOR (fill these in before running):
  AMS_URL=https://<your-ams-host>:5443
  AMS_TOKEN=<your-bearer-token>
  AMS_NODE_ID=<node-identifier>
  AMS_APPLICATIONS=live,vod   # or omit for all apps

TASKS — run as an ORCH workflow with Verify + Commit + Handoff flows:

1. WIRE THE REAL-AMS OVERLAY
   - Add PULSE_AMS_URL, PULSE_AMS_AUTH_TOKEN, PULSE_AMS_NODE_ID,
     PULSE_AMS_APPLICATIONS to deploy/.env.
   - Restart the pulse service:
       DC="-p pulse-prod -f deploy/docker-compose.yml -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml --env-file deploy/.env"
       sg docker -c "docker compose $DC up -d pulse"
   - Confirm no 401/403 errors in pulse logs within 30 s.

2. ADVERSARIALLY VALIDATE THE amsclient AGAINST LIVE AMS
   - Curl each AMS endpoint that amsclient calls (see AMS-INTEGRATION.md §1.1
     table) directly from inside the pulse container and capture real JSON.
   - Compare the live JSON shapes to the W2c fixtures in
     server/pkg/amsclient/testdata/*.json (created in D-025).
   - Identify any field renames, envelope changes, or missing fields across
     AMS versions actually deployed.
   - Update fixtures and tests where the live data contradicts the mock. Run
     go test ./... -race from inside the golang:1.25 image to confirm green.

3. REGISTER AND TEST THE SOURCE
   - POST /api/v1/admin/sources with rest_url, rest_user, rest_password.
   - POST /api/v1/admin/sources/{id}/test — confirm reachable: true.
   - GET /api/v1/live/overview — confirm total_viewers or total_publishers > 0
     when AMS has active streams.

4. OPTIONAL WEBHOOK SETUP (if you have PULSE_WEBHOOK_SECRET)
   - Set PULSE_WEBHOOK_ADDR=:8091 and PULSE_WEBHOOK_SECRET in deploy/.env.
   - Add the Caddy route for /webhook/* -> pulse:8091 per AMS-INTEGRATION.md §4.4.
   - Restart caddy, then configure AMS Management Console to POST to
     https://<domain>/webhook/ams with the shared secret.
   - Verify: trigger a publish-start event in AMS and confirm it appears in
     pulse logs within 2 s.

5. COMPLETE DEFERRED HARDENING (scope: server only, no contract changes)
   - B6: fix handleTestSource to decrypt src.CredentialEnc before SetBasicAuth.
     File: server/internal/api/server.go:1379.
   - A2: add per-token rate limit to main-port /ingest/beacon handler. Wire the
     same token-bucket limiter used by the dedicated beacon server.
   - A7: add per-IP rate limit (10 req/s) to handleMetrics in server.go.
   - B7 is a contract change — do NOT implement in this session. File a CR with
     ORCH (log in decisions.md, note: needs AMSSource.webhook_secret column +
     API field + migration). Mark as pending CR.
   - B3 (Docker secrets migration) is a deploy change — ORCH should assess
     scope before starting; flag for next ORCH session if time allows.

6. VERIFY + COMMIT + HANDOFF
   - Per RESUME-PROMPT.md protocol: rebuild (cd server && go test ./... -race
     inside golang:1.25 image); run the e2e compose-up smoke
     (docker-compose.ci.yml) to confirm authed /live/overview still passes.
   - Commit by EXPLICIT path (never git add -A). Message format:
       feat(real-ams) D-026: <summary>
   - Update agents/handoffs/RESUME-PROMPT.md with new status, then commit + push.

CONSTRAINTS (binding — from CLAUDE.md and RESUME-PROMPT.md):
  - AMS wire formats ONLY in server/pkg/amsclient + server/internal/collector.
  - Never run pulse serve or clickhouse server in the foreground inside an agent.
  - Use docker compose up -d (detached) for deploys.
  - Contracts are frozen (D-004) — B7 needs a CR, do not touch API YAML directly.
  - CGO_ENABLED=0 for the binary build; CGO_ENABLED=1 + gcc for go test -race.
  - go is NOT on the VPS PATH — run Go commands inside golang:1.25 images.
```

---

## 9. Verification checklist

Use this checklist after completing integration steps:

- [ ] `docker compose logs pulse | grep "amsclient"` shows no recurring 401 or
      connection-refused errors
- [ ] `GET /healthz` returns `{"status":"ok"}` (HTTP 200)
- [ ] `GET /api/v1/live/overview` (authed) shows `total_viewers` and/or
      `total_publishers` > 0 when AMS has active streams
- [ ] `POST /api/v1/admin/sources/{id}/test` returns `reachable: true`
- [ ] (If webhook enabled) AMS publish-start events appear in pulse logs within
      5 s of a stream going live
- [ ] `go test ./... -race` in `server/` passes green inside `golang:1.25` image
- [ ] `docker compose $DC ps` shows all containers `healthy` or `running`

---

## 10. Troubleshooting

### 401 from AMS REST API

Symptom: pulse logs show `amsclient: GET /rest/v2/broadcasts/live/list: HTTP 401`.

Causes and fixes:
- `PULSE_AMS_AUTH_TOKEN` is wrong, expired, or missing. Regenerate the token in
  AMS Management Console > Settings > Security.
- AMS requires HTTPS but `PULSE_AMS_URL` uses `http://`. Switch to `https://`.
- AMS has IP-based REST API restrictions that exclude the Pulse container IP.

### `http://` vs `https://` mismatch

The amsclient does not warn on startup if `http://` is used. If AMS redirects
`http://` to `https://`, the client will follow the redirect (Go stdlib follows
redirects by default), but the bearer token may not be forwarded to the
`https://` target by some AMS reverse-proxy configurations. Use `https://` from
the start.

The source-test client (`server.go:1387–1392`) uses `CheckRedirect` to block
redirects. A redirect from `http://` to `https://` will appear as
`reachable: false` in the test response (redirect is stopped, treated as an
error). This is by design — it forces the operator to use the correct scheme
in `rest_url`.

### Webhook returns 404

Symptom: AMS logs show HTTP 404 when POSTing to `https://your-domain/webhook/ams`.

Cause: the Caddy route for `/webhook/*` has not been added to `Caddyfile.prod`,
or Caddy was reloaded (not restarted) after adding it.

Fix: add the `/webhook/*` handle block per section 4.4, then restart (not just
reload) Caddy:
```bash
sg docker -c "docker compose $DC restart caddy"
```

### Webhook returns 401 (invalid signature)

Symptom: AMS posts to the webhook endpoint but pulse logs show
`webhook: invalid signature`.

Causes and fixes:
- `PULSE_WEBHOOK_SECRET` in `deploy/.env` does not match the secret configured
  in AMS Management Console.
- AMS is not sending the `X-Ams-Signature` header, or is sending it in a
  different format than `sha256=<hex>`. The handler reads `X-Ams-Signature`
  (`webhook.go:116`) and expects the format `sha256=<hex(HMAC-SHA256(body, secret))>`.
- The header name is wrong in the AMS webhook config (it must be exactly
  `X-Ams-Signature`).

### Source test shows `reachable: true` but `status: "error"` (HTTP 401)

This is expected when `rest_user` is set in the source but `rest_password` was
provided — the test sends the username with an empty password (B6, see section 7).
The REST poller uses the bearer token from `PULSE_AMS_AUTH_TOKEN`, not the stored
basic-auth credential, so polling will still work. Fix B6 to make the test
accurate.

### Dashboard shows 0 viewers after switching to real AMS

- Allow up to 10 s for the first REST poll cycle (`PULSE_POLL_INTERVAL * 2`).
- Confirm AMS has active streams (check AMS Management Console live streams page).
- Check `PULSE_AMS_APPLICATIONS` — if set to `live` but AMS streams are on a
  different app name (e.g. `WebRTCApp`), the poller will find no broadcasts.
  Set to the correct app name or omit to poll all apps.
- Confirm the `pulse` container can reach `PULSE_AMS_URL` from inside Docker:
  ```bash
  sg docker -c "docker exec pulse wget -qO- ${PULSE_AMS_URL}/rest/v2/version"
  ```
