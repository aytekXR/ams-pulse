#!/usr/bin/env bash
# qa/realams/harness/failures.sh
#
# Failure injection helpers.
# SOURCE this file from scenario scripts; do not execute directly.
#
# Functions exported:
#   inject_publisher_kill  ID
#     Abruptly kills a publisher container (delegates to kill_publisher).
#     AMS result: terminated_unexpectedly.
#
#   inject_invalid_stream_key  APP
#     Starts a short-lived (20 s), low-res ffmpeg publisher with a garbage
#     stream key. Self-cleans via --rm + -t 20 time limit.
#     Container name: val-inv-key-<EPOCH>
#     Prints the stream key used to stdout; callers verify no phantom stream.
#
#   inject_pulse_restart
#     Restarts the pulse-realams-pulse-1 container (realams stack ONLY).
#     Polls /healthz with a bounded loop (up to 60 s, 2 s interval).
#     Prints observed recovery time.
#
#   inject_ams_restart
#     REFUSED unless FORCE_DISRUPT=1 is set in the environment.
#     Restarts the antmedia container and polls AMS REST until responsive.
#     OPERATOR-COORDINATED action — antmedia is the live server.
#
#   inject_ams_stop
#     REFUSED unless FORCE_DISRUPT=1 is set in the environment.
#     Stops the antmedia container. Caller must restart it via inject_ams_start.
#     OPERATOR-COORDINATED action — antmedia is the live server.
#
#   inject_ams_start
#     Starts the antmedia container (companion to inject_ams_stop).
#     REFUSED unless FORCE_DISRUPT=1 is set in the environment.
#
# Requires env.sh + publisher.sh to be sourced first (or auto-sourced here).
#
[[ -n "${_FAILURES_SH_LOADED:-}" ]] && return 0
_FAILURES_SH_LOADED=1

set -euo pipefail

# ── Bootstrap ─────────────────────────────────────────────────────────────────
_FAIL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./env.sh
[[ -n "${AMS_URL:-}" ]] || source "${_FAIL_DIR}/env.sh"
# shellcheck source=./publisher.sh
[[ -n "${_PUBLISHER_SH_LOADED:-}" ]] || source "${_FAIL_DIR}/publisher.sh"

# ── _refuse_without_force_disrupt FUNCNAME ─────────────────────────────────────
# Internal guard for operator-coordinated disruptions.
_refuse_without_force_disrupt() {
  local FUNCNAME_ARG="$1"
  if [[ "${FORCE_DISRUPT:-}" != "1" ]]; then
    echo "[failure] REFUSED: ${FUNCNAME_ARG} requires FORCE_DISRUPT=1." >&2
    echo "[failure] The antmedia container is the operator's live server." >&2
    echo "[failure] ${FUNCNAME_ARG} is an S18+ operator-coordinated action." >&2
    echo "[failure] Set FORCE_DISRUPT=1 only during an agreed maintenance window." >&2
    return 1
  fi
  return 0
}

# ── inject_publisher_kill ID ──────────────────────────────────────────────────
# Abruptly kills a running publisher container (SIGKILL).
# Logs the kill timestamp to stdout for timeline.txt.
# AMS result: stream transitions to 'terminated_unexpectedly'.
inject_publisher_kill() {
  local ID="$1"
  echo "[failure] inject_publisher_kill: killing publisher ${ID} at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >&2
  kill_publisher "$ID"
  echo "[failure] publisher ${ID} killed — poll for AMS terminated_unexpectedly + Pulse stream_publish_end" >&2
}

# ── inject_invalid_stream_key APP ─────────────────────────────────────────────
# Starts a short-lived, low-resolution ffmpeg publisher with a garbage stream
# key. Uses a dedicated container name (val-inv-key-<EPOCH>) so it never
# conflicts with real test publishers.
#
# Self-cleans via: --rm (container auto-removed on exit) + ffmpeg -t 20 limit.
# Prints the stream key used so callers can verify no phantom stream in Pulse.
#
# NOTE: Without AMS publishTokenControl=true, AMS will accept any stream key.
# Use this function on apps with token control enabled (not LiveApp).
inject_invalid_stream_key() {
  local APP="${1:-LiveApp}"
  local EPOCH
  EPOCH="$(date +%s)"
  local WRONG_KEY="val-inv-key-${EPOCH}"
  local CNAME="val-inv-key-${EPOCH}"
  local BUFSIZE=200   # 2 × 100k

  echo "[failure] inject_invalid_stream_key: stream key='${WRONG_KEY}' app=${APP}" >&2
  echo "[failure] container ${CNAME} will self-clean after ~20 s (-t 20 + --rm)" >&2

  # Run detached with --rm; ffmpeg exits after -t 20 seconds, container auto-removes.
  sg docker -c "docker run -d --rm \
    --name ${CNAME} \
    ${_FFMPEG_IMAGE} \
    -re \
    -f lavfi -i 'testsrc2=size=320x180:rate=5' \
    -f lavfi -i 'sine=frequency=440:sample_rate=22050' \
    -c:v libx264 -preset ultrafast \
    -b:v 100k -maxrate 100k -bufsize ${BUFSIZE}k \
    -pix_fmt yuv420p -g 10 \
    -c:a aac -b:a 32k -ar 22050 \
    -t 20 \
    -f flv rtmp://${_AMS_HOST}:${_RTMP_PORT}/${APP}/${WRONG_KEY}" > /dev/null 2>&1 || true

  # Print the key so the caller can assert no phantom Pulse stream for this ID
  echo "$WRONG_KEY"
}

# ── inject_pulse_restart ───────────────────────────────────────────────────────
# Restarts the pulse-realams Pulse container and waits for it to become healthy.
# Polls PULSE_HEALTH_URL (healthz) every 2 s for up to 60 s.
# Prints observed recovery time in seconds.
#
# pulse-realams ONLY — never call this against the prod Pulse deployment.
inject_pulse_restart() {
  if [[ "${PULSE_TARGET:-realams}" != "realams" ]]; then
    echo "[failure] inject_pulse_restart: REFUSED — only safe on PULSE_TARGET=realams." >&2
    echo "[failure] Never restart the prod Pulse container from a harness script." >&2
    return 1
  fi

  echo "[failure] inject_pulse_restart: restarting pulse-realams-pulse-1 at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >&2
  sg docker -c "docker restart pulse-realams-pulse-1" > /dev/null
  echo "[failure] container restart issued; polling ${PULSE_HEALTH_URL} for recovery" >&2

  local t=0
  local max_wait=60
  local interval=2
  while (( t < max_wait )); do
    sleep "$interval"
    (( t += interval ))
    local status
    status="$(curl -s -m 5 "${PULSE_HEALTH_URL}" 2>/dev/null \
      | grep -o '"status":"ok"' || true)"
    if [[ "$status" == '"status":"ok"' ]]; then
      echo "[failure] Pulse healthy after ${t} s" >&2
      return 0
    fi
    echo "[failure] ... waiting for Pulse healthz (${t}/${max_wait} s)" >&2
  done

  echo "[failure] ERROR: Pulse did not recover within ${max_wait} s" >&2
  return 1
}

# ── inject_ams_restart ────────────────────────────────────────────────────────
# OPERATOR-COORDINATED action — requires FORCE_DISRUPT=1.
# Restarts the antmedia container and polls AMS REST until responsive.
# Prints observed AMS recovery time.
inject_ams_restart() {
  _refuse_without_force_disrupt "inject_ams_restart" || return 1

  echo "[failure] inject_ams_restart: restarting antmedia at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >&2
  sg docker -c "docker restart antmedia" > /dev/null
  echo "[failure] antmedia restart issued; polling AMS REST for recovery" >&2

  local t=0
  local max_wait=120
  local interval=3
  while (( t < max_wait )); do
    sleep "$interval"
    (( t += interval ))
    if curl -s -m 5 "${AMS_URL}/rest/v2/version" > /dev/null 2>&1; then
      echo "[failure] AMS responsive after ${t} s" >&2
      return 0
    fi
    echo "[failure] ... waiting for AMS REST (${t}/${max_wait} s)" >&2
  done

  echo "[failure] ERROR: AMS did not recover within ${max_wait} s" >&2
  return 1
}

# ── inject_ams_stop ───────────────────────────────────────────────────────────
# OPERATOR-COORDINATED action — requires FORCE_DISRUPT=1.
# Stops the antmedia container. Use inject_ams_start to bring it back.
inject_ams_stop() {
  _refuse_without_force_disrupt "inject_ams_stop" || return 1

  echo "[failure] inject_ams_stop: stopping antmedia at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >&2
  sg docker -c "docker stop antmedia" > /dev/null
  echo "[failure] antmedia stopped — Pulse should surface degraded health within poll interval" >&2
}

# ── inject_ams_start ──────────────────────────────────────────────────────────
# OPERATOR-COORDINATED action — requires FORCE_DISRUPT=1.
# Starts the antmedia container after inject_ams_stop and waits for recovery.
inject_ams_start() {
  _refuse_without_force_disrupt "inject_ams_start" || return 1

  echo "[failure] inject_ams_start: starting antmedia at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >&2
  sg docker -c "docker start antmedia" > /dev/null
  echo "[failure] antmedia start issued; polling AMS REST for readiness" >&2

  local t=0
  local max_wait=120
  local interval=3
  while (( t < max_wait )); do
    sleep "$interval"
    (( t += interval ))
    if curl -s -m 5 "${AMS_URL}/rest/v2/version" > /dev/null 2>&1; then
      echo "[failure] AMS responsive after ${t} s — re-authenticate before polling" >&2
      return 0
    fi
    echo "[failure] ... waiting for AMS REST (${t}/${max_wait} s)" >&2
  done

  echo "[failure] ERROR: AMS did not become responsive within ${max_wait} s" >&2
  return 1
}
