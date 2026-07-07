# Pulse Backup & Restore Runbook

## What is backed up

| Store | Artifact | Location inside sidecar | Schedule |
|---|---|---|---|
| ClickHouse metrics | `ch/pulse-YYYYMMDD-HHMMSS.zip` | `/backups/ch/` | Every 24 h |
| SQLite meta store | `meta/pulse_meta-YYYYMMDD-HHMMSS.db` (+ `-wal`, `-shm`) | `/backups/meta/` | Every 24 h |

**ClickHouse** (`pulse` database): viewer sessions, stream health events, rollups,
all time-series metrics collected from Ant Media Server.

**SQLite meta store** (`pulse_meta.db`): alert rules, API tokens, probe config,
user accounts, license data. NOT time-series — this is the configuration/control plane.

Retention: **7 most-recent artifacts per type** (configurable via `KEEP_COUNT` in the script).
Older artifacts are pruned automatically after each backup cycle.

---

## Architecture

The backup overlay (`deploy/docker-compose.backup.yml`) adds two things:

1. **ClickHouse `config.d` drop-in** (`deploy/config/clickhouse-backups.xml`):
   defines a local disk named `backups` at `/backups/` and adds it to the
   ClickHouse allow-list so `BACKUP`/`RESTORE` SQL works.

2. **`backup` sidecar service**: uses the same digest-pinned ClickHouse image
   (supplies `clickhouse-client` + `bash`; no new image introduced). Mounts:
   - `pulse-backups:/backups` — artifact storage (shared with ClickHouse)
   - `pulse-data:/var/lib/pulse:ro` — SQLite meta store (read-only; see note below)
   - `./scripts:/scripts:ro` — the backup script

**SQLite consistency note:** The ClickHouse image does not ship `sqlite3`.
The meta store uses WAL journal mode (`_pragma=journal_mode(WAL)`). The
sidecar copies `pulse_meta.db` + `.db-wal` + `.db-shm` after a `sync` call.
This is not perfectly atomic: a WAL checkpoint between the `db` copy and `wal`
copy could cause a brief commit gap. For the meta store's low write rate
(rules/tokens/users), this is acceptable. The next backup cycle recovers
the stable state. For strict consistency you would need `sqlite3 .backup`
in the sidecar image.

---

## Enabling backups (prod apply)

Add `deploy/docker-compose.backup.yml` to your compose command:

```sh
docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.hardened.yml \
  -f deploy/docker-compose.prod-tls.yml \
  -f deploy/docker-compose.real-ams.yml \
  -f deploy/docker-compose.backup.yml \
  --env-file deploy/.env \
  up -d
```

The backup sidecar runs automatically. On the first start it executes an
immediate backup, then repeats every 24 h.

---

## Where backup artifacts live

Inside the `pulse-backups` Docker volume (project-prefixed, e.g.
`pulse-prod_pulse-backups`):

```
/backups/
  ch/
    pulse-20260707-020000.zip   ← ClickHouse ZIP archives
    pulse-20260706-020000.zip
    ...
  meta/
    pulse_meta-20260707-020000.db       ← SQLite db copy
    pulse_meta-20260707-020000.db-wal   ← WAL file (if present at backup time)
    pulse_meta-20260707-020000.db-shm   ← SHM file (if present)
    pulse_meta-20260706-020000.db
    ...
```

To browse artifacts:

```sh
docker run --rm \
  -v pulse-prod_pulse-backups:/backups:ro \
  busybox ls -lhR /backups
```

---

## Manual backup

Run a single backup cycle immediately (useful after schema changes or before upgrades):

```sh
# If the stack is up:
docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.backup.yml \
  exec backup /scripts/pulse-backup.sh once

# One-shot without a running daemon:
docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.backup.yml \
  run --rm backup once
```

Exit code 0 = success. Non-zero = at least one store failed; check `docker logs`.

---

## Restore: ClickHouse

> **Credential prerequisite:** Docker Compose automatically reads `deploy/.env` when
> running `docker compose` commands, injecting `CLICKHOUSE_USER` and
> `CLICKHOUSE_PASSWORD` into the containers — but it does NOT propagate those
> variables into your host shell. The `clickhouse-client` flags below reference
> `${CLICKHOUSE_USER}` and `${CLICKHOUSE_PASSWORD}` as host-shell variables. Export
> them explicitly before running any restore step:
>
> ```sh
> # Run from the repo root. Loads the same credentials compose injects into containers.
> export $(grep -E '^CLICKHOUSE_(USER|PASSWORD)=' deploy/.env | xargs)
> ```
>
> Omitting this step causes: `Code: 516. DB::Exception: default: Authentication failed`.

**Step 1 — Identify the backup to restore**

```sh
docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.backup.yml \
  exec backup ls -lhR /backups/ch/
```

Choose a zip filename, e.g. `ch/pulse-20260707-020000.zip`.

**Step 2 — Drop the current database** (requires the backup overlay running so ClickHouse has the backups disk)

```sh
docker compose -p pulse-prod exec backup \
  clickhouse-client \
    --host clickhouse \
    --user "${CLICKHOUSE_USER}" \
    --password "${CLICKHOUSE_PASSWORD}" \
    --query "DROP DATABASE IF EXISTS pulse"
```

**Step 3 — Restore from the backup**

```sh
docker compose -p pulse-prod exec backup \
  clickhouse-client \
    --host clickhouse \
    --user "${CLICKHOUSE_USER}" \
    --password "${CLICKHOUSE_PASSWORD}" \
    --query "RESTORE DATABASE pulse FROM Disk('backups', 'ch/pulse-20260707-020000.zip')"
```

Expected output: `<uuid>    RESTORED`

**Step 4 — Verify**

```sh
docker compose -p pulse-prod exec backup \
  clickhouse-client \
    --host clickhouse \
    --user "${CLICKHOUSE_USER}" \
    --password "${CLICKHOUSE_PASSWORD}" \
    --query "SELECT count() FROM pulse.viewer_sessions"
```

---

## Restore: SQLite meta store

The SQLite meta store is the configuration plane (rules, tokens, users). Stop
the pulse app before replacing the database file to avoid write conflicts.

**Step 1 — Identify the backup to restore**

```sh
docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.backup.yml \
  exec backup ls -lhR /backups/meta/
```

Choose a file, e.g. `meta/pulse_meta-20260707-020000.db`.

**Step 2 — Stop the pulse application** (prevents writes during restore)

```sh
docker compose -p pulse-prod stop pulse
```

**Step 3 — Copy the backup into the pulse-data volume**

> **Critical WAL-clearing step**: Remove the live WAL and SHM files *before*
> overwriting the db. A crashed or SIGKILL'd pulse process may have left
> post-backup WAL frames on disk. If these are not removed first, SQLite
> replays them onto the restored db and produces a state *beyond* the backup
> point (e.g. 120 rows restored instead of 100 backed up).

```sh
docker run --rm \
  -v pulse-prod_pulse-backups:/src:ro \
  -v pulse-prod_pulse-data:/dst \
  busybox sh -c "
    # STEP A: clear any stale WAL/SHM that may exist from a crashed pulse process.
    # Must happen BEFORE cp so SQLite cannot replay post-backup frames onto the
    # restored db file. (rm -f is idempotent — safe if files do not exist.)
    rm -f /dst/pulse_meta.db-wal /dst/pulse_meta.db-shm
    # STEP B: overwrite the db file with the backup.
    cp /src/meta/pulse_meta-20260707-020000.db /dst/pulse_meta.db
    # STEP C: restore backup WAL if the backup contains one (captures in-flight
    # commits that existed at backup time and were not yet checkpointed into the
    # db file). The previous rm ensures only the backup WAL is present.
    [ -f /src/meta/pulse_meta-20260707-020000.db-wal ] && \
      cp /src/meta/pulse_meta-20260707-020000.db-wal /dst/pulse_meta.db-wal || true
    echo done
  "
```

**Step 4 — Restart pulse**

```sh
docker compose -p pulse-prod start pulse
```

**Step 5 — Verify**

```sh
# Wait for pulse to become healthy, then hit the health endpoint:
curl -sf https://your-domain.com/healthz
```

Check that rules, tokens, and settings visible in the UI match the expected restore point.

---

## S3 push (optional, off by default)

The ClickHouse image does not ship `aws-cli`. To enable S3 off-site upload:

1. Build a custom sidecar image that extends the digest-pinned CH image and adds `aws-cli`.
2. Override the `backup.image` in a local override file (do NOT edit `docker-compose.backup.yml`).
3. Set env vars in `deploy/.env`:
   ```env
   PULSE_BACKUP_S3_BUCKET=my-pulse-backups
   PULSE_BACKUP_S3_PREFIX=pulse-backups/
   PULSE_BACKUP_S3_REGION=us-east-1
   AWS_ACCESS_KEY_ID=...
   AWS_SECRET_ACCESS_KEY=...
   ```
4. The `push_s3()` function in `pulse-backup.sh` will detect `aws-cli` and upload
   each cycle's artifacts. It is a no-op if `PULSE_BACKUP_S3_BUCKET` is empty.

---

## Monitoring backup health

```sh
# View live daemon logs:
docker compose -p pulse-prod logs -f backup

# Check last cycle result (look for "Backup cycle complete" or "COMPLETED WITH ERRORS"):
docker compose -p pulse-prod logs backup | grep -E "Backup cycle|ERROR" | tail -5
```

A failed backup cycle logs `[pulse-backup] ERROR: ...` to stderr but does NOT
crash the daemon or the stack. ClickHouse and pulse continue running. Fix the
root cause (disk full, ClickHouse unreachable, etc.) and run a manual backup.
