# Pulse — Troubleshooting

Start with the health endpoint and read the **collector** entry, not just the top-level status:

```sh
curl -s http://localhost:8090/healthz
docker compose -f quickstart/docker-compose.quickstart.yml --env-file quickstart/.env logs --tail=100 pulse
```

## Installer

| Symptom | Cause | Fix |
|---|---|---|
| `Error: --password is required (no TTY available …)` | Flags missing when piping into `bash` | Pass `--ams-url`, `--email` **and** `--password` (use `bash -s --` exactly as in the install guide). |
| `Docker is installed but cannot be reached` | User not in the `docker` group, or the daemon is stopped | `sudo usermod -aG docker $USER && newgrp docker`, or `sudo systemctl start docker`. |
| `Docker Compose v2 not found` | Compose plugin missing | Install the Compose plugin (`docker compose version` must work). |
| `The Pulse container image is not accessible` | Tag typo, proxy blocking `ghcr.io`, or rate limiting | Image tags have no `v` (`0.5.0`). For offline hosts, `docker load` the image first — the installer then uses the local copy. |
| `host port 8090 is already in use` | Something else listens on 8090 | `curl … \| PULSE_HOST_PORT=18090 bash -s -- …` |
| `Pulse did not report healthy within 90s` | ClickHouse slow to start, or low memory | Check `docker compose … logs`; make sure the host has ≥ 2 GB RAM free. Re-running is safe. |
| Exit code `2`, warning "collector cannot reach AMS" | Pulse is installed but cannot read AMS | Do **not** re-run the installer. Fix `quickstart/.env` (see next section), then `docker compose … up -d`. |

## Dashboard shows no streams / collector `degraded`

| Message in `/healthz` or logs | Cause | Fix |
|---|---|---|
| `connection refused`, `no route to host`, timeout | Wrong `PULSE_AMS_URL`, AMS not listening on that address, or a firewall | From the Pulse host: `curl -s http://<ams>:5080/rest/v2/version`. In Docker, `localhost` is the container itself — use the host's IP. |
| `HTTP 403` for an application | The application's REST IP filter (`remoteAllowedCIDR`) does not include Pulse's IP (AMS default: `127.0.0.1`) | In the AMS console, add the Pulse host's IP or subnet for **each** application. Applications that still answer 403 are skipped silently. |
| `HTTP 401` / login failed | Wrong AMS e-mail or password | Fix the credentials in `quickstart/.env`. Pulse backs off automatically (1 → 2 → 4 → 8 → 15 min) so it no longer keeps the AMS account locked. After fixing, Pulse recovers within the current backoff (at most 15 min) or at once after restarting the container. Use a dedicated AMS account. |
| Some applications missing | `PULSE_AMS_APPLICATIONS` lists the wrong names | Leave it empty to monitor every application. |
| Settings → Sources says "No AMS sources configured" | Normal for installs configured through environment variables | Not an error — that tab lists only sources added in the UI. |

## Dashboard says "Polling" instead of "Live"

The live WebSocket (`/api/v1/live/ws`) is not getting through. Behind a reverse proxy, forward
the `Upgrade` and `Connection` headers for that path. If the UI is served from another
origin, add it to `PULSE_ALLOWED_WS_ORIGINS`. Other pages (Alerts, Settings, …) always show
"Polling"; only the live dashboard uses the WebSocket.

## No alert notifications

1. **The rule has no channel, or is muted** — the default rules ship both. Edit the rule, tick
   a channel under **Notify channels**, uncheck **Muted**. The Rules list shows each rule's
   channels; a rule with **No channel** records history but notifies no one.
2. **E-mail channel without an SMTP server** — set the SMTP server (`host:port`), sender and,
   if needed, user and password in the channel form; without them Pulse tries `localhost:587`.
3. **Maintenance window or cooldown** active for the rule.
4. Use **Test fire** on the channel to separate delivery problems from rule problems.

## Viewer QoE / beacon data missing

| Cause | Fix |
|---|---|
| Wrong token | Players need an **ingest** token (Settings → Ingest Tokens), not an API token. |
| Endpoint not reachable from viewers | Expose `/ingest/beacon` (port 8090, or 8091 with `PULSE_INGEST_LISTEN_ADDR`) through your TLS proxy; pages served over HTTPS need an HTTPS beacon URL. |
| SDK not in the player | Add the Pulse beacon SDK (`docs/beacon-sdk.md`). REST polling alone gives live counts, not per-viewer QoE or audience history. |

## Analytics page shows zero views while streams are live

Known defect found in the 2026-10-01 audit (submission notes §4, D1): the audience summary
query fails and the page shows zeros. Not a configuration problem on your side.

## License key message

Pulse is free and no license key is needed. If you pass a legacy key, Pulse still verifies
it (offline) for compatibility but it does not change behavior. **Settings → License** shows
"Pulse is free — every feature is included, with no node or retention limits. No license key
is needed."

## Lost the admin token

There is no reset command. Create a new token with another admin token
(`POST /api/v1/admin/tokens`), or on a fresh install remove the `pulse-data` volume and re-run
the installer (this deletes users, tokens and alert configuration).

## Disk usage grows

Raw events are kept for `PULSE_RETENTION_DAYS` (default 90) and rollups for
`PULSE_ROLLUP_TTL_DAYS` (default 395). Lower them and restart; ClickHouse deletes expired rows
in the background.

## Getting help

- GitHub Issues: https://github.com/aytekXR/ams-pulse/issues (include `/healthz` output and the
  last 100 log lines, with tokens removed)
- E-mail: support@beyondkaira.com
- Security issues: aytek@beyondkaira.com — not a public issue
