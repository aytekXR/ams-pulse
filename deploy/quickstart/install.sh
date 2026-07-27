#!/usr/bin/env bash
# Pulse quickstart installer — D-089 RULE-2
#
# One-command install that:
#   1. Checks for Docker + Compose v2 (distinguishes "not in docker group" from
#      "Compose plugin missing")
#   2. Collects AMS connection details (flags or interactive prompts)
#   3. Fetches docker-compose.quickstart.yml if not co-located
#   4. Preflights the image pull — fails honestly on 401/403 with auth instructions
#   5. Generates PULSE_SECRET_KEY if absent
#   6. Writes .env (chmod 600); removed on pre-start failure (kept if stack started)
#   7. Brings up the stack
#   8. Polls /healthz for up to 90 s (fails hard on timeout — NEVER claims success)
#   9. Extracts the bootstrap admin token from container logs (honest re-run handling)
#
# Usage:
#   bash install.sh --ams-url http://10.0.1.10:5080 --email admin@example.com --password <pw>
#   bash install.sh --help
#
# Supports both in-repo execution and curl-piped install. Note the `-s --`: with a
# plain `| bash` the script owns stdin, so the interactive prompts cannot read the
# terminal and every argument is unreachable. Always pass the flags explicitly:
#   curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/install.sh \
#     | bash -s -- --ams-url http://10.0.1.10:5080 --email admin@example.com --password <pw>

set -euo pipefail

# PULSE_REF pins all raw.githubusercontent.com downloads to the release tag
# matching the pinned image, so a curl|bash install can never pair an old image
# with a newer compose file from main (the release-time guard enforces the pin).
PULSE_REF="${PULSE_REF:-v0.4.3}"
REPO_RAW="https://raw.githubusercontent.com/aytekXR/ams-pulse/${PULSE_REF}"
REPO_WEB="https://github.com/aytekXR/ams-pulse"
HEALTHZ_DEADLINE=90   # seconds
# How long to keep watching the collector AFTER the stack reports healthy, so a
# wrong AMS URL / wrong credentials is reported by the INSTALLER rather than
# discovered in the UI a minute later. Must exceed the 30 s collector staleness
# floor (D-164) for the verdict to be meaningful.
COLLECTOR_SETTLE_SECONDS="${COLLECTOR_SETTLE_SECONDS:-40}"
export PULSE_IMAGE="${PULSE_IMAGE:-ghcr.io/aytekxr/ams-pulse:0.4.3}"
# Host port for the Pulse UI/API. Override when 8090 is already taken on this
# machine: PULSE_HOST_PORT=18090 ./install.sh …
export PULSE_HOST_PORT="${PULSE_HOST_PORT:-8090}"

# ── Locate working directory ──────────────────────────────────────────────────
# When executed via `curl ... | bash`, BASH_SOURCE[0] is empty or set to "bash".
# In that case create a `quickstart/` subdirectory of cwd so output files land
# somewhere predictable and the operator knows where to find them.
SCRIPT_SOURCE="${BASH_SOURCE[0]:-}"
_WORK_CANDIDATE=""
if [[ -n "$SCRIPT_SOURCE" && "$SCRIPT_SOURCE" != "bash" && -f "$SCRIPT_SOURCE" ]]; then
  _WORK_CANDIDATE="$(cd "$(dirname "$SCRIPT_SOURCE")" 2>/dev/null && pwd)"
fi
# Reject /dev (handles `bash /dev/stdin < install.sh`) and empty results.
if [[ -n "$_WORK_CANDIDATE" && "$_WORK_CANDIDATE" != "/dev" ]]; then
  WORK_DIR="$_WORK_CANDIDATE"
else
  WORK_DIR="$(pwd)/quickstart"
  mkdir -p "$WORK_DIR"
fi

COMPOSE_FILE="$WORK_DIR/docker-compose.quickstart.yml"
ENV_FILE="$WORK_DIR/.env"
CREATED_ENV=0   # set to 1 only when THIS run writes the .env for the first time
STACK_STARTED=0 # set to 1 after `docker compose up -d` succeeds

# ── Cleanup on failure ────────────────────────────────────────────────────────
# Remove the .env ONLY when: (a) the exit is non-zero, AND (b) this run created
# it, AND (c) the stack never started (deleting it while containers run leaves an
# orphaned stack with no key file for the next operator run).
cleanup() {
  local rc=$?
  if [[ $rc -ne 0 && "$CREATED_ENV" -eq 1 && "$STACK_STARTED" -eq 0 && -f "$ENV_FILE" ]]; then
    rm -f "$ENV_FILE"
    printf '[cleanup] Removed %s (not leaving secrets from a failed pre-start run).\n' "$ENV_FILE" >&2
  elif [[ $rc -ne 0 && -f "$ENV_FILE" && "$STACK_STARTED" -eq 1 ]]; then
    printf '[cleanup] Kept %s — the stack is running and the file contains the secret key.\n' "$ENV_FILE" >&2
  fi
}
trap cleanup EXIT

# ── Usage ─────────────────────────────────────────────────────────────────────
usage() {
  cat <<EOF
Usage: $0 [flags]

Flags:
  --ams-url      <url>   AMS REST base URL  (e.g. http://10.0.1.10:5080)
  --email        <addr>  AMS admin email
  --password     <pass>  AMS admin password
  --license-key  <key>   Pulse license key  (optional; empty = Free tier)
  --help                 Show this message and exit

Environment overrides:
  PULSE_HOST_PORT             Host port for the Pulse UI/API (default 8090).
                              Use this when 8090 is already taken.
  COLLECTOR_SETTLE_SECONDS    How long to keep verifying the AMS connection
                              after the stack is up (default 40).
  PULSE_IMAGE                 Override the container image.

All required flags are prompted interactively when a TTY is attached.
When stdin is not a TTY (CI, piped input) every required flag must be passed
on the command line; the installer exits with an error if any are absent.
EOF
}

# ── Parse flags ───────────────────────────────────────────────────────────────
AMS_URL=""
EMAIL=""
PASSWORD=""
LICENSE_KEY=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --ams-url)     AMS_URL="$2";     shift 2 ;;
    --email)       EMAIL="$2";       shift 2 ;;
    --password)    PASSWORD="$2";    shift 2 ;;
    --license-key) LICENSE_KEY="$2"; shift 2 ;;
    --help|-h)     usage; exit 0 ;;
    *)
      printf 'Unknown flag: %s\n\n' "$1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

# ── Interactive prompts (TTY only) ────────────────────────────────────────────
prompt_required() {
  local flag="$1" prompt_text="$2"
  if [[ -t 0 ]]; then
    local reply
    read -rp "${prompt_text}: " reply
    printf '%s' "$reply"
  else
    printf 'Error: %s is required (no TTY available for interactive prompt)\n' "$flag" >&2
    printf 'Run with --help for usage.\n' >&2
    exit 1
  fi
}

if [[ -z "$AMS_URL" ]];  then AMS_URL="$(prompt_required  "--ams-url"  "AMS REST URL (e.g. http://10.0.1.10:5080)")";  fi
if [[ -z "$EMAIL" ]];    then EMAIL="$(prompt_required    "--email"    "AMS admin email")";                             fi
if [[ -z "$PASSWORD" ]]; then PASSWORD="$(prompt_required "--password" "AMS admin password")";                         fi

[[ -z "$AMS_URL"  ]] && { printf 'Error: AMS URL is required.\n' >&2;   exit 1; }
[[ -z "$EMAIL" ]]    && { printf 'Error: AMS email is required.\n' >&2;  exit 1; }
[[ -z "$PASSWORD" ]] && { printf 'Error: AMS password is required.\n' >&2; exit 1; }

# ── Preflight: Docker accessibility ──────────────────────────────────────────
if ! command -v docker >/dev/null 2>&1; then
  printf 'Error: docker not found.\n' >&2
  printf '       Install Docker Engine 24+ from https://docs.docker.com/engine/install/\n' >&2
  exit 1
fi

# Distinguish "user not in docker group / daemon not running" from
# "Compose plugin not installed" — both make docker compose version fail
# but the remediation is completely different.
_DOCKER_INFO="$(docker info 2>&1 || true)"
if printf '%s' "$_DOCKER_INFO" | grep -qiE 'permission denied|cannot connect|dial unix'; then
  printf 'Error: Docker is installed but cannot be reached.\n' >&2
  printf '       Possible causes:\n' >&2
  printf '         • You are not in the "docker" group.\n' >&2
  printf "           Fix: sudo usermod -aG docker \$USER && newgrp docker\n" >&2
  printf '         • The Docker daemon is not running.\n' >&2
  printf '           Fix: sudo systemctl start docker\n' >&2
  printf '       Re-run this installer after fixing the issue.\n' >&2
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  printf 'Error: Docker Compose v2 not found.\n' >&2
  printf '       Install the Compose plugin: https://docs.docker.com/compose/install/\n' >&2
  exit 1
fi

# ── Fetch compose file if not co-located ─────────────────────────────────────
if [[ ! -f "$COMPOSE_FILE" ]]; then
  printf 'Downloading docker-compose.quickstart.yml...\n'
  curl -fsSL \
    "$REPO_RAW/deploy/quickstart/docker-compose.quickstart.yml" \
    -o "$COMPOSE_FILE"
fi

# ── Preflight: verify image is accessible before writing any secrets to disk ──
printf '\nChecking image access: %s\n' "$PULSE_IMAGE"
PULL_OUT="$(docker pull "$PULSE_IMAGE" 2>&1)" && PULL_RC=0 || PULL_RC=$?
if [[ $PULL_RC -ne 0 ]]; then
  if printf '%s' "$PULL_OUT" | grep -qiE '401|403|unauthorized|denied|not found|manifest unknown'; then
    printf '\n' >&2
    printf 'ERROR: The Pulse container image is not accessible from the registry.\n' >&2
    printf '\n' >&2
    printf '  Image : %s\n' "$PULSE_IMAGE" >&2
    printf '  Detail:\n' >&2
    printf '%s\n' "$PULL_OUT" | grep -iE '401|403|error|unauthorized|denied' | head -3 | sed 's/^/    /' >&2
    printf '\n' >&2
    printf 'This is a REGISTRY ACCESS problem — nothing is wrong with your Docker setup.\n' >&2
    printf 'The image is PUBLIC (no login needed), so this usually means the tag does not\n' >&2
    printf 'exist, a network/proxy is blocking ghcr.io, or GHCR is rate-limiting you.\n' >&2
    printf '\n' >&2
    printf 'Option 1 — Check the tag and connectivity:\n' >&2
    printf '  - Confirm the tag exists (image tags have NO v prefix, e.g. 0.4.1 not v0.4.1):\n' >&2
    printf '      https://github.com/aytekXR/ams-pulse/pkgs/container/ams-pulse\n' >&2
    printf '  - Retry the pull directly:  docker pull %s\n' "$PULSE_IMAGE" >&2
    printf '\n' >&2
    printf 'Option 2 — Build from source (no registry needed):\n' >&2
    printf '  git clone %s\n' "$REPO_WEB" >&2
    printf '  cd ams-pulse && make build\n' >&2
    printf '  PULSE_IMAGE=pulse:dev bash deploy/quickstart/install.sh ...\n' >&2
    printf '\n' >&2
    printf 'Still stuck? Open an issue: %s/issues\n' "$REPO_WEB" >&2
  else
    printf '\nERROR: Image pull failed:\n' >&2
    printf '%s\n' "$PULL_OUT" >&2
    printf '\nReport this: %s/issues\n' "$REPO_WEB" >&2
  fi
  exit 1
fi
printf 'Image OK.\n'

# ── Reuse operator-set values from an existing .env ──────────────────────────
# Extract single values with grep — do NOT `source` the file: it is a Compose
# env file, not a shell script. Unquoted values written by this script (an AMS
# password with a space, `$(…)`, or `;`) would be word-split or EXECUTED by the
# shell on re-run (REVIEW-MP3 N5). grep-extract is inert by construction.
#
# The anchor tolerates leading whitespace and an `export ` prefix (REVIEW-MP3-R12):
# a hand-edited .env with `export PULSE_SECRET_KEY=…` used to miss the strict
# `^PULSE_SECRET_KEY=` anchor, so the key read back EMPTY, a fresh one was minted,
# and the existing meta store became undecryptable — the same data loss the N5 fix
# was written to prevent.
env_value() {
  grep -E "^[[:space:]]*(export[[:space:]]+)?$1=" "$ENV_FILE" 2>/dev/null \
    | head -1 | cut -d= -f2- || true
}

if [[ -f "$ENV_FILE" ]]; then
  if [[ -z "${PULSE_SECRET_KEY:-}" ]]; then
    PULSE_SECRET_KEY="$(env_value PULSE_SECRET_KEY)"
  fi
  # Preserve an operator-added license key when --license-key was not passed —
  # the rewrite below would otherwise blank it on every re-run.
  if [[ -z "$LICENSE_KEY" ]]; then
    LICENSE_KEY="$(env_value PULSE_LICENSE_KEY)"
  fi
  # Preserve a SELF-SIGNED verification key (REVIEW-MP3-R11). The write below
  # stamped the official pubkey unconditionally, so every re-run silently
  # reverted the operator's own key and invalidated their self-signed licenses —
  # the same class of bug as N5, in the one case the code's own comment
  # ("do not change unless self-signing") anticipates.
  EXISTING_PUBKEY="$(env_value PULSE_LICENSE_PUBKEY)"
fi

# ── Generate PULSE_SECRET_KEY if not already in environment ──────────────────
if [[ -z "${PULSE_SECRET_KEY:-}" ]]; then
  PULSE_SECRET_KEY="$(openssl rand -hex 32)"
fi

# ── Official Pulse license verification key ──────────────────────────────────
OFFICIAL_PUBKEY="6403d7b49951d0220c7219e491b6525971edf10f0e64616b17023eab002ab4ba"
LICENSE_PUBKEY="${EXISTING_PUBKEY:-$OFFICIAL_PUBKEY}"
if [[ -n "${EXISTING_PUBKEY:-}" && "$EXISTING_PUBKEY" != "$OFFICIAL_PUBKEY" ]]; then
  printf 'Keeping the existing PULSE_LICENSE_PUBKEY (self-signed setup detected).\n'
fi

# ── Write .env ────────────────────────────────────────────────────────────────
# Mark as freshly created only when the file did not exist before this run;
# on re-runs the file already existed, so we must NOT delete it on a pre-start
# failure — the operator's secret key would be lost.
if [[ ! -f "$ENV_FILE" ]]; then
  CREATED_ENV=1
fi
{
  printf '# Pulse quickstart .env — generated by install.sh on %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  printf '# Keep this file private; it contains credentials and a secret key.\n'
  printf '\n'
  printf 'PULSE_SECRET_KEY=%s\n' "$PULSE_SECRET_KEY"
  printf 'PULSE_AMS_URL=%s\n'    "$AMS_URL"
  printf 'PULSE_AMS_LOGIN_EMAIL=%s\n'    "$EMAIL"
  printf 'PULSE_AMS_LOGIN_PASSWORD=%s\n' "$PASSWORD"
  printf 'PULSE_LICENSE_KEY=%s\n' "$LICENSE_KEY"
  # Official Pulse license verification key — do not change unless self-signing.
  # A self-signed key already present in .env is preserved across re-runs.
  printf 'PULSE_LICENSE_PUBKEY=%s\n' "$LICENSE_PUBKEY"
} >"$ENV_FILE"
chmod 600 "$ENV_FILE"
printf 'Wrote %s (mode 600)\n' "$ENV_FILE"

# ── Preflight: host port availability ────────────────────────────────────────
# Without this, a machine that already uses 8090 fails inside `docker compose up`
# with a raw daemon error ("Bind for 0.0.0.0:8090 failed: port is already
# allocated") and no hint that the port — not Pulse — is the problem
# (clean-room audit A2).
if command -v ss >/dev/null 2>&1; then
  _PORT_IN_USE="$(ss -ltn 2>/dev/null | awk -v p=":${PULSE_HOST_PORT}\$" '$4 ~ p {print $4}' | head -1)"
elif command -v netstat >/dev/null 2>&1; then
  _PORT_IN_USE="$(netstat -ltn 2>/dev/null | awk -v p=":${PULSE_HOST_PORT}\$" '$4 ~ p {print $4}' | head -1)"
else
  _PORT_IN_USE=""
fi
# A previous quickstart install is not a conflict — it is the re-run path (N5/R11/R12).
# Compose will recreate that container and re-bind the port itself, so a listener owned
# by THIS stack must not abort the install. Without this exemption, re-running the
# installer against a healthy install exits 1 on Pulse's own published port.
#
# Exempt on the PORT the stack actually publishes, not merely on the stack existing
# (round-5 review G-04): re-running with PULSE_HOST_PORT=18090 while an unrelated process
# owns 18090 and the quickstart stack is up on 8090 would otherwise skip the preflight and
# fail later with the raw daemon port-conflict this check exists to pre-empt.
if [[ -n "${_PORT_IN_USE:-}" ]]; then
  # `docker compose port` prints the host binding (e.g. "0.0.0.0:8090") for the service's
  # container port, or nothing when the stack is down. Error-tolerant: any failure leaves
  # _OWN_PORT empty and the conflict is reported, which is the safe direction.
  _OWN_PORT="$(docker compose -f "$COMPOSE_FILE" port pulse 8090 2>/dev/null | sed 's/.*://' | head -1)"
  if [[ -n "$_OWN_PORT" && "$_OWN_PORT" == "$PULSE_HOST_PORT" ]]; then
    printf '\nPort %s is published by an existing Pulse quickstart install — re-using it.\n' "$PULSE_HOST_PORT"
    printf 'This is a re-run: the stack will be recreated in place.\n'
    _PORT_IN_USE=""
  fi
fi
if [[ -n "${_PORT_IN_USE:-}" ]]; then
  printf '\nError: host port %s is already in use (%s).\n' "$PULSE_HOST_PORT" "$_PORT_IN_USE" >&2
  printf '       Pulse cannot publish its UI there. Either stop whatever is using it, or\n' >&2
  printf '       pick another port and re-run:\n' >&2
  printf '         PULSE_HOST_PORT=18090 bash install.sh --ams-url %s --email %s --password <pw>\n' "$AMS_URL" "$EMAIL" >&2
  exit 1
fi

# ── Preflight: the pinned compose file must be able to honour PULSE_HOST_PORT ──
# Releases up to and including v0.4.2 hardcode "8090:8090" in the quickstart compose,
# so PULSE_HOST_PORT is silently ignored on the curl-piped path (which fetches the
# compose from $PULSE_REF, not from this working tree). Failing loudly here beats
# health-polling a port nothing is listening on until the 90 s deadline expires.
if [[ "$PULSE_HOST_PORT" != "8090" ]] && ! grep -q 'PULSE_HOST_PORT' "$COMPOSE_FILE"; then
  printf '\nError: PULSE_HOST_PORT=%s was requested, but the compose file pinned to %s\n' \
    "$PULSE_HOST_PORT" "$PULSE_REF" >&2
  printf '       (%s) hardcodes port 8090 and cannot honour it.\n' "$COMPOSE_FILE" >&2
  printf '       Use a release whose quickstart compose parameterises the host port, e.g.:\n' >&2
  printf '         PULSE_REF=main PULSE_HOST_PORT=%s bash install.sh …\n' "$PULSE_HOST_PORT" >&2
  printf '       or free port 8090 and re-run without PULSE_HOST_PORT.\n' >&2
  exit 1
fi

# ── Start the stack ───────────────────────────────────────────────────────────
# Image is already in local cache from the preflight pull above.
printf '\nStarting Pulse stack...\n'
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d
STACK_STARTED=1

# ── Poll /healthz — NEVER claim success without positive body evidence ────────
# Accept both "ok" and "degraded" as proof Pulse is up: the server returns
# HTTP 200 with status "degraded" when the collector has no snapshot yet
# (e.g. wrong AMS credentials → no REST poll data). Only "down" (HTTP 503) or
# no response is a true failure.
printf '\nWaiting for Pulse to become healthy (up to %ss)...\n' "$HEALTHZ_DEADLINE"
DEADLINE=$(( $(date +%s) + HEALTHZ_DEADLINE ))
HEALTHY=0
LAST_BODY=""
while true; do
  LAST_BODY="$(curl -sf "http://localhost:${PULSE_HOST_PORT}/healthz" 2>/dev/null || true)"
  if printf '%s' "$LAST_BODY" | grep -qE '"status":"(ok|degraded)"'; then
    HEALTHY=1
    break
  fi
  if [[ $(date +%s) -ge $DEADLINE ]]; then
    break
  fi
  sleep 3
done

if [[ "$HEALTHY" -ne 1 ]]; then
  printf '\nERROR: Pulse did not report healthy within %ss.\n' "$HEALTHZ_DEADLINE" >&2
  printf '       Last /healthz response: %s\n' "${LAST_BODY:-<no response>}"  >&2
  printf '\nContainer logs (last 50 lines):\n' >&2
  docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" logs --tail=50 >&2
  exit 1
fi

# ── Diagnose collector degradation (AMS unreachable) — not a hard failure ────
# Extract only the collector component status; never match the whole body
# (other components' "ok" would mask a degraded collector).
collector_status_of() {
  if command -v python3 >/dev/null 2>&1; then
    printf '%s' "$1" \
      | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('components',{}).get('collector',{}).get('status',''))" \
      2>/dev/null || true
  elif command -v jq >/dev/null 2>&1; then
    printf '%s' "$1" | jq -r '.components.collector.status // ""' 2>/dev/null || true
  fi
}

# Let the collector's verdict SETTLE before reporting it (REVIEW-MP3-R14 companion,
# clean-room audit A3). The collector reports "ok" until the staleness floor
# (30 s, D-164) has elapsed since start-up, and the health loop above exits at the
# first 200 — typically ~10 s in. So an installer given a wrong AMS URL or wrong
# credentials printed a confident "Pulse is healthy." and the operator only met the
# failure seconds later in the UI. Re-poll until the floor has passed, or until the
# collector goes degraded on its own (whichever comes first).
_COLLECTOR_STATUS="$(collector_status_of "$LAST_BODY")"
if [[ "$_COLLECTOR_STATUS" != "degraded" ]]; then
  printf 'Verifying the AMS connection (up to %ss)...\n' "$COLLECTOR_SETTLE_SECONDS"
  SETTLE_DEADLINE=$(( $(date +%s) + COLLECTOR_SETTLE_SECONDS ))
  while [[ $(date +%s) -lt $SETTLE_DEADLINE ]]; do
    sleep 5
    BODY="$(curl -sf "http://localhost:${PULSE_HOST_PORT}/healthz" 2>/dev/null || true)"
    [[ -z "$BODY" ]] && continue
    LAST_BODY="$BODY"
    _COLLECTOR_STATUS="$(collector_status_of "$LAST_BODY")"
    [[ "$_COLLECTOR_STATUS" == "degraded" ]] && break
  done
fi

if [[ "$_COLLECTOR_STATUS" == "degraded" ]]; then
  printf '\n'
  printf '============================================================\n'
  printf '  WARNING: Pulse is installed and running, but the\n'
  printf '  collector cannot reach AMS at: %s\n' "$AMS_URL"
  printf '\n'
  printf '  The stack IS installed — do not re-run install.sh.\n'
  printf '  Check your AMS URL, admin email, and password, then\n'
  printf '  update %s and restart:\n' "$ENV_FILE"
  printf '    docker compose -f %s --env-file %s up -d\n' "$COMPOSE_FILE" "$ENV_FILE"
  printf '============================================================\n'
else
  printf 'Pulse is healthy.\n'
fi

# ── Stack is up — keep the .env ───────────────────────────────────────────────
# Disable the failure-cleanup trap now that the run has succeeded.
trap - EXIT

# ── Extract bootstrap admin token ─────────────────────────────────────────────
printf '\nLooking for bootstrap admin token...\n'
TOKEN="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" logs pulse 2>/dev/null \
  | grep 'FIRST RUN' | grep -oE 'plt_[a-f0-9]+' | head -1 || true)"

if [[ -n "$TOKEN" ]]; then
  printf '\n'
  printf '======================================================================\n'
  printf '  FIRST RUN — Admin bootstrap token (shown ONCE — save it now):\n'
  printf '\n'
  printf '    %s\n' "$TOKEN"
  printf '\n'
  printf '  UI:  http://localhost:%s\n' "$PULSE_HOST_PORT"
  printf '======================================================================\n'
else
  printf '\nNo bootstrap token found in logs — expected on a re-run.\n'
  printf 'The token is printed only on the very first boot (when no admin tokens exist).\n'
  printf 'To generate a replacement: POST /api/v1/admin/tokens with an existing admin token,\n'
  printf 'or delete the pulse-data volume and re-run install.sh for a fresh first boot.\n'
fi

# ── Next steps ────────────────────────────────────────────────────────────────
printf '\nNext steps:\n'
printf '  1. Open http://localhost:%s and enter the admin token shown above.\n' "$PULSE_HOST_PORT"
printf '  2. Complete the 4-step onboarding wizard (welcome → source → verify → done).\n'
if [[ -z "$LICENSE_KEY" ]]; then
  printf '  3. Free tier active (1 AMS node, 7-day retention).\n'
  printf '     To upgrade: paste a trial key in Settings → License in the UI,\n'
  printf '     or set PULSE_LICENSE_KEY in %s and re-run:\n' "$ENV_FILE"
  printf '       docker compose -f %s --env-file %s up -d\n' "$COMPOSE_FILE" "$ENV_FILE"
fi
printf '\nFor more: %s/blob/main/docs/runbooks/install.md\n' "$REPO_WEB"
