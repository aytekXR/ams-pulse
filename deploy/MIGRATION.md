# Pulse — Caddy → nginx edge migration runbook

> **STATUS: DONE (2026-07).** The cutover below has been executed: host nginx
> owns `:80/:443` for every site on the VPS, the shared Caddy container (and its
> images/volumes) has been deleted, and the repo's Caddy artifacts
> (`deploy/config/Caddyfile{,.prod}`, `deploy/docker-compose.prod-tls.yml`, the
> `caddy` service in the hardened overlay) have been removed. The prod compose
> stack is consolidated in **`deploy/docker-compose.prod.yml`** (used by
> `deploy/nginx/deployment.sh`). TLS: cert at
> `/etc/letsencrypt/live/beyondkaira.com/`, renewed by certbot; vhosts in
> `deploy/nginx/`. The Caddy-era steps and rollback below are kept as a record
> of how the migration was done — the rollback path no longer exists (the Caddy
> container and config are gone).

Move the Pulse edge from the **shared containerised Caddy** to **nginx on the
host**, with one `server` file per subdomain, the **wildcard** TLS cert, and the
app reachable only over a **private loopback** bind. **Edge-only:** Pulse stays a
docker-compose stack (Go server + ClickHouse + Kafka); this changes *what
terminates TLS and how the edge reaches the app*, nothing about how Pulse is
built or supervised.

Legend: 👤 = **owner**, needs `sudo` / DNS / a maintenance window · 🤖 =
committed artifact in this repo, no sudo. This whole doc is additive: it adds
files and does **not** modify a live Caddy snippet, compose file, or unit. The
live site keeps working on Caddy until step 6, and step 6 is reversible.

- **Host:** `YOUR-SERVER-IP` = `beyondkaira.com`, user `YOUR-USER`, Docker + systemd.
- **Subdomains this repo owns** (from `deploy/config/Caddyfile.prod`):
  `beyondkaira.com` (apex) + `pulse.beyondkaira.com` → the Pulse app;
  `ams.beyondkaira.com` → the Ant Media panel; `www.beyondkaira.com` → 301 apex.
  The `yanki.` / `bedirhandemirel.` / `matbu.` blocks in that same Caddyfile
  belong to **other repos** and are migrated from *their* repos — leave them out
  of this cutover.

---

## What was true before (Caddy), and what changes

| | Old (shared Caddy container) | New (host nginx) |
|---|---|---|
| Edge process | `pulse-prod-caddy-1` container, binds `0.0.0.0:80/443` | nginx on the host, `:80/:443` |
| Reaches the app by | Docker network alias `pulse:8090/8091/8092` | `127.0.0.1:8090/8091/8092` (loopback publish) |
| App exposure | ports internal to the docker network | ports bound to `127.0.0.1` only — never public |
| TLS | Caddy auto Let's Encrypt per name | one wildcard `*.beyondkaira.com` (+ apex), DNS-01 |
| Add a site | edit the one shared Caddyfile, `caddy reload` | drop a `.conf`, `nginx -t && reload` |

The app's listeners are unchanged: `PULSE_LISTEN_ADDR=:8090` (UI+API+`/healthz`),
`PULSE_INGEST_LISTEN_ADDR=:8091` (beacon), `PULSE_WEBHOOK_ADDR=:8092` (webhook).
Only their *host publishing* changes, via the additive overlay below.

---

## Committed artifacts (🤖, already in this branch)

| File | Role |
|---|---|
| `deploy/nginx/pulse.beyondkaira.com.conf` | apex + `pulse.` server block; `/beacon/*`→8091 (prefix stripped), `/webhook/*`→8092, everything else→8090 |
| `deploy/nginx/ams.beyondkaira.com.conf` | `ams.` → `127.0.0.1:5080` (AMS `--network host`) |
| `deploy/nginx/www.beyondkaira.com.conf` | `www.` → 301 apex |
| `deploy/nginx/00-beyondkaira-maps.conf` | shared `$connection_upgrade` map (WebSocket), installed once into `conf.d/` |
| `deploy/docker-compose.prod.yml` | consolidated prod stack (base + hardened + nginx-edge in one file) — what `deployment.sh` runs |
| `deploy/docker-compose.nginx-edge.yml` | additive overlay: publishes 8090/8091/8092 on `127.0.0.1` so host-nginx can reach them (folded into `prod.yml`; kept for layer-by-layer composition) |
| `deploy/nginx/deployment.sh` | self-contained build→up→loopback-health for the compose stack |

Routing was transcribed from the Caddyfile.prod `{$PULSE_DOMAIN}, pulse.{$PULSE_DOMAIN}`
and `ams.{$PULSE_DOMAIN}` blocks. The `/beacon/*` strip is reproduced by the
trailing slash on `location /beacon/` + `proxy_pass .../` (Caddy `handle_path`);
`/webhook/*` is passed through unstripped (Caddy plain `handle`).

---

## 0. 👤 Prerequisites (once per host — likely already done for other sites)

- DNS `A` records → `YOUR-SERVER-IP`, propagated, for each name you are cutting
  over: `beyondkaira.com`, `www`, `pulse`, `ams`. A wildcard `*.beyondkaira.com`
  A record covers the subdomains.
  ```bash
  for n in beyondkaira.com www.beyondkaira.com pulse.beyondkaira.com ams.beyondkaira.com; do
    printf '%s -> ' "$n"; dig @8.8.8.8 +short "$n"; done   # each must print YOUR-SERVER-IP
  ```
- nginx installed and enabled: `sudo apt-get install -y nginx && sudo systemctl enable --now nginx`.
- **Wildcard cert present** at `/etc/letsencrypt/live/beyondkaira.com/{fullchain,privkey}.pem`,
  issued via certbot DNS-01 for `-d '*.beyondkaira.com' -d beyondkaira.com`
  (the apex needs the explicit second `-d`; a wildcard alone does NOT cover it).
  `certbot renew`'s systemd timer keeps it fresh — no per-site cert step.

## 1. 🤖 Bring the Pulse stack up with the loopback overlay (no edge change yet)

This publishes the app on `127.0.0.1`. It does not touch `:443`.

```bash
cd ~/repo/ams-pulse
# from the deploy script (recommended — it health-gates the private port):
DOCKER_SG=1 ./deploy/nginx/deployment.sh --check      # validates the config, changes nothing
DOCKER_SG=1 ./deploy/nginx/deployment.sh              # build -> up -> assert /healthz on 127.0.0.1:8090

# or by hand (single consolidated prod file):
sg docker -c 'docker compose -p pulse-prod \
  -f deploy/docker-compose.prod.yml \
  --env-file deploy/.env up -d --build'

curl -fsS http://127.0.0.1:8090/healthz | grep -q '"components"' && echo "app OK on loopback"
```

## 2. 👤 Install the shared WebSocket map (once per host)

```bash
sudo cp deploy/nginx/00-beyondkaira-maps.conf /etc/nginx/conf.d/00-beyondkaira-maps.conf
```

## 3. 👤 Install the Pulse / AMS / www nginx sites

```bash
cd ~/repo/ams-pulse
for c in pulse.beyondkaira.com ams.beyondkaira.com www.beyondkaira.com; do
  sudo cp deploy/nginx/$c.conf /etc/nginx/sites-available/$c.conf
  sudo ln -sfn /etc/nginx/sites-available/$c.conf /etc/nginx/sites-enabled/$c.conf
done
sudo nginx -t     # MUST pass before you go near :443
```

`nginx -t` will FAIL if the wildcard cert is missing (step 0) — fix that first;
a failed `-t` means nginx never loads the bad config, so nothing is at risk yet.

> **Note on `:443` before the cutover.** nginx and Caddy cannot both own `:443`.
> Until step 4, the shared Caddy still holds it, so `systemctl reload nginx` here
> will log "address already in use" for the `:443` listeners — that is expected;
> nginx keeps serving whatever it already had and Caddy stays authoritative. The
> real swap is step 4.

## 4. 👤 The cutover (the one risky step — windowed, reversible)

Do this in a maintenance window. Stopping the shared Caddy frees `:80/:443` and
takes **every** site it fronts offline for the seconds until nginx takes over —
so cut over only when the nginx blocks for all the sites you care about are
staged and `nginx -t` is green. (If other repos' sites — yanki/bedo/matbu — are
still Caddy-only, coordinate: either migrate them first, or accept they go down
with Caddy. This repo owns pulse/ams/www; the rest are their repos' cutover.)

```bash
# 1. Prove the app answers privately (again) right before the flip:
curl -fsS http://127.0.0.1:8090/healthz | grep -q '"components"' && echo "app ready"

# 2. Free :443 by stopping the shared Caddy, then start nginx on it:
sudo docker stop pulse-prod-caddy-1
sudo systemctl reload nginx     # or `restart` if reload can't grab :443 cleanly

# 3. Verify each site over the PUBLIC edge (real cert, no -k):
curl -fsS https://pulse.beyondkaira.com/healthz | grep -q '"components"' && echo "pulse OK"
curl -fsS -o /dev/null -w '%{http_code}\n' https://beyondkaira.com/            # 200
curl -fsS -o /dev/null -w '%{http_code}\n' https://ams.beyondkaira.com/        # AMS panel (200/302)
curl -sS  -o /dev/null -w '%{http_code}\n' -I https://www.beyondkaira.com/     # 301 -> apex
# beacon strip check (SDK posts to /beacon/ingest/beacon):
curl -sS -o /dev/null -w '%{http_code}\n' -X POST https://pulse.beyondkaira.com/beacon/ingest/beacon
```

### 4b. 👤 ROLLBACK (if any verify fails) — back to Caddy, unchanged

```bash
sudo systemctl stop nginx          # release :443 (or remove the :443 listen and reload)
sudo docker start pulse-prod-caddy-1   # Caddy retakes :443 with its unchanged config
curl -fsS https://pulse.beyondkaira.com/healthz    # confirm the old edge is back
```

At migration time nothing had deleted the Caddyfile or the prod-tls overlay, so
Caddy came back exactly as it was. (**No longer true post-cleanup:** the Caddy
container, its volumes and the repo's Caddy config have since been deleted —
this rollback is historical record only.) The loopback overlay from step 1 can
stay up (it only publishes private ports) or be dropped with `compose down`.

## 5. 👤 Persist (after a stable soak)

Once nginx has served all sites cleanly for a soak period, keep the shared Caddy
container stopped (do not `rm` it until every repo's site is proven on nginx).
`systemctl enable nginx` is already on from step 0, so nginx survives reboot; the
Pulse compose stack has `restart: unless-stopped`.

---

## Environment keys (no new secrets introduced)

This edge migration adds **no** new secrets. The overlay reuses the existing
`deploy/.env` consumed by the base + hardened stack. Keys already documented in
`deploy/.env.example` and required for a prod bring-up:

| Key | Purpose | Notes |
|---|---|---|
| `PULSE_SECRET_KEY` | AES-GCM token encryption | `openssl rand -hex 32`; required in prod |
| `CLICKHOUSE_USER` / `CLICKHOUSE_PASSWORD` | ClickHouse auth | alphanumeric + `_` |
| `PULSE_WEBHOOK_SECRET` | HMAC for AMS webhooks | required for the `:8092` listener to bind (fail-closed) |

nginx does **not** read a domain from env — the FQDNs are hard-set in each
`deploy/nginx/*.conf` (`PULSE_DOMAIN` was Caddy-only and has been removed).
`deploy/.env` stays gitignored — never commit real values. The AMS/S3/metrics
keys are only needed by other overlays (real-ams) and are out of scope for the
nginx edge.

## Deployment script config (`deploy/nginx/deployment.sh`)

Env-driven, never prints values. Defaults target this stack:

| Var | Default | Meaning |
|---|---|---|
| `PULSE_PROJECT` | `pulse-prod` | compose project name |
| `PULSE_ENV_FILE` | `<repo>/deploy/.env` | `--env-file` (gitignored) |
| `HEALTH_URL` | `http://127.0.0.1:8090/healthz` | **private** loopback probe |
| `HEALTH_EXPECT` | `components` | substring the body must contain (Pulse `/healthz` marker) |
| `HEALTH_TIMEOUT` | `90` | seconds before hard-fail |
| `DOCKER_SG` | `0` | `1` wraps compose in `sg docker -c` on a host without the docker group |

The health gate asserts HTTP 200 **and** the `"components"` object that only
Pulse's `/healthz` emits — a 200 from something else does not pass. Note
`/healthz` returns 200 for both `"status":"ok"` and `"status":"degraded"` (only a
hard `down` is 503), so the gate deliberately checks the marker, not the status.
