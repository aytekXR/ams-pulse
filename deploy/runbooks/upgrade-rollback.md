# Pulse Upgrade & Rollback Runbook

**Target:** `pulse-prod` stack on `beyondkaira.com` (VPS `161.97.172.146`).
**Authored:** 2026-07-09 (D-062 SESSION-06).
**Scope:** Docker Compose prod stack; Go binary only. Helm path deferred (S6).

---

## Canonical 5-overlay compose command

All production operations use this exact overlay set. Copy it as-is; never omit an overlay.

```sh
# Run from the repo root. sg docker is required — docker group is stale in non-login shells.
DC_ARGS="-p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.prod-tls.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env"

# Validate config before any destructive operation:
sg docker -c "docker compose ${DC_ARGS} config -q" && echo CONFIG_OK
```

Overlay purpose:
- `docker-compose.yml` — base: pulse + ClickHouse (expose-only ports, no host bindings)
- `docker-compose.hardened.yml` — Caddy TLS proxy, CH auth, resource limits, webhook listener
- `docker-compose.prod-tls.yml` — public `0.0.0.0:80/443`, `Caddyfile.prod`, `PULSE_DOMAIN` required
- `docker-compose.real-ams.yml` — disables mock-ams, wires `PULSE_AMS_*` from env
- `docker-compose.backup.yml` — backup sidecar (24 h cycles, 7-artifact retention, CH zip + SQLite)

---

## Pre-upgrade steps (do these before every deploy)

### Step 1 — Tag the current image as a rollback point

Use the D-number of the change being rolled out as the tag suffix. This is the rollback target
if the new deploy fails.

```sh
# Example: rolling out D-063
sg docker -c "docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-d063"
sg docker -c "docker images pulse-prod-pulse"   # confirm the new tag
```

**Tags that exist today and the commits they map to (refreshed D-065):**

| Tag | `pulse version` output | Built |
|---|---|---|
| `latest` | `pulse v0.1.0-50-g5d77a05 (commit 5d77a05, built 2026-07-09T13:23:47Z)` | 2026-07-09 |
| `pre-d064` | `pulse v0.1.0-25-gbc15d43 (commit bc15d43, built 2026-07-09T00:56:07Z)` | 2026-07-09 |
| `pre-d061` | `pulse 1a701d6 (commit 1a701d6, built 2026-07-08T01:51:17Z)` | 2026-07-08 |
| `pre-d058` | `pulse dev (commit unknown, built unknown)` | 2026-07-07 |
| `prev` | `pulse dev (commit unknown, built unknown)` | 2026-06-15 |

Verified with: `sg docker -c "docker run --rm --entrypoint pulse pulse-prod-pulse:<tag> version"`

### Step 2 — Take a manual backup before upgrading

```sh
sg docker -c "docker compose ${DC_ARGS} exec backup /scripts/pulse-backup.sh once"
# Exit code 0 = success. Non-zero = at least one store failed; do NOT proceed.
```

See `deploy/runbooks/backup-restore.md` for restore instructions if the upgrade goes wrong.

---

## Stamped-build upgrade procedure

> **Why two steps (build then up)?**
> `docker compose up -d --build` does NOT forward `--build-arg` values, so the binary
> ends up stamped `dev/unknown`. Instead, run `compose build` with explicit build-args first,
> then `compose up -d` WITHOUT `--build` — the pre-built image is reused. (D-058 lesson b.)

### Step 3 — Build the new image with version stamps

```sh
sg docker -c "docker compose ${DC_ARGS} build \
  --build-arg VERSION=$(git describe --tags --always) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  pulse"
```

### Step 4 — Assert the stamp before deploying

```sh
sg docker -c "docker run --rm --entrypoint pulse pulse-prod-pulse:latest version"
# Must NOT print 'dev' or 'unknown'. Example good output:
#   pulse v0.1.0-3-gabc1234 (commit abc1234, built 2026-07-10T14:00:00Z)
```

If the stamp is `dev/unknown`, re-run Step 3. Do not proceed with a dev-stamped image.

### Step 5 — Deploy (no --build)

```sh
sg docker -c "docker compose ${DC_ARGS} up -d"
```

Compose will recreate only containers whose image or config changed. The `pulse-migrate`
one-shot runs automatically before `pulse` starts (see `depends_on` chain in hardened overlay).

### Step 6 — Post-swap smoke checklist

```sh
# Health endpoint (use --resolve because VPS local DNS may be stale):
curl -sf --max-time 10 \
  --resolve beyondkaira.com:443:161.97.172.146 \
  https://beyondkaira.com/healthz
# Expected: {"status":"ok","components":{...}}

# Confirm version in running container:
sg docker -c "docker compose ${DC_ARGS} exec pulse /usr/local/bin/pulse version"

# Webhook signature check (replace $PULSE_WEBHOOK_SECRET from deploy/.env):
BODY='{"action":"liveStreamStarted","streamId":"smoke","app":"LiveApp"}'
SIG="sha256=$(echo -n "${BODY}" | openssl dgst -sha256 -hmac "${PULSE_WEBHOOK_SECRET}" -hex | sed 's/.* //')"
curl -sf -X POST \
  --resolve beyondkaira.com:443:161.97.172.146 \
  https://beyondkaira.com/webhook/ams \
  -H "Content-Type: application/json" \
  -H "X-Ams-Signature: ${SIG}" \
  -d "${BODY}"
# Expected HTTP 200

# Resource limits (inspect, not trust compose YAML).
# NOTE: the CONTAINER is pulse-prod-pulse-1 (compose v2 naming); pulse-prod-pulse
# is the IMAGE name — docker inspect on it returns the image, not the limits.
sg docker -c "docker inspect pulse-prod-pulse-1 \
  --format 'memory={{.HostConfig.Memory}} cpus={{.HostConfig.NanoCpus}}'"
# Expected: memory=536870912 cpus=500000000  (cpu cap restored to 0.5; D-065 mitigation reverted by S10/D-068)

# Logs clean (no ERROR, no unexpected WARN):
sg docker -c "docker compose ${DC_ARGS} logs pulse --tail 50" | grep -iE 'ERROR|panic'
# Expected: empty
```

---

## Rollback procedure

Use when the new deploy fails smoke or is found broken in prod.

### Step 1 — Retag the known-good image as latest

```sh
# Replace pre-dNNN with the tag taken in Pre-upgrade Step 1:
sg docker -c "docker tag pulse-prod-pulse:pre-d063 pulse-prod-pulse:latest"
```

### Step 2 — Bring up the rolled-back stack

```sh
sg docker -c "docker compose ${DC_ARGS} up -d"
# No --build: the pre-built known-good image is used.
```

### Step 3 — Smoke the rollback

```sh
curl -sf --max-time 10 \
  --resolve beyondkaira.com:443:161.97.172.146 \
  https://beyondkaira.com/healthz
# Expected: {"status":"ok",...}
```

---

## Verifying a meta-store schema upgrade (SQLite WAL gotcha)

`applySchemaUpgrades` runs at every `pulse serve` boot (additive `ALTER TABLE`s).
To verify a column landed, do NOT inspect a copy of `pulse_meta.db` alone — the
ALTER may still sit un-checkpointed in the WAL and the bare file shows the OLD
schema (observed D-065). Copy all three files, then inspect:

```sh
for f in pulse_meta.db pulse_meta.db-wal pulse_meta.db-shm; do
  sg docker -c "docker cp pulse-prod-pulse-1:/var/lib/pulse/$f /tmp/check-$f"
done
python3 -c "import sqlite3; print([r[1] for r in sqlite3.connect('/tmp/check-pulse_meta.db').execute('PRAGMA table_info(ams_sources)')])"
rm -f /tmp/check-pulse_meta.db*
```

## ClickHouse DDL stance

**Migrations are forward-only and frozen.** The DDL files in `contracts/db/clickhouse/` and
`contracts/db/meta/` are immutable once merged; schema upgrades are additive
(`ALTER TABLE ... ADD COLUMN`). There is no `down-migrate` path.

**If a bad migration is deployed:**
1. Do NOT attempt to undo the DDL manually.
2. Roll back the binary (rollback procedure above).
3. Restore ClickHouse from the pre-upgrade backup (see `deploy/runbooks/backup-restore.md`).
4. Open a work order for a forward-only corrective migration.

---

## Critical NEVER rules

> **NEVER run `docker compose down -v`** on the production stack or on any command that
> names the `pulse-data` volume. The `pulse-prod_pulse-data` volume holds the SQLite
> meta store (`pulse_meta.db`) which contains the admin API token, all alert rules,
> channels, users, and probe configs. Destroying it requires a manual `pulse admin token`
> regeneration and a full rules reconfiguration.
>
> If a volume wipe is unavoidable (e.g. corrupted DB), restore from backup first, then
> proceed — never drop the volume cold.

Safe teardown (stops containers, keeps volumes):
```sh
sg docker -c "docker compose ${DC_ARGS} down"
# NOT: docker compose down -v
```
