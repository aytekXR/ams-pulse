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

**Paths updated for REAL AMS 3.0.3 (D-029/D-029v, curl-verified 2026-06-21).** The
earlier root-level `/rest/v2/broadcasts/...` paths 404'd; real AMS uses per-app paths.

| amsclient method | HTTP path | Purpose / real-wire note |
|---|---|---|
| `ListApplications` | `GET /rest/v2/applications` | Discover apps — **array-of-strings** envelope `{"applications":["LiveApp",…]}` |
| `ListBroadcastsPaged` | `GET /{app}/rest/v2/broadcasts/list/{offset}/{size}` | All broadcasts, 200/page (per-app) |
| `BroadcastStatistics` | `GET /{app}/rest/v2/broadcasts/{streamId}/broadcast-statistics` | Watcher-count totals (RTMP count can be **-1** = untracked) |
| `WebRTCClientStats` | `GET /{app}/rest/v2/broadcasts/{streamId}/webrtc-client-stats/0/100` | Per-peer WebRTC QoE (empty `[]` when no viewers) |
| `ClusterNodes` | `GET /rest/v2/cluster/nodes` | Node list — **404 on standalone** → mapped to nil (no error) |
| `NodeInfo` | `GET /rest/v2/cluster/nodes/{nodeId}` | Single-node detail (404-tolerant) |
| `SystemStats` | `GET /rest/v2/system-status` | `{osName,osArch,javaVersion,processorCount}` — **no cpu/mem** on AMS 3.x |

**D-029v real-wire units (curl-verified — get these wrong and the dashboard lies):**
broadcast `bitrate` is **bits/sec** (÷1000 → kbps); `speed` is a **realtime ratio**
(≈1.0), NOT a bitrate; `jitterMs`/`rttMs` are **milliseconds**; `packetLostRatio`
is a **0..1 fraction** (×100 → pct); `currentFPS` is **absent** from the REST
broadcast object on AMS 3.0.3 (health scoring redistributes the FPS weight);
`terminated_unexpectedly` is a real broadcast status (crash) → emit publish_end.

> **⚠️ Implicit RTMP broadcasts (S17 live finding, D-079):**
> AMS 3.0.3 auto-creates a broadcast object when an RTMP publisher connects
> **without** a prior `POST /{app}/rest/v2/broadcasts/create`.  While the
> publisher is live the object appears in `ListBroadcastsPaged` with
> `status=broadcasting`.  When the publisher disconnects, AMS **deletes** the
> object entirely — `GET /{app}/rest/v2/broadcasts/{streamId}` returns HTTP 404
> immediately; the broadcast **never** transitions to `status=finished` or
> `status=terminated_unexpectedly`.  Those two terminal statuses have been
> observed only on REST-pre-created broadcasts (S17 presumption; direct S18
> verification pending).  This is the normal RTMP workflow; pre-creating
> a broadcast is optional, not required for publishing.
>
> Pulse handles this correctly: `detectEnded()` (`restpoller.go:222–265`) fires
> when a `status=broadcasting` stream that was present in the previous poll
> cycle is **absent** from the current broadcast list.  It emits
> `EventStreamPublishEnd` with `reason: "disappeared"` — not `reason:
> "finished"`.  The stream disappears from `GET /api/v1/live/streams` within
> one poll cycle; D-079 live evidence measured 7 s in practice (PRD ≤10 s).
>
> **Developer implication:** integrations that poll `GET /api/v1/live/streams`
> or the AMS REST API looking for `status: finished` on an implicit RTMP
> broadcast will never see it.  Treat a broadcast's **absence from the active
> list** as its terminal state; do not wait for `status: finished`.  See
> DG-11 (scenario-matrix.md S17 Corrections #2) for the test evidence.

Auth (D-029): AMS 3.0.3 Enterprise has JWT disabled (`jwtServerControlEnabled=false`),
so amsclient uses **cookie-session** auth — `POST /rest/v2/users/authenticate
{email,password}` → JSESSIONID via a custom IP-safe cookie jar, with re-login +
single-retry on 401/403 (throttled vs IP-block storms). `PULSE_AMS_LOGIN_EMAIL` +
`PULSE_AMS_LOGIN_PASSWORD` drive it. `PULSE_AMS_AUTH_TOKEN` (static Bearer) is still
supported but unset for this server.

TLS: the amsclient uses a plain `http.Client` with no TLS enforcement beyond what
the Go stdlib provides. If `PULSE_AMS_URL` begins with `https://`, the stdlib TLS
stack verifies the server cert against system roots. There is no `InsecureSkipVerify`
option; use `http://` only on trusted private networks.

Body safety: every response is capped at 10 MB (`client.go:179`). Individual
request timeout defaults to 10 s (`serve.go:134`).

AMS version tolerance: the client was hardened in W2c (D-025) + D-030 to tolerate
v2.10/v2.14/v3.0 wire variance — unknown JSON fields are silently ignored, missing
fields zero. ~~`speed` was a bitrate fallback~~ — **removed D-030**: `speed` is a
dimensionless realtime ratio (≈1.0), not a bitrate; the old fallback emitted ~1 "kbps"
garbage. The `version` field on `ClusterNodeDTO` is preserved so the Fleet page can
render it. Empty `streamId` in list responses is guarded. See the D-029v real-wire
units table above for the complete set of unit corrections.

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
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env"
```

Bring up (or restart with the new overlay). Use the stamped-build two-step
(D-058) — `--build` mixed into `up -d` loses `VERSION`/`COMMIT`/`BUILD_DATE` stamps:

```bash
# 1. Build the pulse image with explicit version stamps:
sg docker -c "docker compose $DC build \
  --build-arg VERSION=$(git describe --tags --always) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  pulse"

# 2. Start WITHOUT --build — uses the pre-built stamped image:
sg docker -c "docker compose $DC up -d pulse"
```

If the Pulse image has not changed (env-only restart), omit step 1 and run only
step 2.

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
| `webhook_secret` | Per-source HMAC secret for `/webhook/ams/{name}` (B7, D-062) — write-only, stored AES-GCM encrypted; reading back shows `webhook_secret_set: true/false` | No |

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

The webhook handler registers two paths on the webhook listener port
(legacy: `webhook.go:64`; per-source: `webhook.go:67`). With `PULSE_WEBHOOK_ADDR=:8091`,
AMS should POST events to the shared path:

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

### 4.5 Configure a sender to POST webhooks

> **⚠️ AMS 3.0.3 REALITY CHECK (D-066, verified against the live console REST):**
> AMS application settings expose `listenerHookURL` (plus retry/content-type
> knobs) but **NO field for an HMAC secret or a custom signature header** — AMS
> lifecycle webhooks are UNSIGNED. Pulse's webhook listener is fail-closed on
> `X-Ams-Signature` HMAC by design, so pointing `listenerHookURL` at Pulse would
> yield only 401s and WARN noise. **Do not configure it.** The REST poller (5 s
> interval) is the supported AMS ingest and already meets the ≤10 s visibility
> budget. The webhook path is for HMAC-capable senders (custom middleware, a
> signing proxy, or a future AMS version that signs hooks).
>
> **Downstream impact of absent webhook delivery:**
>
> - `recording_gb` in `GET /api/v1/reports/usage` is **always 0** on an AMS
>   3.0.3 deployment.  VoD recording-size data arrives only via the `vodReady`
>   webhook event; Pulse has no REST-poll path for VoD lists
>   (`/{app}/rest/v2/vods/list/...` is not polled).  This is
>   **BUG-002** on the roadmap (AV-09 CONFIRMED).
> - Stream start/stop visibility is **not** degraded.  The REST poller
>   (`detectEnded` in `restpoller.go`) detects stream appearance and
>   disappearance from the broadcast list within 4–7 s on the production AMS
>   (D-079 live evidence); the PRD ≤10 s budget is met.  The
>   `liveStreamStarted`/`liveStreamEnded` webhook events are simply not
>   delivered, but REST polling already covers this path; there is no net
>   latency regression versus a correctly-signed webhook deployment.
>
> **Workarounds:**
>
> - *Stream lifecycle:* REST polling is complete and sufficient.  No operator
>   action required.
> - *VoD recording tracking:* No current workaround.  A REST-poll fallback
>   (`/{app}/rest/v2/vods/list/{offset}/{size}`) is the planned fix
>   (BUG-002 roadmap item); `recording_gb` will remain 0 until BUG-002 is
>   implemented.
>
> **Future path — D-V2-1 (OPEN decision):**
>
> Two options are under consideration (agents/handoffs/ROADMAP-V2.md §2.6, D-V2-1):
> (a) build an unsigned-webhook ingest mode gated on a `PULSE_WEBHOOK_ALLOW_UNSIGNED_SOURCES`
> IP CIDR allowlist env var so AMS 3.x hook deliveries from a known IP are
> accepted without HMAC; (b) maintain REST-poll-only indefinitely.  No
> code exists for option (a).  The operator must decide the preferred option
> before any build work begins on D-V2-1.

**Shared route (legacy, all sources):**

For any sender that CAN sign requests:
- **Webhook URL:** `https://your-domain/webhook/ams`
- **Webhook secret:** the value of `PULSE_WEBHOOK_SECRET`
- **Header name:** `X-Ams-Signature` (value `sha256=<hex(HMAC-SHA256(body, secret))>`)

**Per-source route (B7, multi-source deployments):**

Use a distinct URL per AMS source: `https://your-domain/webhook/ams/{source_name}`.
Set the per-source secret via the API (see section 7, B7). Example for a source
named `production-eu`:
- **Webhook URL:** `https://your-domain/webhook/ams/production-eu`
- **Webhook secret:** the per-source secret set via `PUT /api/v1/admin/sources/{id}` with `{"webhook_secret": "..."}`
- **Header name:** `X-Ams-Signature`

When a per-source secret is configured for a source name, only that secret is
accepted on the per-source URL — the global `PULSE_WEBHOOK_SECRET` is not
accepted for that source. See section 7 (B7) for auth semantics and the startup-only
load limitation.

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

### B7 — Per-source webhook HMAC secret (shipped D-062)

Per-source webhook secrets are implemented and live. A multi-source deployment can
assign a distinct HMAC secret to each AMS source so that one compromised node cannot
inject events for another.

**How it works:**

- **Storage:** `ams_sources.webhook_secret_enc TEXT` (AES-256-GCM encrypted,
  `contracts/db/meta/0001_init.sql:88`).
- **Write field:** `SourceWrite.webhook_secret` — nullable string, write-only, stored
  encrypted (`contracts/openapi/pulse-api.yaml:2672`).
- **Read flag:** `SourceRead.webhook_secret_set` — boolean, `true` when a per-source
  secret is stored; the secret value is never echoed back (`pulse-api.yaml:2631`).
- **Routes:**
  - Legacy: `POST /webhook/ams` — uses the global `PULSE_WEBHOOK_SECRET` (`webhook.go:64`).
  - Per-source: `POST /webhook/ams/{source_name}` — uses the per-source secret for that
    name, with no fallback to the global secret (`webhook.go:67`).
- **Auth semantics:**
  - If `SourceSecrets[name]` is set: per-source secret is used **exclusively** — the
    global `PULSE_WEBHOOK_SECRET` is NOT accepted for that source.
  - If the source name is unknown: falls back to `PULSE_WEBHOOK_SECRET`; if that is
    also empty, responds 401 (fail-closed).
  - Unknown source names never return 404 — to avoid leaking which source names exist
    (returns 200 when SharedSecret is valid, 401 when it is empty or the HMAC is wrong).
- **Startup-only load:** `SourceSecrets` is built once at startup from the meta store
  (`serve.go:286–301`). **Rotating a per-source secret requires a `pulse` process
  restart** to take effect (B7 limitation, `serve.go:279–280`).

**Set a per-source secret via the API:**

```bash
curl -s -X PUT https://your-domain/api/v1/admin/sources/${SOURCE_ID} \
  -H "Authorization: Bearer plt_<your-admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"webhook_secret": "your-strong-per-source-secret"}'
```

**Concrete operator example — configure AMS to POST to the per-source URL:**

In AMS Management Console > Settings > Webhooks for the source named `production-eu`:
- **Webhook URL:** `https://beyondkaira.com/webhook/ams/production-eu`
- **Webhook secret:** the value you set in `webhook_secret` above
- **Header name:** `X-Ams-Signature`

Pulse will validate the HMAC using the per-source secret stored for `production-eu`.
A different AMS instance (e.g. `production-us`) must use its own per-source URL
(`/webhook/ams/production-us`) and its own secret; its requests on
`/webhook/ams/production-eu` will be rejected.

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
agents/handoffs/RESUME-PROMPT.md, then docs/AMS-INTEGRATION.md
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
       DC="-p pulse-prod -f deploy/docker-compose.yml -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f deploy/docker-compose.backup.yml --env-file deploy/.env"
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
   - B7 is implemented and shipped (D-062). Per-source webhook secrets are live:
     see section 4.5 for the full operator guide. No further work is required
     unless rotating per-source secrets without a restart (currently needs a
     restart — see B7 limitation note in section 7).
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
  - Contracts are frozen (D-004) — B7 contract fields are already in pulse-api.yaml (webhook_secret_set at line 2631, SourceWrite.webhook_secret at line 2672); no further contract changes are needed for B7.
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
