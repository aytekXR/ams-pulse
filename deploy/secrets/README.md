# deploy/secrets/ — Docker Compose file-based secrets

This directory holds **plaintext secret files** used by the opt-in
`docker-compose.secrets.yml` overlay.  It is **gitignored** — never commit
files from here.

## One-time setup

```bash
mkdir -p deploy/secrets
chmod 700 deploy/secrets

# Required secrets
openssl rand -hex 32 > deploy/secrets/pulse_secret_key.txt
openssl rand -hex 32 > deploy/secrets/pulse_webhook_secret.txt

# Optional — create as empty files if not used
touch deploy/secrets/pulse_ams_login_password.txt
touch deploy/secrets/pulse_metrics_token.txt

chmod 600 deploy/secrets/*.txt
```

## File → environment variable mapping

| File                              | Variable (via _FILE)            | Used in         |
|-----------------------------------|---------------------------------|-----------------|
| `pulse_secret_key.txt`            | `PULSE_SECRET_KEY`              | AES-GCM token encryption |
| `pulse_webhook_secret.txt`        | `PULSE_WEBHOOK_SECRET`          | AMS webhook HMAC |
| `pulse_ams_login_password.txt`    | `PULSE_AMS_LOGIN_PASSWORD`      | AMS cookie-session auth |
| `pulse_metrics_token.txt`         | `PULSE_METRICS_TOKEN`           | Prometheus scrape auth |

## Opt-in usage

Add `docker-compose.secrets.yml` AFTER the hardened overlay:

```bash
docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.prod-tls.yml \
  -f deploy/docker-compose.secrets.yml \
  up -d
```

The secrets overlay overrides `PULSE_SECRET_KEY` and `PULSE_WEBHOOK_SECRET`
to `""` so the hardened overlay's `:?` guards do not fail, while the actual
values come from the mounted secret files via the `_FILE` env vars.

## Rotating a secret

```bash
# 1. Write the new value
openssl rand -hex 32 > deploy/secrets/pulse_secret_key.txt

# 2. Restart the pulse container to pick it up
docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.prod-tls.yml \
  -f deploy/docker-compose.secrets.yml \
  up -d pulse
```
