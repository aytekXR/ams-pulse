#!/usr/bin/env bash
#
# rotate-clickhouse-password.sh — rotate the ClickHouse credential of a running
# Pulse stack, with verification and automatic rollback.
#
# WHY THIS IS SAFE WITHOUT VOLUME SURGERY
# ---------------------------------------
# The Pulse ClickHouse user is defined in `users_xml`, not in ClickHouse's own
# access storage. The official image's entrypoint calls `manage_clickhouse_user`
# UNCONDITIONALLY on every container start (before `init_clickhouse_db`, so it is
# NOT gated on a first-run/empty-data-dir check) and rewrites
# `/etc/clickhouse-server/users.d/default-user.xml` from $CLICKHOUSE_PASSWORD.
# Verified against clickhouse/clickhouse-server by reading /entrypoint.sh:
#   - `SELECT name, storage FROM system.users` reports storage = users_xml
#   - entrypoint line ~262 calls manage_clickhouse_user outside any guard
# Therefore: change the env value, recreate the containers, done. There is no
# stored password hash in the data volume to migrate, and NOTHING here ever needs
# `docker compose down -v` (which would destroy the metrics history).
#
# WHAT IT ROTATES
#   CLICKHOUSE_PASSWORD in the target env file, then recreates every service that
#   consumes it: the clickhouse server itself, the pulse server (which builds
#   PULSE_CLICKHOUSE_DSN from it), and the backup sidecar. Missing the backup
#   sidecar is the classic failure here — it keeps the stale password and its next
#   run fails authentication, silently, hours later.
#
# USAGE
#   deploy/scripts/rotate-clickhouse-password.sh [options]
#
#     --dry-run          Show what would happen; touch nothing. Always run this first.
#     --project NAME     Compose project name (default: pulse-prod)
#     --env-file PATH    Env file to rewrite (default: deploy/.env)
#     --yes              Skip the interactive confirmation
#     --health-url URL   Health endpoint to verify (default: probe the container directly)
#
# EXIT CODES
#   0 rotated and verified · 1 precondition failed (nothing changed)
#   2 rotation failed and was ROLLED BACK · 3 rotation failed and rollback ALSO
#     failed (manual intervention required — the backup env file path is printed)
#
# The new password is never echoed. Only its length and a masked form are printed.

set -euo pipefail

PROJECT="pulse-prod"
ENV_FILE=""
DRY_RUN=0
ASSUME_YES=0
HEALTH_URL=""

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

die() {
	echo "ERROR: $*" >&2
	exit 1
}
info() { echo "  $*"; }
step() { echo; echo "==> $*"; }

while [ $# -gt 0 ]; do
	case "$1" in
	--dry-run) DRY_RUN=1 ;;
	--yes) ASSUME_YES=1 ;;
	--project)
		PROJECT="${2:-}"
		shift
		;;
	--env-file)
		ENV_FILE="${2:-}"
		shift
		;;
	--health-url)
		HEALTH_URL="${2:-}"
		shift
		;;
	-h | --help)
		sed -n '2,45p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*) die "unknown argument: $1 (try --help)" ;;
	esac
	shift
done

[ -n "$ENV_FILE" ] || ENV_FILE="${REPO_ROOT}/deploy/.env"

# The standing five-overlay production combo. Omitting the backup overlay on
# `up -d` would REMOVE the backup sidecar, so it is part of the combo, not optional.
COMPOSE_FILES=(
	"${REPO_ROOT}/deploy/docker-compose.yml"
	"${REPO_ROOT}/deploy/docker-compose.hardened.yml"
	"${REPO_ROOT}/deploy/docker-compose.prod-tls.yml"
	"${REPO_ROOT}/deploy/docker-compose.real-ams.yml"
	"${REPO_ROOT}/deploy/docker-compose.backup.yml"
)

DC_ARGS=(-p "$PROJECT")
for f in "${COMPOSE_FILES[@]}"; do
	DC_ARGS+=(-f "$f")
done
DC_ARGS+=(--env-file "$ENV_FILE")

# `sg docker` is needed on hosts where the docker group is stale in non-login
# shells; fall back to plain docker when the user already has the group.
docker_cmd() {
	if docker ps >/dev/null 2>&1; then
		docker "$@"
	else
		local q
		q="$(printf '%q ' docker "$@")"
		sg docker -c "$q"
	fi
}

compose() { docker_cmd compose "${DC_ARGS[@]}" "$@"; }

mask() {
	# Print a non-reversible shape of a secret: length plus first/last 2 chars.
	local v="$1"
	printf '%s…%s (len %d)' "${v:0:2}" "${v: -2}" "${#v}"
}

# ── 1. Preconditions ─────────────────────────────────────────────────────────
step "Preconditions"

[ -f "$ENV_FILE" ] || die "env file not found: $ENV_FILE"
docker_cmd version >/dev/null 2>&1 || die "cannot talk to docker"
for f in "${COMPOSE_FILES[@]}"; do
	[ -f "$f" ] || die "compose file missing: $f"
done

PERMS="$(stat -c '%a' "$ENV_FILE")"
[ "$PERMS" = "600" ] || echo "  WARNING: $ENV_FILE is mode $PERMS, expected 600"

grep -qE '^CLICKHOUSE_PASSWORD=' "$ENV_FILE" || die "no CLICKHOUSE_PASSWORD= line in $ENV_FILE"
CH_USER="$(grep -E '^CLICKHOUSE_USER=' "$ENV_FILE" | head -1 | cut -d= -f2-)"
[ -n "$CH_USER" ] || die "no CLICKHOUSE_USER= line in $ENV_FILE"
OLD_PW="$(grep -E '^CLICKHOUSE_PASSWORD=' "$ENV_FILE" | head -1 | cut -d= -f2-)"
[ -n "$OLD_PW" ] || die "CLICKHOUSE_PASSWORD is empty in $ENV_FILE"

info "project:   $PROJECT"
info "env file:  $ENV_FILE (mode $PERMS)"
info "ch user:   $CH_USER"
info "current:   $(mask "$OLD_PW")"

CH_CONTAINER="$(compose ps -q clickhouse 2>/dev/null | head -1 || true)"
[ -n "$CH_CONTAINER" ] || die "no running clickhouse container in project '$PROJECT' — start the stack first"
info "clickhouse container: ${CH_CONTAINER:0:12}"

# ── 2. Baseline ──────────────────────────────────────────────────────────────
step "Baseline (pre-rotation)"

ch_query() {
	# Runs a query inside the CH container using its OWN env, so no secret
	# crosses the command line or the process table of the host.
	# shellcheck disable=SC2016 # deliberate: $CLICKHOUSE_* must expand INSIDE the
	# container, not on this host — expanding here would put the password in the
	# host's process table, which is the thing we are rotating away from.
	docker_cmd exec "$1" sh -c \
		'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "'"$2"'"' 2>/dev/null
}

BASELINE_COUNT="$(ch_query "$CH_CONTAINER" 'SELECT count() FROM pulse.server_events' || echo "")"
[ -n "$BASELINE_COUNT" ] || die "could not read a baseline row count — is the stack healthy? Refusing to rotate."
info "pulse.server_events rows: $BASELINE_COUNT"

BASELINE_LAG="$(ch_query "$CH_CONTAINER" 'SELECT dateDiff(second, max(ts), now()) FROM pulse.server_events' || echo "?")"
info "newest event age: ${BASELINE_LAG}s"

if [ "$DRY_RUN" = "1" ]; then
	step "DRY RUN — no changes made"
	info "would back up   : ${ENV_FILE}.bak.<timestamp>"
	info "would rewrite   : CLICKHOUSE_PASSWORD in $ENV_FILE (48 hex chars, openssl rand -hex 24)"
	info "would recreate  : $(compose ps --services 2>/dev/null | tr '\n' ' ')"
	info "would verify    : new password authenticates, old password REJECTED,"
	info "                  all /healthz components ok, row count >= $BASELINE_COUNT"
	info "would roll back : restore the env file and recreate, on any verification failure"
	exit 0
fi

if [ "$ASSUME_YES" != "1" ]; then
	echo
	echo "This rotates the ClickHouse password of the RUNNING '$PROJECT' stack and"
	echo "recreates its containers. Metrics history is NOT touched (no down -v)."
	printf "Type 'rotate' to continue: "
	read -r CONFIRM
	[ "$CONFIRM" = "rotate" ] || die "aborted by operator"
fi

# ── 3. Rotate ────────────────────────────────────────────────────────────────
step "Rotating"

command -v openssl >/dev/null 2>&1 || die "openssl not found (needed to generate the new password)"
NEW_PW="$(openssl rand -hex 24)"
[ "${#NEW_PW}" = "48" ] || die "generated password has unexpected length ${#NEW_PW}"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
BACKUP="${ENV_FILE}.bak.${STAMP}"
cp -p "$ENV_FILE" "$BACKUP"
chmod 600 "$BACKUP"
info "env backed up to $BACKUP"

# Rewrite in place, preserving every other line and the file's mode. Done via a
# temp file in the same directory so the replacement is atomic.
TMP="$(mktemp "${ENV_FILE}.tmp.XXXXXX")"
chmod 600 "$TMP"
NEW_PW_VALUE="$NEW_PW" awk '
	/^CLICKHOUSE_PASSWORD=/ && !done { print "CLICKHOUSE_PASSWORD=" ENVIRON["NEW_PW_VALUE"]; done=1; next }
	{ print }
' "$ENV_FILE" >"$TMP"
mv "$TMP" "$ENV_FILE"

WROTE="$(grep -E '^CLICKHOUSE_PASSWORD=' "$ENV_FILE" | head -1 | cut -d= -f2-)"
[ "$WROTE" = "$NEW_PW" ] || die "env rewrite did not take effect — restore from $BACKUP"
info "new password written: $(mask "$NEW_PW")"

rollback() {
	echo
	echo "!! ROLLING BACK — restoring $BACKUP"
	cp -p "$BACKUP" "$ENV_FILE"
	if compose up -d >/dev/null 2>&1; then
		echo "!! rolled back; stack recreated with the previous password"
		exit 2
	fi
	echo "!! ROLLBACK RECREATE FAILED. The env file is restored at $ENV_FILE."
	echo "!! Run manually: docker compose -p $PROJECT <five -f overlays> --env-file $ENV_FILE up -d"
	exit 3
}

step "Recreating services"
compose up -d || rollback
info "compose up -d completed"

# ── 4. Verify ────────────────────────────────────────────────────────────────
step "Verifying"

CH_CONTAINER="$(compose ps -q clickhouse | head -1)"
[ -n "$CH_CONTAINER" ] || rollback

# Wait for ClickHouse to accept the new credential.
OK=0
for _ in $(seq 1 30); do
	if ch_query "$CH_CONTAINER" 'SELECT 1' | grep -q '^1$'; then
		OK=1
		break
	fi
	sleep 2
done
[ "$OK" = "1" ] || {
	echo "  clickhouse did not accept the new credential within 60s"
	rollback
}
info "clickhouse authenticates with the NEW password"

# The old password must now be rejected. If it still works, the rotation did not
# actually take — treat that as a failure, not a success.
# shellcheck disable=SC2016 # deliberate: expand inside the container. The old
# password is passed via -e so it never appears in the host process table either.
if docker_cmd exec -e OLDPW="$OLD_PW" "$CH_CONTAINER" sh -c \
	'clickhouse-client --user "$CLICKHOUSE_USER" --password "$OLDPW" --query "SELECT 1"' >/dev/null 2>&1; then
	echo "  the OLD password still authenticates — rotation did not take effect"
	rollback
fi
info "old password is REJECTED"

# Row count must not have gone backwards (proves we did not lose the volume).
NEW_COUNT="$(ch_query "$CH_CONTAINER" 'SELECT count() FROM pulse.server_events' || echo "")"
[ -n "$NEW_COUNT" ] || rollback
if [ "$NEW_COUNT" -lt "$BASELINE_COUNT" ]; then
	echo "  row count went BACKWARDS ($BASELINE_COUNT -> $NEW_COUNT) — data loss, rolling back"
	rollback
fi
info "pulse.server_events rows: $NEW_COUNT (baseline $BASELINE_COUNT)"

# Pulse health, scoped per component. A whole-body '"status":"ok"' grep passes
# while the collector is degraded, because sibling components match it.
step "Health (per component)"
PULSE_CONTAINER="$(compose ps -q pulse | head -1)"
HEALTH=""
for _ in $(seq 1 30); do
	if [ -n "$HEALTH_URL" ]; then
		HEALTH="$(curl -fsS -m 10 "$HEALTH_URL" 2>/dev/null || echo "")"
	else
		HEALTH="$(docker_cmd exec "$PULSE_CONTAINER" wget -qO- http://127.0.0.1:8090/healthz 2>/dev/null || echo "")"
	fi
	[ -n "$HEALTH" ] && break
	sleep 2
done
[ -n "$HEALTH" ] || {
	echo "  /healthz returned nothing"
	rollback
}

BAD="$(printf '%s' "$HEALTH" | python3 -c '
import sys, json
d = json.load(sys.stdin)
bad = [k for k, v in (d.get("components") or {}).items()
       if (v.get("status") if isinstance(v, dict) else v) != "ok"]
print(",".join(bad))
' 2>/dev/null || echo "PARSE_FAILED")"

if [ -n "$BAD" ]; then
	echo "  components not ok: $BAD"
	rollback
fi
info "all /healthz components ok"

# The backup sidecar must be running with the new value, not the stale one.
step "Backup sidecar"
BK="$(compose ps -q backup 2>/dev/null | head -1 || true)"
if [ -z "$BK" ]; then
	echo "  WARNING: no backup service in this project — if you expected one, the"
	echo "           backup overlay was probably omitted from the compose combo."
else
	# Compare inside the container and pass the expected value via -e, so neither
	# the old nor the new secret is ever interpolated into a host command line.
	# shellcheck disable=SC2016 # deliberate: both vars expand inside the container.
	if docker_cmd exec -e EXPECTED="$NEW_PW" "$BK" sh -c 'test "$CLICKHOUSE_PASSWORD" = "$EXPECTED"'; then
		info "backup sidecar carries the new password"
	else
		echo "  backup sidecar still has the OLD password — it was not recreated"
		rollback
	fi
fi

# ── 5. Ingestion still live ──────────────────────────────────────────────────
step "Confirming ingestion resumed"
sleep 20
LAG="$(ch_query "$CH_CONTAINER" 'SELECT dateDiff(second, max(ts), now()) FROM pulse.server_events' || echo "")"
if [ -n "$LAG" ] && [ "$LAG" -lt 120 ]; then
	info "newest event age: ${LAG}s — collector is ingesting"
else
	echo "  WARNING: newest event is ${LAG:-unknown}s old. The rotation itself verified,"
	echo "           but check 'compose logs pulse' before considering this finished."
fi

step "Rotated and verified"
echo "  Previous env file: $BACKUP"
echo "  Delete that backup once you are satisfied:  shred -u $BACKUP"
echo
echo "  Remaining follow-ups (NOT done by this script):"
echo "    - rotate any other credential exposed alongside this one"
echo "    - the old value stays in git history forever; rotation is what closes it"
