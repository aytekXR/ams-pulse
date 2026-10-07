# Pulse — Installation Guide

For Ant Media Server operators. Covers the one-command quickstart (recommended), a manual
Docker Compose install, Kubernetes (experimental), offline installs, and what to change
before exposing Pulse beyond a private network.

**Release covered:** Pulse **v0.5.0** — image `ghcr.io/aytekxr/ams-pulse:0.5.0` (public,
signed), the Helm chart published with v0.5.0. **Verified:** the quickstart below was run end to end on
2026-10-01 as a fresh install (no previous Pulse on the host; installer exit code `0`, 73 seconds).

---

## 1. Before you start

**On the Pulse host** — see [`system-requirements.md`](system-requirements.md):

- Linux with **Docker Engine 24+** and **Docker Compose v2** (`docker compose version` works).
- 2 vCPU / 2 GB RAM minimum (4 GB recommended above ~100 concurrent streams).
- Outbound HTTPS to `ghcr.io` and `raw.githubusercontent.com` (or use the offline path, §5).
- Port **8090** free (or choose another with `PULSE_HOST_PORT`).

**On the Ant Media Server side** — Pulse changes nothing on AMS, but AMS has to let it in:

1. **Network path.** The Pulse host must reach the AMS REST API — normally
   `http://<ams-host>:5080` (or `https://…:5443`).
2. **An AMS login.** AMS 3.x Enterprise uses its management-console login: give Pulse an admin
   **e-mail and password**. Use a dedicated account — AMS locks an account for 5 minutes after
   two failed logins, and a shared account would lock out humans too.
   *(AMS with JWT REST security instead: supply a bearer token as `PULSE_AMS_AUTH_TOKEN`.)*
3. **The REST IP filter.** Each AMS application restricts its REST API to `127.0.0.1` by
   default (`remoteAllowedCIDR`). For every application Pulse should monitor, add the Pulse
   host's IP (or its subnet) in the AMS console. An application that still blocks Pulse is
   silently left out of monitoring, and Pulse logs an HTTP 403 for it on every poll.

## 2. Quickstart — one command (recommended)

```sh
curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/install.sh \
  | bash -s -- --ams-url http://YOUR-AMS:5080 \
               --email YOUR-AMS-ADMIN-EMAIL \
               --password 'YOUR-AMS-PASSWORD'
```

- **All three flags are required.** When the script is piped into `bash` it cannot prompt,
  and it stops with `Error: --password is required (no TTY available …)` if one is missing.
- Use another host port with `curl … | PULSE_HOST_PORT=18090 bash -s -- …` (the variable must be set for `bash`, not `curl`).

What it does, in order: checks Docker and Compose → downloads the compose file pinned to
v0.5.0 → pulls the image (falls back to a local copy if the registry is unreachable) →
writes `quickstart/.env` (mode 600, contains your AMS login, a generated secret key and a
generated Prometheus token) → starts **ClickHouse**, a one-shot **schema migration**, and
**Pulse** → waits up to 90 s for `/healthz` → watches for up to 40 s that Pulse can actually
reach AMS → prints a **one-time admin token** and a note that every feature is included.

| Exit code | Meaning | What to do |
|---|---|---|
| `0` | Installed, and Pulse reached AMS. | Open the dashboard (§3). |
| `2` | Installed and running, but Pulse **cannot reach AMS** (URL, login or REST filter). | Do not re-run the installer. Fix `quickstart/.env`, then `docker compose -f quickstart/docker-compose.quickstart.yml --env-file quickstart/.env up -d`. |
| `1` | Hard failure (Docker missing, image unavailable, health timeout). | Read the message; nothing is left half-configured. |

![Installer output](../assets/walkthrough/step-1-install-output.png)

## 3. Open the dashboard

Open **`http://<your-server>:8090`** and paste the admin token (`plt_…`) into the sign-in
screen. The token is printed **once**; store it in a password manager. The live dashboard
opens straight away, and streams appear within seconds of the first poll. The green
**Live** badge (top right) means the dashboard is receiving live updates.

![Sign-in](../assets/walkthrough/step-4a-sign-in.png)

![First-run dashboard](../assets/walkthrough/step-4b-dashboard-first-run.png)

More on access, ports, HTTPS and users: [`dashboard-access.md`](dashboard-access.md).

## 4. Manual Docker Compose install

Same stack, without the script:

```sh
mkdir quickstart && cd quickstart
curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/docker-compose.quickstart.yml \
  -o docker-compose.quickstart.yml
curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/.env.example \
  -o .env && chmod 600 .env
# Edit .env: PULSE_AMS_URL, PULSE_AMS_LOGIN_EMAIL, PULSE_AMS_LOGIN_PASSWORD,
# PULSE_SECRET_KEY (openssl rand -hex 32), optionally PULSE_METRICS_TOKEN / PULSE_HOST_PORT.
docker compose -f docker-compose.quickstart.yml --env-file .env up -d
docker compose -f docker-compose.quickstart.yml --env-file .env logs pulse | grep 'FIRST RUN'
```

Keep `PULSE_SECRET_KEY` safe and unchanged: it encrypts the credentials Pulse stores
(alert-channel secrets, AMS source logins). Losing it makes them unreadable.

## 5. Offline / air-gapped hosts

On a machine with internet access: `docker pull ghcr.io/aytekxr/ams-pulse:0.5.0` and
`docker pull clickhouse/clickhouse-server@sha256:1d1f6508eba2dccce2cee9913907c5f7766327debc57a6b1991f2c9e3176c163`,
then `docker save` both, copy, and `docker load` them on the target. Copy
`deploy/quickstart/` (compose file + `install.sh`) alongside, and run `bash install.sh …` from
that directory: the installer uses the co-located compose file, and when the registry pull
fails it uses the image already loaded on the host.

> The local-image fallback is in `install.sh` on `main` from 2026-10-01. On older copies of the
> script, use the manual install (§4) instead.

No license key is needed — Pulse is free and every feature is included.

## 6. Kubernetes with Helm — experimental

> **Status: experimental.** The chart renders and lints cleanly and is published, but it has
> **not yet been deployed to a real cluster**. Prefer Docker Compose for production today.

```sh
# 1. Secrets (never put them in values.yaml)
kubectl create secret generic pulse-secrets \
  --from-literal=PULSE_SECRET_KEY="$(openssl rand -hex 32)" \
  --from-literal=PULSE_AMS_LOGIN_EMAIL='YOUR-AMS-ADMIN-EMAIL' \
  --from-literal=PULSE_AMS_LOGIN_PASSWORD='YOUR-AMS-PASSWORD'
CH_PASS="$(openssl rand -hex 16)"
kubectl create secret generic pulse-clickhouse-secret \
  --from-literal=CLICKHOUSE_USER=pulse \
  --from-literal=CLICKHOUSE_PASSWORD="$CH_PASS" \
  --from-literal=PULSE_CLICKHOUSE_DSN="clickhouse://pulse:${CH_PASS}@pulse-clickhouse:9000/pulse"

# 2. values.yaml
cat > pulse-values.yaml <<'YAML'
pulse:
  ams:
    url: "http://your-ams:5080"
    nodeId: "ams-node-1"
  secretRef:
    name: pulse-secrets
  extraEnv:   # AMS 3.x console login (the chart wires a bearer token natively, not a login)
    - name: PULSE_AMS_LOGIN_EMAIL
      valueFrom: { secretKeyRef: { name: pulse-secrets, key: PULSE_AMS_LOGIN_EMAIL } }
    - name: PULSE_AMS_LOGIN_PASSWORD
      valueFrom: { secretKeyRef: { name: pulse-secrets, key: PULSE_AMS_LOGIN_PASSWORD } }
clickhouse:
  auth:
    existingSecret: pulse-clickhouse-secret
YAML

# 3. Install, then create the schema (the server does not create ClickHouse tables itself)
helm install pulse oci://ghcr.io/aytekxr/charts/pulse -f pulse-values.yaml
kubectl exec deploy/pulse -- pulse migrate

# 4. Reach the UI
kubectl port-forward svc/pulse 8090:8090     # then open http://localhost:8090
kubectl logs deploy/pulse | grep 'FIRST RUN'  # one-time admin token
```

These values were rendered with `helm template` and `helm lint` (Helm 3.17.0) on 2026-10-01.
The chart also exposes the player-beacon listener on **8091** (`pulse-ingest` service) for
an internet-facing ingress. Chart reference: `deploy/helm/pulse/README.md`.

## 7. Before going beyond a private network

- **TLS.** The quickstart publishes port 8090 over plain HTTP on all interfaces, and Docker's
  port publishing bypasses host firewalls such as `ufw`. On a public server, publish it on
  `127.0.0.1` only and front it with a TLS reverse proxy (reference nginx vhosts:
  `deploy/nginx/`; production compose: `deploy/docker-compose.prod.yml`).
- **`PULSE_BASE_URL`** — set it to the public URL (e.g. `https://pulse.example.com`).
- **`PULSE_METRICS_TOKEN`** — set it, or `/metrics` is served without authentication.
- **AMS over HTTPS** if Pulse and AMS are on different hosts: the AMS login travels with every
  session (Pulse warns at startup when the AMS URL is plain HTTP on a non-local host).
- **Backups** — `deploy/docker-compose.backup.yml` adds a daily ClickHouse + meta-store backup.

## 8. Upgrade and uninstall (quickstart)

- **Upgrade:** add `PULSE_IMAGE=ghcr.io/aytekxr/ams-pulse:<new-version>` to `quickstart/.env`
  and run `docker compose -f quickstart/docker-compose.quickstart.yml --env-file quickstart/.env up -d`.
  The migration container applies any new schema from the new image before Pulse restarts.
  Back up the `pulse-quickstart_pulse-data` and `pulse-quickstart_clickhouse-data` volumes first.
- **Stop:** `docker compose -f quickstart/docker-compose.quickstart.yml --env-file quickstart/.env down`.
- **Remove everything, including data:** add `-v` to the command above (irreversible; deletes
  all metrics, users, tokens and alert configuration).

## 9. Verify the install

```sh
curl -s http://localhost:8090/healthz
```

Expect `"status":"ok"` **and** `"collector":{…"status":"ok"}`. Read the `collector` entry
specifically: other components can be `ok` while the collector is `degraded`, which means
Pulse cannot reach AMS. Fixes: [`troubleshooting.md`](troubleshooting.md).
