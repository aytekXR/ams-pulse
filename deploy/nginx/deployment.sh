#!/usr/bin/env bash
# =============================================================================
# deployment.sh — thin, self-contained ON-HOST deploy for the Pulse compose
# stack behind the new host-nginx edge.
#
# SCOPE. This is EDGE-migration tooling, not a rewrite of how Pulse runs. Pulse
# stays a docker-compose stack (Go server + ClickHouse + Kafka). This script
# does exactly: build -> up (with the loopback-publish overlay) -> health-gate
# the app on its PRIVATE loopback port. It does NOT touch nginx, :443, or Caddy
# — the edge cutover is owner-run and windowed (see deploy/MIGRATION.md). Run it
# to prove the app answers on 127.0.0.1 BEFORE you flip the edge, and on every
# app redeploy after.
#
# SELF-CONTAINED. No bin/repo, no repo_cli, no external health-check.sh. The
# health gate below is its own bounded curl loop asserting a REAL signal (the
# `"components"` object that only Pulse's /healthz emits — not merely "it
# answered") with a hard-fail timeout.
#
# THE `set -e` RULE THIS FILE IS SHAPED AROUND. Under `set -euo pipefail` a bare
# failing command exits the script, so a rollback written as `step; if [ $? -ne 0
# ]...` is dead code. Every failure path here is `if ! <step>; then rollback;
# exit 1; fi`, because a command in an `if` condition is exempt from `set -e`.
# (This is the single most important lesson carried over from yanki's deploy.sh.)
#
# ORDER
#   1. preflight  tools + files present, env file present, compose config valid
#   2. last-good  record whether the project is already running (rollback needs it)
#   3. build      docker compose build
#   4. apply      docker compose up -d   (brings up loopback-published stack)
#   5. health     bounded curl loop vs the PRIVATE loopback /healthz, real signal
#                 A failure at 4 or 5 rolls back (see rollback()).
#
# CONFIG (env; never printed):
#   PULSE_PROJECT      compose project name          default: pulse-prod
#   PULSE_ENV_FILE     --env-file path (gitignored)  default: <repo>/deploy/.env
#   HEALTH_URL         private health probe          default: http://127.0.0.1:8090/healthz
#   HEALTH_EXPECT      substring the body must have  default: "components"
#   HEALTH_TIMEOUT     seconds before hard-fail      default: 90
#   DOCKER_SG          "1" wraps compose in `sg docker -c` (host w/o docker group)
#
# EXIT: 0 built+up+healthy · 1 failed (rolled back) · 64 not configured · 2 usage
# =============================================================================
set -euo pipefail

ROOT="$(cd -P "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

CHECK_ONLY=0
while [ $# -gt 0 ]; do
  case "$1" in
    --check | --dry-run) CHECK_ONLY=1 ;;
    -h | --help)
      grep -E '^#( |$)' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) printf 'deployment: unknown argument %s\n' "$1" >&2; exit 2 ;;
  esac
  shift
done

say()  { printf '\n== %s\n' "$1"; }
info() { printf '   %s\n' "$1"; }
oops() { printf 'deployment: %s\n' "$1" >&2; }

PULSE_PROJECT="${PULSE_PROJECT:-pulse-prod}"
PULSE_ENV_FILE="${PULSE_ENV_FILE:-$ROOT/deploy/.env}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8090/healthz}"
HEALTH_EXPECT="${HEALTH_EXPECT:-components}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-90}"
DOCKER_SG="${DOCKER_SG:-0}"

COMPOSE_FILES=(
  -f "$ROOT/deploy/docker-compose.yml"
  -f "$ROOT/deploy/docker-compose.hardened.yml"
  -f "$ROOT/deploy/docker-compose.nginx-edge.yml"
)

# compose <args...> — one wrapper so the `sg docker` path is a faithful rehearsal
# of the plain path. Everything that touches Docker goes through here.
compose() {
  if [ "$DOCKER_SG" = "1" ]; then
    # Build a single shell string for `sg docker -c`. Args here are fixed/literal
    # (no untrusted input), so simple quoting is sufficient.
    local cmd="docker compose -p $PULSE_PROJECT"
    local f
    for f in "${COMPOSE_FILES[@]}"; do cmd="$cmd $f"; done
    cmd="$cmd --env-file $PULSE_ENV_FILE $*"
    sg docker -c "$cmd"
  else
    docker compose -p "$PULSE_PROJECT" "${COMPOSE_FILES[@]}" \
      --env-file "$PULSE_ENV_FILE" $*
  fi
}

# ---------------------------------------------------------------------------
# 1. preflight
# ---------------------------------------------------------------------------
say "1/5 preflight"
missing=()
command -v docker >/dev/null 2>&1 || missing+=("docker")
command -v curl   >/dev/null 2>&1 || missing+=("curl")
if [ "${#missing[@]}" -gt 0 ]; then
  oops "required tools not found: ${missing[*]}"
  exit 64
fi
for f in "${COMPOSE_FILES[@]}"; do
  [ "$f" = "-f" ] && continue
  if [ ! -f "$f" ]; then oops "compose file not found: $f"; exit 64; fi
done
if [ ! -f "$PULSE_ENV_FILE" ]; then
  oops "env file not found: $PULSE_ENV_FILE"
  oops "  copy deploy/.env.example -> deploy/.env and fill real values (gitignored)."
  exit 64
fi
# `compose config` validates the merged overlay (catches a bad override/env ref)
# without starting anything — safe in --check too.
if ! compose config >/dev/null; then
  oops "docker compose config failed — the merged overlay is invalid or env is missing keys."
  exit 64
fi
info "compose config valid · project=$PULSE_PROJECT · health=$HEALTH_URL"

if [ "$CHECK_ONLY" -eq 1 ]; then
  say "--check: configuration valid, changing nothing."
  info "would run: build -> up -d -> health-gate $HEALTH_URL (expect \"$HEALTH_EXPECT\")"
  exit 0
fi

# ---------------------------------------------------------------------------
# 2. last-good (record current state BEFORE mutating)
# ---------------------------------------------------------------------------
say "2/5 last-good"
WAS_RUNNING=0
if [ -n "$(compose ps -q 2>/dev/null)" ]; then WAS_RUNNING=1; fi
info "project was $([ "$WAS_RUNNING" -eq 1 ] && echo 'already running' || echo 'not running')"

# rollback — used by the `if !` guards below. Honest about a compose topology:
# if we started the stack fresh and it failed to come healthy, take it back down
# so no half-broken loopback stack is left listening; if it was already running,
# leave it as the owner had it and report. Either way the live :443 edge is
# UNTOUCHED (Caddy still owns :443 until the manual cutover), so a failure here
# never takes the public site down.
rollback() {
  oops "rolling back — the live edge (:443) was not touched by this script."
  if [ "$WAS_RUNNING" -eq 0 ]; then
    oops "  stack was started fresh; bringing it down."
    compose down || oops "  'compose down' also failed — inspect: compose ps / compose logs"
  else
    oops "  stack was already running; left in place. Inspect: compose logs pulse"
    oops "  to restore the prior code, 'git checkout <last-good-sha>' and re-run this script."
  fi
}

# ---------------------------------------------------------------------------
# 3. build
# ---------------------------------------------------------------------------
say "3/5 build"
if ! compose build; then
  oops "build failed"
  rollback
  exit 1
fi

# ---------------------------------------------------------------------------
# 4. apply
# ---------------------------------------------------------------------------
say "4/5 apply (up -d, loopback-published)"
if ! compose up -d; then
  oops "compose up failed"
  rollback
  exit 1
fi

# ---------------------------------------------------------------------------
# 5. health (bounded curl loop, real signal, private loopback)
# ---------------------------------------------------------------------------
say "5/5 health"
health_ok() {
  local deadline=$(( SECONDS + HEALTH_TIMEOUT ))
  local code body
  while [ "$SECONDS" -lt "$deadline" ]; do
    # -m caps a single hung request; body captured, status code separated.
    body="$(curl -fsS -m 5 "$HEALTH_URL" 2>/dev/null || true)"
    code="$(curl -s -o /dev/null -m 5 -w '%{http_code}' "$HEALTH_URL" 2>/dev/null || true)"
    if [ "$code" = "200" ] && printf '%s' "$body" | grep -q "$HEALTH_EXPECT"; then
      info "healthy: HTTP 200 and body contains \"$HEALTH_EXPECT\""
      return 0
    fi
    info "not ready yet (code=${code:-none}); retrying..."
    sleep 3
  done
  oops "health gate failed after ${HEALTH_TIMEOUT}s: last code=${code:-none}"
  oops "  a 200 WITHOUT \"$HEALTH_EXPECT\" means something else is answering $HEALTH_URL,"
  oops "  not Pulse — do NOT cut the edge over to it."
  return 1
}
if ! health_ok; then
  rollback
  exit 1
fi

say "done — Pulse is up and healthy on its private loopback port."
info "next: run the owner nginx edge cutover in deploy/MIGRATION.md (§ cutover)."
