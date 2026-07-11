#!/usr/bin/env bash
# qa/realams/harness/env.sh
#
# SOURCE this file — do not execute directly.
# Exports all harness-wide variables; does NOT create per-scenario dirs.
#
# Usage:
#   source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
#
# Key env vars consumed:
#   PULSE_TARGET   realams (default) | prod
#   PULSE_TOKEN    bearer token override (required for prod; auto-extracted for realams)
#
set -euo pipefail

# ── Locate repo root (env.sh lives at qa/realams/harness/env.sh) ──────────────
_HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${_HARNESS_DIR}/../../.." && pwd)"

# ── Target selection ───────────────────────────────────────────────────────────
PULSE_TARGET="${PULSE_TARGET:-realams}"

if [ "$PULSE_TARGET" = "prod" ]; then
  PULSE_URL="https://beyondkaira.com/api/v1"
  PULSE_WS="wss://beyondkaira.com/api/v1/live/ws"
  PULSE_HEALTH_URL="https://beyondkaira.com/healthz"
else
  PULSE_URL="http://127.0.0.1:18090/api/v1"
  PULSE_WS="ws://127.0.0.1:18090/api/v1/live/ws"
  PULSE_HEALTH_URL="http://127.0.0.1:18090/healthz"
fi

# ── AMS URL (from deploy/.env, key: PULSE_AMS_URL) ────────────────────────────
AMS_URL="$(grep '^PULSE_AMS_URL=' "${REPO_ROOT}/deploy/.env" | cut -d= -f2-)"
if [ -z "$AMS_URL" ]; then
  echo "[env] ERROR: PULSE_AMS_URL not found in deploy/.env" >&2
  exit 1
fi

# ── Evidence root (gitignored dir; NOT per-scenario) ──────────────────────────
EVIDENCE_ROOT="${REPO_ROOT}/qa/realams/evidence"
mkdir -p "$EVIDENCE_ROOT"

# Cookie file lives in evidence (gitignored) — never in repo-tracked space
AMS_COOKIE_FILE="${EVIDENCE_ROOT}/.ams-cookie"

# ── Pulse API token ───────────────────────────────────────────────────────────
# Precedence: env override > realams auto-extract > prod must-provide
if [ -n "${PULSE_TOKEN:-}" ]; then
  : # use the override as-is
elif [ "$PULSE_TARGET" = "realams" ]; then
  PULSE_TOKEN="$(sg docker -c "docker logs pulse-realams-pulse-1 2>&1" \
    | grep -oE 'plt_[a-f0-9]+' | head -1 || true)"
  if [ -z "${PULSE_TOKEN:-}" ]; then
    echo "[env] ERROR: could not auto-extract token from pulse-realams-pulse-1 logs." >&2
    echo "[env]   Make sure the container is running:" >&2
    echo "[env]     sg docker -c \"docker logs pulse-realams-pulse-1 2>&1\" | grep plt_" >&2
    exit 1
  fi
  echo "[env] Auto-extracted realams token: ${PULSE_TOKEN:0:12}..." >&2
else
  # prod target: token must be set in the environment — never stored in files
  echo "[env] ERROR: PULSE_TARGET=prod requires PULSE_TOKEN in the environment." >&2
  echo "[env]   Get the token from oguz-testing.md line 159 and run:" >&2
  echo "[env]     PULSE_TOKEN=plt_... PULSE_TARGET=prod source qa/realams/harness/env.sh" >&2
  echo "[env]   NEVER commit token values to any file." >&2
  exit 1
fi

# ── Export ─────────────────────────────────────────────────────────────────────
export PULSE_TARGET
export PULSE_URL PULSE_WS PULSE_HEALTH_URL
export AMS_URL
export AMS_COOKIE_FILE
export PULSE_TOKEN
export EVIDENCE_ROOT
export REPO_ROOT

echo "[env] target=$PULSE_TARGET  PULSE_URL=$PULSE_URL  AMS_URL=$AMS_URL" >&2
echo "[env] EVIDENCE_ROOT=$EVIDENCE_ROOT" >&2
