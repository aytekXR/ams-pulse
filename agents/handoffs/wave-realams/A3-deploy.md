# A3 — deploy overrides for real-AMS (login auth + isolated test stack)

**Scope (single writer):** `deploy/docker-compose.real-ams.yml` (edit) and
`deploy/docker-compose.realams-test.yml` (new). Do NOT touch `deploy/.env` (ORCH owns it — it is
gitignored and holds the live password). **Author only — do NOT `git add`/commit.**

Read first: `deploy/docker-compose.real-ams.yml`, `deploy/docker-compose.ci.yml` (your model for the
test stack), and `deploy/docker-compose.yml` (base — note `pulse` uses `expose:`, build context `..`,
Dockerfile `deploy/docker/pulse.Dockerfile`).

## 1. Edit `deploy/docker-compose.real-ams.yml`

Auth is now cookie-session login (email/password), so the bearer token is **optional**, not required.
- Change the required token line `PULSE_AMS_AUTH_TOKEN: "${PULSE_AMS_AUTH_TOKEN:?...}"` to optional:
  `PULSE_AMS_AUTH_TOKEN: "${PULSE_AMS_AUTH_TOKEN:-}"`.
- Add login-credential passthrough in the same `pulse.environment` block:
  ```yaml
  PULSE_AMS_LOGIN_EMAIL: "${PULSE_AMS_LOGIN_EMAIL:-}"
  PULSE_AMS_LOGIN_PASSWORD: "${PULSE_AMS_LOGIN_PASSWORD:-}"
  ```
- Update the header comment block: AMS Enterprise uses cookie-session auth; set
  `PULSE_AMS_LOGIN_EMAIL` + `PULSE_AMS_LOGIN_PASSWORD` (token optional). Keep the existing
  `PULSE_AMS_URL`/`PULSE_AMS_NODE_ID`/`PULSE_AMS_APPLICATIONS` docs.

## 2. New `deploy/docker-compose.realams-test.yml`

An **isolated validation stack** so ORCH can curl Pulse against the real AMS on **loopback ports**
without touching `pulse-prod` (the live Oğuz demo) or its TLS/Caddy. It is combined as:

```
docker compose -p pulse-realams \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.real-ams.yml \      # AMS connection env + disables mock-ams
  -f deploy/docker-compose.realams-test.yml \   # infra: loopback ports, migrations, secret
  --env-file deploy/.env up -d --build
```

So this file provides only **infra** (the real-ams override already supplies AMS env + disables
mock-ams). Model it on `docker-compose.ci.yml`:

```yaml
# Pulse real-AMS ISOLATED TEST stack (loopback only) — pairs with docker-compose.real-ams.yml.
# Project: pulse-realams. Lets ORCH validate against a real AMS without disturbing pulse-prod.
services:
  clickhouse:
    environment:
      CLICKHOUSE_SKIP_USER_SETUP: "1"   # isolated, no auth — separate volume per project

  pulse-migrate:
    build:
      context: ..
      dockerfile: deploy/docker/pulse.Dockerfile
    entrypoint: ["pulse", "migrate"]
    environment:
      PULSE_CLICKHOUSE_DSN: "clickhouse://clickhouse:9000/pulse"
      PULSE_CLICKHOUSE_DATABASE: "pulse"
      PULSE_MIGRATIONS_DIR: "/contracts/db/clickhouse"
      PULSE_META_DSN: "/tmp/pulse_meta.db"
    volumes:
      - ../contracts:/contracts:ro
    depends_on:
      clickhouse:
        condition: service_healthy
    restart: "no"

  pulse:
    ports:
      - "127.0.0.1:18090:8090"   # UI + API — loopback only (pulse-prod owns :80/:443)
      - "127.0.0.1:18091:8091"   # beacon ingest — loopback only
    environment:
      PULSE_MIGRATIONS_DIR: "/contracts/db/clickhouse"
      PULSE_META_DDL_PATH: "/contracts/db/meta/0001_init.sql"
      PULSE_SECRET_KEY: "${PULSE_SECRET_KEY:?set PULSE_SECRET_KEY}"
    volumes:
      - ../contracts:/contracts:ro
    depends_on:
      clickhouse:
        condition: service_healthy
```

Match the exact service/volume names + healthcheck conventions used in `docker-compose.ci.yml` and the
base file (e.g. if base `pulse` `depends_on` `pulse-migrate`, preserve that). Do NOT add a `mock-ams`
block here — the real-ams override disables it.

## 3. Validate compose merges (config -q)

Self-check both stacks parse (placeholder env so it works without the real `.env`):
```
cd /home/aytek/repo/ams-pulse
PULSE_AMS_URL=https://ams PULSE_AMS_LOGIN_EMAIL=a@b.c PULSE_AMS_LOGIN_PASSWORD=x \
PULSE_AMS_NODE_ID=t PULSE_SECRET_KEY=deadbeef CLICKHOUSE_USER=p CLICKHOUSE_PASSWORD=p \
sg docker -c "docker compose -p pulse-realams-cfgcheck \
  -f deploy/docker-compose.yml -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.realams-test.yml config -q" && echo "MERGE OK"
```
Also confirm the existing prod stack still parses:
```
PULSE_DOMAIN=x CLICKHOUSE_USER=p CLICKHOUSE_PASSWORD=p PULSE_SECRET_KEY=x \
sg docker -c "docker compose -p cfgcheck2 -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml \
  -f deploy/docker-compose.real-ams.yml config -q" && echo "PROD MERGE OK"
```

Return: the two file diffs (summary) and the `config -q` results (MERGE OK / PROD MERGE OK).
