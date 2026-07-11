#!/usr/bin/env bash
# qa/realams/harness/auth.sh
#
# Idempotent AMS cookie-session authentication.
# Source or execute; safe to call multiple times in one session.
#
# CRITICAL CONSTRAINT: AMS brute-force lockout is 2 failed attempts → 5-min lock
# keyed by EMAIL (not IP). This script attempts login EXACTLY ONCE on failure
# and exits 1. NEVER call this in a retry loop.
#
# Validity check: a lightweight server-scope GET of
#   $AMS_URL/rest/v2/applications
# must return a JSON array. Server-scope endpoints require the cookie session.
#
set -euo pipefail

# ── Bootstrap ─────────────────────────────────────────────────────────────────
_AUTH_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./env.sh
source "${_AUTH_DIR}/env.sh"

# ── Credentials from deploy/.env ──────────────────────────────────────────────
_DOTENV="${REPO_ROOT}/deploy/.env"
AMS_EMAIL="$(grep '^PULSE_AMS_LOGIN_EMAIL=' "${_DOTENV}" | cut -d= -f2-)"
AMS_PASS="$(grep '^PULSE_AMS_LOGIN_PASSWORD=' "${_DOTENV}" | cut -d= -f2-)"

if [ -z "$AMS_EMAIL" ] || [ -z "$AMS_PASS" ]; then
  echo "[auth] ERROR: PULSE_AMS_LOGIN_EMAIL or PULSE_AMS_LOGIN_PASSWORD" \
       "not found in deploy/.env" >&2
  exit 1
fi

# ── Idempotency: reuse cookie if still valid ──────────────────────────────────
_ams_session_valid() {
  # A valid server-scope session returns JSON from /rest/v2/applications
  # containing an "applications" key (AMS 3.0.3 returns {"applications":[...]}).
  if [ ! -f "$AMS_COOKIE_FILE" ]; then
    return 1
  fi
  local result
  result="$(curl -s -m 10 -b "$AMS_COOKIE_FILE" \
    "${AMS_URL}/rest/v2/applications" 2>/dev/null || true)"
  # Valid: JSON object with "applications" key, or a bare JSON array
  case "$result" in
    *'"applications"'*) return 0 ;;
    '['*)               return 0 ;;
    *)                  return 1 ;;
  esac
}

if _ams_session_valid; then
  echo "[auth] Existing AMS session is valid — reusing cookie (${AMS_COOKIE_FILE})" >&2
  exit 0
fi

# ── Single login attempt ───────────────────────────────────────────────────────
echo "[auth] Authenticating to AMS as ${AMS_EMAIL}" >&2
mkdir -p "$(dirname "$AMS_COOKIE_FILE")"

RESP="$(curl -s -m 15 \
  -c "$AMS_COOKIE_FILE" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${AMS_EMAIL}\",\"password\":\"${AMS_PASS}\"}" \
  "${AMS_URL}/rest/v2/users/authenticate" 2>/dev/null || true)"

SUCCESS="$(printf '%s' "$RESP" | jq -r '.success // false' 2>/dev/null || echo false)"

if [ "$SUCCESS" = "true" ]; then
  echo "[auth] OK — AMS session established" >&2
else
  echo "[auth] FAIL: AMS authentication returned: ${RESP}" >&2
  echo "" >&2
  echo "[auth] !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!" >&2
  echo "[auth] LOCKOUT WARNING: AMS allows only 2 failed login attempts" >&2
  echo "[auth] before locking the account for 5 minutes (keyed by email)." >&2
  echo "[auth] admin@ is also Pulse's polling account — a lockout breaks" >&2
  echo "[auth] prod polling. DO NOT retry. Wait 5 min before any next" >&2
  echo "[auth] attempt, then re-check credentials in deploy/.env." >&2
  echo "[auth] !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!" >&2
  # Remove any partial cookie to prevent cached-failure confusion
  rm -f "$AMS_COOKIE_FILE"
  exit 1
fi
