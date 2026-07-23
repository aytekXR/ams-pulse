#!/usr/bin/env bash
# =============================================================================
# deployment.sh — thin, self-contained ON-HOST deploy for the Pulse compose
# stack behind the host-nginx edge.
#
# SCOPE. This is app-deploy tooling, not a rewrite of how Pulse runs. Pulse
# stays a docker-compose stack (Go server + ClickHouse + Kafka). This script
# does exactly: build -> up (deploy/docker-compose.prod.yml, loopback-published)
# -> health-gate the app on its PRIVATE loopback port. It does NOT touch nginx
# or :443 — host nginx owns the edge (vhosts in deploy/nginx/, TLS via certbot,
# cert at /etc/letsencrypt/live/beyondkaira.com/) and is reloaded separately by
# the owner. Only the app containers are reconciled (a brief app recreate onto
# loopback). Run it on every app redeploy to prove the app answers on 127.0.0.1.
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
#   3. build      docker compose build, WITH the version stamps (D-058)
#   4. apply      docker compose up -d   (brings up the loopback-published stack)
#   5. health     bounded curl loop vs the PRIVATE loopback /healthz, real signal
#   6. collector  after the staleness window, assert Pulse is actually COLLECTING
#                 A failure at 4, 5 or 6 rolls back (see rollback()).
#
# CONFIG (env; never printed):
#   PULSE_PROJECT      compose project name          default: pulse-prod
#   PULSE_ENV_FILE     --env-file path (gitignored)  default: <repo>/deploy/.env
#   HEALTH_URL         private health probe          default: http://127.0.0.1:8090/healthz
#   HEALTH_EXPECT      substring the body must have  default: "components"
#   HEALTH_TIMEOUT     seconds before hard-fail      default: 90
#   COLLECTOR_GRACE    seconds to wait before the    default: 35
#                      freshness assert; 0 skips it
#   DOCKER_SG          "1" wraps compose in `sg docker -c` (host w/o docker group)
#
# EXIT: 0 built+up+healthy+collecting · 1 failed (rolled back) · 64 not configured · 2 usage
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

# THE CANONICAL OVERLAY SET — keep in sync with deploy/runbooks/upgrade-rollback.md
# ("Canonical compose command"). Copy it as-is; NEVER omit an overlay.
#
# D-164 (2026-07-23): this list held ONLY docker-compose.prod.yml. Running the
# script therefore recreated the prod app from the base file alone, which
# silently
#   - reverted PULSE_AMS_URL to the built-in mock-ams default (an unreachable
#     host in a real deployment) => the collector went blind for 7 h 46 m, and
#   - dropped PULSE_LICENSE_KEY, which ONLY real-ams.yml maps => the server
#     degraded to the Free tier, disabling reports, probes, anomalies and the
#     data API.
# Both failures are silent by design (the server never crashes on a bad licence
# or an unreachable AMS), so the overlay set is load-bearing, not cosmetic.
COMPOSE_FILES=(
  -f "$ROOT/deploy/docker-compose.prod.yml"      # app + ClickHouse, hardening, loopback publish
  -f "$ROOT/deploy/docker-compose.real-ams.yml"  # real AMS wiring + PULSE_LICENSE_KEY mapping
  -f "$ROOT/deploy/docker-compose.backup.yml"    # 24 h backup sidecar
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
    # "$@" (not $*): the build step passes --build-arg VERSION=... values that
    # must survive as single words. SC2048.
    docker compose -p "$PULSE_PROJECT" "${COMPOSE_FILES[@]}" \
      --env-file "$PULSE_ENV_FILE" "$@"
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
# `compose config` validates the rendered config (catches a bad override/env ref)
# without starting anything — safe in --check too.
if ! compose config >/dev/null; then
  oops "docker compose config failed — the compose file is invalid or env is missing keys."
  exit 64
fi
info "compose config valid · project=$PULSE_PROJECT · health=$HEALTH_URL"

if [ "$CHECK_ONLY" -eq 1 ]; then
  say "--check: configuration valid, changing nothing."
  info "would run: build (stamped) -> up -d -> health-gate $HEALTH_URL (expect \"$HEALTH_EXPECT\")"
  info "            -> collector-freshness assert after ${COLLECTOR_GRACE:-35}s"
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
# leave it as the owner had it and report. Either way the :443 edge (host nginx)
# is UNTOUCHED — this script never reloads nginx — though a dead app upstream
# means nginx serves 502s until the app is healthy again.
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
# Version stamps must be passed HERE, to `compose build`. `compose up --build`
# does not forward --build-arg, which is why the image that shipped at D-164
# reported `pulse dev (commit unknown)` and prod could not name the code it was
# running. (D-058 lesson b; deploy/runbooks/upgrade-rollback.md Step 3.)
STAMP_VERSION="$(git -C "$ROOT" describe --tags --always 2>/dev/null || echo dev)"
STAMP_COMMIT="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
STAMP_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
info "stamping $STAMP_VERSION (commit $STAMP_COMMIT)"
if ! compose build \
  --build-arg "VERSION=$STAMP_VERSION" \
  --build-arg "COMMIT=$STAMP_COMMIT" \
  --build-arg "BUILD_DATE=$STAMP_DATE"; then
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

# ---------------------------------------------------------------------------
# 6. collector freshness (D-164)
# ---------------------------------------------------------------------------
# WHY THIS STEP EXISTS. Step 5 proves Pulse ANSWERS. It cannot prove Pulse is
# COLLECTING: /healthz reports the whole document "ok" during a cold-start grace
# window, and before D-164 the collector component was a pure liveness proxy
# that stayed "ok" forever even when every AMS poll failed. That is exactly how
# a deploy with the wrong PULSE_AMS_URL passed its health gate and ran blind for
# 7 h 46 m. So: wait past the staleness window, THEN require the document to be
# "ok" — with D-164 that is true only if a real AMS poll has recently succeeded.
#
# COLLECTOR_GRACE=0 skips this check — the documented escape hatch for
# deploying Pulse while AMS is deliberately down (maintenance).
COLLECTOR_GRACE="${COLLECTOR_GRACE:-35}"   # > the 30 s staleAfter floor
if [ "$COLLECTOR_GRACE" -gt 0 ]; then
  say "6/6 collector freshness (waiting ${COLLECTOR_GRACE}s for a real AMS poll)"
  sleep "$COLLECTOR_GRACE"
  body="$(curl -fsS -m 5 "$HEALTH_URL" 2>/dev/null || true)"
  if printf '%s' "$body" | grep -q '"status":"ok"'; then
    info "collector is fresh: AMS polls are landing."
  else
    oops "collector is NOT fresh after ${COLLECTOR_GRACE}s — Pulse answers but is not collecting."
    oops "  /healthz says: ${body:-<no response>}"
    oops "  Most likely the AMS wiring is wrong (PULSE_AMS_URL / credentials in"
    oops "  $PULSE_ENV_FILE), or AMS itself is down. Check: compose logs pulse | grep restpoller"
    oops "  Deploying anyway (set COLLECTOR_GRACE=0) means shipping a blind monitor."
    rollback
    exit 1
  fi
fi

say "done — Pulse is up, healthy on its private loopback port, and collecting."
info "host nginx (the live edge) proxies to it; no nginx action needed for an app redeploy."
