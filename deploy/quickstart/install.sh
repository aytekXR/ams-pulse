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
#   6. Writes .env (chmod 600); removed automatically if the run fails
#   7. Brings up the stack
#   8. Polls /healthz for up to 90 s (fails hard on timeout — NEVER claims success)
#   9. Extracts the bootstrap admin token from container logs (honest re-run handling)
#
# Usage:
#   bash install.sh --ams-url http://10.0.1.10:5080 --email admin@example.com --password <pw>
#   bash install.sh --help
#
# Supports both in-repo execution and curl|bash piped install:
#   curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/install.sh | bash

set -euo pipefail

REPO_RAW="https://raw.githubusercontent.com/aytekXR/ams-pulse/main"
REPO_WEB="https://github.com/aytekXR/ams-pulse"
HEALTHZ_DEADLINE=90   # seconds
export PULSE_IMAGE="${PULSE_IMAGE:-ghcr.io/aytekxr/ams-pulse:0.4.0}"

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

# ── Cleanup on failure ────────────────────────────────────────────────────────
# Remove the .env if the script exits with a non-zero status so a failed run
# does not leave a file containing a generated secret key on disk.
cleanup() {
  local rc=$?
  if [[ $rc -ne 0 && -f "$ENV_FILE" ]]; then
    rm -f "$ENV_FILE"
    printf '[cleanup] Removed %s (not leaving secrets from a failed run).\n' "$ENV_FILE" >&2
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
    printf 'The image is currently private on GHCR and requires authentication to pull.\n' >&2
    printf '\n' >&2
    printf 'Option 1 — Authenticate with a GitHub Personal Access Token:\n' >&2
    printf '  1. Create a PAT at https://github.com/settings/tokens/new\n' >&2
    printf '     (classic, read:packages scope is sufficient)\n' >&2
    printf '  2. Log in:  docker login ghcr.io -u YOUR_GITHUB_USERNAME\n' >&2
    printf '     (paste your PAT when prompted for a password)\n' >&2
    printf '  3. Re-run this installer.\n' >&2
    printf '\n' >&2
    printf 'Option 2 — Build from source (no registry credentials needed):\n' >&2
    printf '  git clone %s\n' "$REPO_WEB" >&2
    printf '  cd ams-pulse && make build\n' >&2
    printf '  PULSE_IMAGE=pulse:dev bash deploy/quickstart/install.sh ...\n' >&2
    printf '\n' >&2
    printf 'Need access or help? Open an issue: %s/issues\n' "$REPO_WEB" >&2
  else
    printf '\nERROR: Image pull failed:\n' >&2
    printf '%s\n' "$PULL_OUT" >&2
    printf '\nReport this: %s/issues\n' "$REPO_WEB" >&2
  fi
  exit 1
fi
printf 'Image OK.\n'

# ── Generate PULSE_SECRET_KEY if not already in environment ──────────────────
if [[ -z "${PULSE_SECRET_KEY:-}" ]]; then
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
# Image is already in local cache from the preflight pull above.
printf '\nStarting Pulse stack...\n'
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
printf '\nFor more: %s/blob/main/docs/runbooks/install.md\n' "$REPO_WEB"
