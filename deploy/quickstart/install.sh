#!/usr/bin/env bash
# Pulse quickstart installer — D-089 RULE-2
#
# One-command install that:
#   1. Checks for Docker + Compose v2
#   2. Collects AMS connection details (flags or interactive prompts)
#   3. Generates PULSE_SECRET_KEY if absent
#   4. Writes .env (chmod 600)
#   5. Downloads docker-compose.quickstart.yml if not co-located
#   6. Brings up the stack
#   7. Polls /healthz for up to 90 s (fails hard on timeout — NEVER claims success)
#   8. Extracts the bootstrap admin token from container logs (honest re-run handling)
#
# Usage:
#   bash install.sh --ams-url http://10.0.1.10:5080 --email admin@example.com --password <pw>
#   bash install.sh --help
#
# Supports both in-repo execution and curl|bash piped install:
#   curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/install.sh | bash

set -euo pipefail

REPO_RAW="https://raw.githubusercontent.com/aytekXR/ams-pulse/main"
HEALTHZ_DEADLINE=90   # seconds

# ── Locate working directory ──────────────────────────────────────────────────
# When executed via `curl ... | bash`, BASH_SOURCE[0] is empty or set to "bash".
# In that case create a `quickstart/` subdirectory of cwd so output files land
# somewhere predictable and the operator knows where to find them.
SCRIPT_SOURCE="${BASH_SOURCE[0]:-}"
if [[ -n "$SCRIPT_SOURCE" && "$SCRIPT_SOURCE" != "bash" && -f "$SCRIPT_SOURCE" ]]; then
  WORK_DIR="$(cd "$(dirname "$SCRIPT_SOURCE")" && pwd)"
else
  WORK_DIR="$(pwd)/quickstart"
  mkdir -p "$WORK_DIR"
fi

COMPOSE_FILE="$WORK_DIR/docker-compose.quickstart.yml"
ENV_FILE="$WORK_DIR/.env"

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
  # $1 = flag name (e.g. "--ams-url")
  # $2 = prompt text
  # $3 = current value (passed by the caller — printed ref only, cannot modify)
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

# ── Preflight checks ──────────────────────────────────────────────────────────
if ! command -v docker >/dev/null 2>&1; then
  printf 'Error: docker not found.\n' >&2
  printf '       Install Docker Engine 24+ from https://docs.docker.com/engine/install/\n' >&2
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

# ── Generate PULSE_SECRET_KEY if not already in environment ──────────────────
PULSE_SECRET_KEY="${PULSE_SECRET_KEY:-}"
if [[ -z "$PULSE_SECRET_KEY" ]]; then
  PULSE_SECRET_KEY="$(openssl rand -hex 32)"
fi

# ── Write .env ────────────────────────────────────────────────────────────────
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
  printf 'PULSE_LICENSE_PUBKEY=%s\n' \
    "6403d7b49951d0220c7219e491b6525971edf10f0e64616b17023eab002ab4ba"
} >"$ENV_FILE"
chmod 600 "$ENV_FILE"
printf 'Wrote %s (mode 600)\n' "$ENV_FILE"

# ── Start the stack ───────────────────────────────────────────────────────────
printf '\nStarting Pulse stack (this pulls images on first run)...\n'
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d

# ── Poll /healthz — NEVER claim success without positive body evidence ────────
printf '\nWaiting for Pulse to become healthy (up to %ss)...\n' "$HEALTHZ_DEADLINE"
DEADLINE=$(( $(date +%s) + HEALTHZ_DEADLINE ))
HEALTHY=0
LAST_BODY=""
while true; do
  LAST_BODY="$(curl -sf http://localhost:8090/healthz 2>/dev/null || true)"
  if printf '%s' "$LAST_BODY" | grep -q '"status":"ok"'; then
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
printf 'Pulse is healthy.\n'

# ── Extract bootstrap admin token ─────────────────────────────────────────────
printf '\nLooking for bootstrap admin token...\n'
TOKEN="$(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" logs pulse 2>/dev/null \
  | grep -oE 'plt_[a-f0-9]+' | head -1 || true)"

if [[ -n "$TOKEN" ]]; then
  printf '\n'
  printf '======================================================================\n'
  printf '  FIRST RUN — Admin bootstrap token (shown ONCE — save it now):\n'
  printf '\n'
  printf '    %s\n' "$TOKEN"
  printf '\n'
  printf '  UI:  http://localhost:8090\n'
  printf '======================================================================\n'
else
  printf '\nNo bootstrap token found in logs — expected on a re-run.\n'
  printf 'The token is printed only on the very first boot (when no admin tokens exist).\n'
  printf 'To generate a replacement: POST /api/v1/admin/tokens with an existing admin token,\n'
  printf 'or delete the pulse-data volume and re-run install.sh for a fresh first boot.\n'
fi

# ── Next steps ────────────────────────────────────────────────────────────────
printf '\nNext steps:\n'
printf '  1. Open http://localhost:8090 and enter the admin token shown above.\n'
printf '  2. Complete the 4-step onboarding wizard (welcome → source → verify → done).\n'
if [[ -z "$LICENSE_KEY" ]]; then
  printf '  3. Free tier active (1 AMS node, 7-day retention).\n'
  printf '     To upgrade: paste a trial key in Settings → License in the UI,\n'
  printf '     or set PULSE_LICENSE_KEY in %s and re-run:\n' "$ENV_FILE"
  printf '       docker compose -f %s --env-file %s up -d\n' "$COMPOSE_FILE" "$ENV_FILE"
fi
printf '\nFor more: docs/runbooks/install.md\n'
