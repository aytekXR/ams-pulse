# Pulse — Dashboard Access

## The short answer (for the "Open the dashboard" step)

| | |
|---|---|
| **URL** | `http://<your-server>:8090` |
| **Default port** | **8090** (quickstart: change with `PULSE_HOST_PORT`) |
| **Protocol** | Plain HTTP out of the box; HTTPS through your own reverse proxy |
| **Sign-in** | The one-time admin token the installer prints (`plt_…`) |
| **Production URL** | `https://<your-pulse-hostname>` — chosen by each operator, not fixed by Pulse |

The live dashboard is the landing page (`/`). It needs no AMS setup in the UI when AMS was
configured during installation (the quickstart does this from its flags).

---

## Ports

| Port | What | When it is open | Set by |
|---|---|---|---|
| **8090** | Web UI, REST API (`/api/v1/…`), WebSocket (`/api/v1/live/ws`), health (`/healthz`), Prometheus (`/metrics`), player beacons (`/ingest/beacon`) | Always | `PULSE_LISTEN_ADDR` (default `:8090`) inside the container; the quickstart publishes it as `PULSE_HOST_PORT` (default `8090`) |
| **8091** | Dedicated player-beacon listener (lets you expose only beacon ingest to the internet) | Only if `PULSE_INGEST_LISTEN_ADDR` is set (the Helm chart sets `:8091`) | `PULSE_INGEST_LISTEN_ADDR` |
| **8092** | Signed-webhook receiver | Only if `PULSE_WEBHOOK_ADDR` **and** `PULSE_WEBHOOK_SECRET` are set | `PULSE_WEBHOOK_ADDR` |
| 9000 / 8123 | ClickHouse | Inside the Docker/Kubernetes network only — never published | — |

Pulse also makes **outbound** connections: to the AMS REST API (port 5080 by default), to your
notification targets (SMTP, Slack, Telegram, PagerDuty, webhooks), and to probe targets. It
never contacts a vendor server.

## HTTP vs HTTPS

Pulse itself serves plain HTTP. The quickstart publishes port 8090 on **all** network
interfaces, and Docker's published ports bypass host firewalls such as `ufw`. That is fine on
a private LAN or a firewalled test box. For anything else:

1. Publish Pulse on `127.0.0.1:8090` only.
2. Terminate TLS in front of it — nginx, Caddy, Traefik or a cloud load balancer. Reference
   nginx vhosts are in `deploy/nginx/`; the production compose file
   (`deploy/docker-compose.prod.yml`) already binds to loopback.
3. Proxy the WebSocket path `/api/v1/live/ws` with the `Upgrade`/`Connection` headers, or the
   dashboard falls back to polling (badge shows **Polling** instead of **Live**). If the UI
   and API are served under a different origin, list it in `PULSE_ALLOWED_WS_ORIGINS`.
4. Set `PULSE_BASE_URL=https://<your-pulse-hostname>`.

On Kubernetes, use the chart's `ingress` values for the UI/API service (port 8090) and
`ingressIngest` for the beacon listener (8091), with TLS at the ingress.

## Signing in

- **First sign-in:** paste the bootstrap admin token printed by the installer, or find it with
  `docker compose … logs pulse | grep 'FIRST RUN'`. It is shown on the very first boot only.
- **More people:** create additional API tokens in **Settings → API Tokens** (or
  `POST /api/v1/admin/tokens`). User accounts are managed through the API
  (`/api/v1/admin/users`); the **Users** tab in Settings is still a placeholder.
- **SSO:** OpenID Connect sign-in — set `PULSE_OIDC_ISSUER`, `PULSE_OIDC_CLIENT_ID`,
  `PULSE_OIDC_CLIENT_SECRET`, `PULSE_OIDC_REDIRECT_URL`; the sign-in page then shows
  **Sign in with SSO**.
- **Lost the only admin token:** there is no reset command. On a fresh install, delete the
  `pulse-data` volume and re-run the installer. On a populated install, use another admin
  token to create a new one.

## Checking that Pulse is polling your AMS

- Top-right badge **Live** (green) — the dashboard receives pushed updates.
- Your AMS streams are listed under **Active streams** within seconds of the first poll
  (5-second interval).
- `curl -s http://<your-server>:8090/healthz` → read the `collector` component specifically;
  `degraded` with a message such as `connection refused` or `HTTP 403` means Pulse cannot read
  AMS (see [`troubleshooting.md`](troubleshooting.md)).
- **Settings → Sources** lists only AMS sources added *in the UI*. An AMS configured during
  installation (environment variables) does not appear there, even though Pulse is polling it.

## Main screens

| Path | Screen |
|---|---|
| `/` | Live dashboard — viewers, publishers, CPU/RAM, protocol mix, per-application and per-stream tables |
| `/alerts` | Alert rules, notification channels, alert history |
| `/ingest` | Ingest health per stream, bitrate / FPS / loss / jitter timelines |
| `/qoe` | Viewer QoE from the beacon SDK |
| `/analytics` | Historical audience, geo and device breakdowns |
| `/probes` | Synthetic probes |
| `/reports` | Usage reports, schedules, tenants |
| `/anomalies` | Anomaly detection flags |
| `/fleet` | AMS nodes (cluster discovery) |
| `/audit-log` | Admin changes |
| `/settings` | Sources, API/ingest tokens, integrations, license, users |
| `/onboarding` | Optional setup wizard |
