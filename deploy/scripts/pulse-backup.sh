#!/usr/bin/env bash
# pulse-backup.sh — ClickHouse metrics + SQLite meta store backup for Pulse.
#
# Modes:
#   once    Run a single backup cycle; exit code reflects success (0) or failure (1).
#           Suitable for cron, CI, or manual invocation.
#   daemon  (default) Run one cycle immediately, then repeat every 24 h.
#           In-loop errors are logged but do NOT exit the daemon.
#
# Required env (set via docker-compose.backup.yml):
#   CLICKHOUSE_HOST       ClickHouse hostname (default: clickhouse)
#   CLICKHOUSE_USER       ClickHouse user     (default: default)
#   CLICKHOUSE_PASSWORD   ClickHouse password (default: empty — plain isolated stacks)
#   CLICKHOUSE_DATABASE   Database to back up (default: pulse)
#   PULSE_META_DB         Path to the SQLite meta store inside the sidecar
#                         (default: /var/lib/pulse/pulse_meta.db)
#
# Optional S3 push (all three must be non-empty to enable):
#   PULSE_BACKUP_S3_BUCKET   Target S3 bucket name
#   PULSE_BACKUP_S3_PREFIX   Key prefix inside the bucket  (default: pulse-backups/)
#   PULSE_BACKUP_S3_REGION   AWS region                    (default: us-east-1)
#   AWS credentials must be injected via AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY
#   or an IAM instance role. aws-cli is NOT shipped in the ClickHouse image; the S3
#   push section below documents the approach for a custom sidecar image that adds it.
#
# Daemon readiness / retry knobs:
#   CH_READY_WAIT          Seconds to wait for ClickHouse to be ready at daemon
#                          start before giving up (default: 120). Daemon only —
#                          once mode does not call wait_for_clickhouse().
#   CH_BACKUP_ATTEMPTS     Max backup attempts per cycle for network-class failures
#                          (default: 3). Non-network failures break immediately.
#   CH_BACKUP_RETRY_DELAY  Seconds to sleep between network-class retry attempts
#                          (default: 20).
#
# SQLite consistency approach (IMPORTANT — READ BEFORE CHANGING):
#   The ClickHouse image does NOT ship sqlite3 (confirmed on 24.8 / sha256:1ffa82e).
#   The pulse meta store is opened with _pragma=journal_mode(WAL), confirmed in
#   server/internal/store/meta/meta.go. In WAL mode three files coexist:
#     pulse_meta.db       — main database file (checkpointed state)
#     pulse_meta.db-wal   — write-ahead log (uncommitted/uncheckpointed commits)
#     pulse_meta.db-shm   — shared-memory index (WAL coordination)
#
#   Backup strategy: sync(1) + copy db → copy wal → copy shm.
#   Rationale: copying db first then WAL means the WAL copy captures all commits
#   that landed after our db snapshot, giving a consistent view at WAL-copy-time.
#   Risk: if a WAL checkpoint fires between the db copy and WAL copy, some WAL
#   frames may have been moved to the db file (in our db copy) but excluded from
#   our WAL copy. On restore, SQLite replays only the frames in the WAL copy,
#   which might leave a brief gap. For the meta store's low write rate (rules,
#   tokens, users — NOT hot-path), this window is negligible; the next backup
#   cycle captures the stable state. This is accepted and documented here.
#   For strict consistency: use sqlite3 .backup command (not available in this image).

set -euo pipefail

CLICKHOUSE_HOST="${CLICKHOUSE_HOST:-clickhouse}"
CLICKHOUSE_USER="${CLICKHOUSE_USER:-default}"
CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD:-}"
CLICKHOUSE_DATABASE="${CLICKHOUSE_DATABASE:-pulse}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
META_SRC="${PULSE_META_DB:-/var/lib/pulse/pulse_meta.db}"
KEEP_COUNT=7
MODE="${1:-daemon}"
CH_READY_WAIT="${CH_READY_WAIT:-120}"
CH_BACKUP_ATTEMPTS="${CH_BACKUP_ATTEMPTS:-3}"
CH_BACKUP_RETRY_DELAY="${CH_BACKUP_RETRY_DELAY:-20}"

log() { printf '[pulse-backup] %s %s\n' "$(date -u +%T)" "$*"; }
err() { printf '[pulse-backup] ERROR: %s\n' "$*" >&2; }
die() { err "$*"; exit 1; }

# ── ClickHouse readiness wait (daemon only) ────────────────────────────────────
# Polls clickhouse-client SELECT 1 every 5 s up to CH_READY_WAIT seconds.
# NOT called in once mode.
wait_for_clickhouse() {
    local deadline
    deadline=$(( $(date +%s) + CH_READY_WAIT ))
    local args=(--host "$CLICKHOUSE_HOST" --user "$CLICKHOUSE_USER")
    [ -n "$CLICKHOUSE_PASSWORD" ] && args+=(--password "$CLICKHOUSE_PASSWORD")

    log "Waiting for ClickHouse to be ready (timeout=${CH_READY_WAIT}s)…"
    while true; do
        if clickhouse-client "${args[@]}" --query "SELECT 1" >/dev/null 2>&1; then
            log "ClickHouse is ready."
            return 0
        fi
        local now
        now=$(date +%s)
        if [ "$now" -ge "$deadline" ]; then
            err "ClickHouse not ready after ${CH_READY_WAIT}s — giving up"
            return 1
        fi
        local remaining
        remaining=$(( deadline - now ))
        log "CH not ready yet (${remaining}s remaining) — retrying in 5s"
        sleep 5
    done
}

# ── ClickHouse backup ──────────────────────────────────────────────────────────
backup_clickhouse() {
    local ts="$1"
    local dest="ch/pulse-${ts}.zip"
    # Ensure /backups/ch is owned by the ClickHouse server user (uid/gid 101).
    # The ClickHouse server daemon runs as uid=101 even though the sidecar container
    # starts as root. If /backups/ch was previously created by root (e.g. by a
    # helper container during setup), ClickHouse cannot write to it. We pre-create
    # and chown here since the sidecar runs as root and can change ownership.
    mkdir -p "${BACKUP_DIR}/ch"
    chown -R 101:101 "${BACKUP_DIR}/ch" 2>/dev/null || true

    local args=(--host "$CLICKHOUSE_HOST" --user "$CLICKHOUSE_USER")
    [ -n "$CLICKHOUSE_PASSWORD" ] && args+=(--password "$CLICKHOUSE_PASSWORD")

    local attempt=0
    while true; do
        attempt=$(( attempt + 1 ))
        log "CH: BACKUP DATABASE ${CLICKHOUSE_DATABASE} TO Disk('backups','${dest}') (attempt ${attempt}/${CH_BACKUP_ATTEMPTS})"
        if clickhouse-client "${args[@]}" \
                --query "BACKUP DATABASE ${CLICKHOUSE_DATABASE} TO Disk('backups', '${dest}')"; then
            log "CH backup OK: /backups/${dest}"
            return 0
        fi
        err "CH backup FAILED (dest=${dest}, attempt ${attempt}/${CH_BACKUP_ATTEMPTS})"
        # Probe to classify: network-class vs non-network failure.
        if clickhouse-client "${args[@]}" --query "SELECT 1" >/dev/null 2>&1; then
            # Server is reachable — non-network failure; retrying will not help.
            err "CH is reachable but backup failed — non-network error, not retrying"
            return 1
        fi
        # Network-class failure — may be transient.
        if [ "$attempt" -ge "$CH_BACKUP_ATTEMPTS" ]; then
            err "CH backup failed ${attempt} time(s) with network-class errors — giving up"
            return 1
        fi
        log "CH unreachable (network-class failure) — retrying in ${CH_BACKUP_RETRY_DELAY}s"
        sleep "$CH_BACKUP_RETRY_DELAY"
    done
}

# ── SQLite meta store backup ──────────────────────────────────────────────────
backup_sqlite() {
    local ts="$1"
    local dest_dir="${BACKUP_DIR}/meta"
    local dest="${dest_dir}/pulse_meta-${ts}.db"

    mkdir -p "$dest_dir"

    if [ ! -f "$META_SRC" ]; then
        log "SQLite: $META_SRC not found — skipping (first boot before any run?)"
        return 0
    fi

    log "SQLite: $META_SRC → $dest"

    # Flush OS page cache to disk before copying.
    sync

    # Copy main db file, then WAL, then SHM.
    # (See consistency note in header; this is the best available approach
    #  without sqlite3 binary.)
    cp "$META_SRC" "$dest"
    if [ -f "${META_SRC}-wal" ]; then
        cp "${META_SRC}-wal" "${dest}-wal"
        log "  copied $(du -b "${dest}-wal" | cut -f1) bytes of WAL"
    fi
    if [ -f "${META_SRC}-shm" ]; then
        cp "${META_SRC}-shm" "${dest}-shm"
    fi

    # Integrity: verify the SQLite magic header ("SQLite" in first 6 bytes).
    # od is part of GNU coreutils (available in the ClickHouse image).
    local magic
    magic="$(od -A n -t a -N 6 "$dest" 2>/dev/null | tr -d ' \n' || true)"
    if printf '%s' "$magic" | grep -q "SQLite"; then
        log "SQLite backup OK: $dest (header: ${magic})"
        return 0
    else
        err "SQLite integrity check FAILED for $dest (header: '${magic}')"
        err "Removing incomplete backup files"
        rm -f "$dest" "${dest}-wal" "${dest}-shm"
        return 1
    fi
}

# ── Retention: keep newest KEEP_COUNT files matching pattern in dir ────────────
# Removes extras oldest-first. Silently skips if the dir is empty or missing.
prune_old() {
    local dir="$1"
    local pattern="$2"
    [ -d "$dir" ] || return 0

    local old_files
    # shellcheck disable=SC2012  # ls -t is correct here; no newlines in backup filenames
    old_files="$(ls -1t "${dir}"/"${pattern}" 2>/dev/null \
        | tail -n "+$((KEEP_COUNT + 1))" \
        || true)"

    if [ -n "$old_files" ]; then
        printf '%s\n' "$old_files" | while IFS= read -r f; do
            log "Retention: pruning $f"
            rm -rf "$f"
        done
    fi
}

# ── S3 push (env-gated, OFF by default) ───────────────────────────────────────
# Requires: aws-cli installed in the sidecar image (NOT shipped in CH image).
# To enable: build a custom sidecar image FROM the CH digest image + aws-cli,
# set PULSE_BACKUP_S3_BUCKET, PULSE_BACKUP_S3_PREFIX, PULSE_BACKUP_S3_REGION,
# and inject AWS credentials via AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY or IAM role.
# This function is a no-op (returns 0) when the bucket is not configured.
push_s3() {
    local ts="$1"
    local bucket="${PULSE_BACKUP_S3_BUCKET:-}"
    [ -n "$bucket" ] || return 0

    local prefix="${PULSE_BACKUP_S3_PREFIX:-pulse-backups/}"
    local region="${PULSE_BACKUP_S3_REGION:-us-east-1}"

    if ! command -v aws >/dev/null 2>&1; then
        err "PULSE_BACKUP_S3_BUCKET is set but aws-cli is not installed — S3 push skipped"
        err "Build a custom sidecar image that adds aws-cli to the CH base image."
        return 1
    fi

    log "S3: uploading to s3://${bucket}/${prefix} (region=${region})"
    if aws s3 sync "${BACKUP_DIR}/" "s3://${bucket}/${prefix}" \
        --region "$region" \
        --exclude "*" \
        --include "ch/pulse-${ts}.*" \
        --include "meta/pulse_meta-${ts}.*"; then
        log "S3 upload OK"
    else
        err "S3 upload FAILED"
        return 1
    fi
}

# ── Main backup cycle ─────────────────────────────────────────────────────────
do_backup() {
    local ts
    ts="$(date -u +%Y%m%d-%H%M%S)"
    local failed=0

    backup_clickhouse "$ts" || failed=1
    backup_sqlite     "$ts" || failed=1
    push_s3           "$ts" || failed=1

    if [ "$failed" -eq 0 ]; then
        # Retention — keep newest KEEP_COUNT of each artifact type.
        # Only prune when ALL steps succeeded so a broken backup path cannot
        # erode the intact retention set.
        prune_old "${BACKUP_DIR}/ch"   "pulse-*.zip"
        prune_old "${BACKUP_DIR}/meta" "pulse_meta-*.db"
        prune_old "${BACKUP_DIR}/meta" "pulse_meta-*.db-wal"
        prune_old "${BACKUP_DIR}/meta" "pulse_meta-*.db-shm"
        log "Backup cycle complete (ts=${ts})"
    else
        err "Backup cycle FAILED (ts=${ts}) — check logs above"
    fi
    return "$failed"
}

# ── Entry point ───────────────────────────────────────────────────────────────
case "$MODE" in
    once)
        do_backup
        ;;
    daemon)
        log "Daemon starting — first backup now, then every 24 h (keep=${KEEP_COUNT})"
        wait_for_clickhouse
        while true; do
            do_backup || log "Cycle FAILED - sleeping 24 h"
            log "Sleeping 24 h"
            sleep 86400
        done
        ;;
    *)
        die "Unknown mode '${MODE}'. Use: once | daemon"
        ;;
esac
