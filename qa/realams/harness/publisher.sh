#!/usr/bin/env bash
# qa/realams/harness/publisher.sh
#
# Publisher control — start/stop/kill ffmpeg RTMP publishers.
# SOURCE this file from scenario scripts; do not execute directly.
#
# Functions exported:
#   start_publisher        ID APP KBPS
#   stop_publisher         ID            (graceful docker stop → AMS: finished)
#   kill_publisher         ID            (abrupt SIGKILL → AMS: terminated_unexpectedly)
#   start_bulk_publishers  COUNT APP PREFIX KBPS
#
# Container naming:    pulse-pub-val-<ID>
# FFmpeg image:        jrottenberg/ffmpeg:4.1-alpine  (ENTRYPOINT = ffmpeg)
# Arg model (from ams-teststream):
#   -re -f lavfi -i testsrc2=size=1280x720:rate=30
#   -f lavfi -i sine=frequency=1000:sample_rate=44100
#   -c:v libx264 -preset veryfast -tune zerolatency
#   -b:v <N>k -maxrate <N>k -bufsize <2N>k -pix_fmt yuv420p -g 60
#   -c:a aac -b:a 128k -ar 44100
#   -f flv rtmp://<HOST>:1935/<APP>/<ID>
#
# Requires env.sh to be sourced first (for AMS_URL).
#
[[ -n "${_PUBLISHER_SH_LOADED:-}" ]] && return 0
_PUBLISHER_SH_LOADED=1

set -euo pipefail

# ── Bootstrap ─────────────────────────────────────────────────────────────────
_PUB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./env.sh
[[ -n "${AMS_URL:-}" ]] || source "${_PUB_DIR}/env.sh"

# ── Constants ─────────────────────────────────────────────────────────────────
_FFMPEG_IMAGE="jrottenberg/ffmpeg:4.1-alpine"

# Derive RTMP hostname from AMS_URL (http://HOST:PORT → HOST)
_ams_url_strip="${AMS_URL#*://}"
_AMS_HOST="${_ams_url_strip%%:*}"
unset _ams_url_strip

_RTMP_PORT="1935"
_PUB_PREFIX="pulse-pub-val"

# ── start_publisher ID APP KBPS ────────────────────────────────────────────────
# Starts a detached ffmpeg RTMP publisher container.
# Container name: pulse-pub-val-<ID>
start_publisher() {
  local ID="$1"
  local APP="${2:-LiveApp}"
  local KBPS="${3:-2000}"
  local BUFSIZE
  BUFSIZE=$(( KBPS * 2 ))
  local CNAME="${_PUB_PREFIX}-${ID}"
  local RTMP_URL="rtmp://${_AMS_HOST}:${_RTMP_PORT}/${APP}/${ID}"

  echo "[publisher] starting ${CNAME} → ${RTMP_URL} @ ${KBPS}k" >&2
  sg docker -c "docker run -d --rm \
    --name ${CNAME} \
    ${_FFMPEG_IMAGE} \
    -re \
    -f lavfi -i 'testsrc2=size=1280x720:rate=30' \
    -f lavfi -i 'sine=frequency=1000:sample_rate=44100' \
    -c:v libx264 -preset veryfast -tune zerolatency \
    -b:v ${KBPS}k -maxrate ${KBPS}k -bufsize ${BUFSIZE}k \
    -pix_fmt yuv420p -g 60 \
    -c:a aac -b:a 128k -ar 44100 \
    -f flv ${RTMP_URL}" > /dev/null
  echo "[publisher] started ${CNAME}" >&2
}

# ── stop_publisher ID ──────────────────────────────────────────────────────────
# Sends SIGTERM then waits (docker stop default 10 s), giving ffmpeg a chance to
# write the FLV trailer and close the RTMP connection cleanly.
# AMS result: stream transitions to 'finished'.
stop_publisher() {
  local ID="$1"
  local CNAME="${_PUB_PREFIX}-${ID}"
  echo "[publisher] stopping ${CNAME} gracefully (SIGTERM)" >&2
  sg docker -c "docker stop ${CNAME}" > /dev/null 2>&1 || true
  echo "[publisher] stopped ${CNAME}" >&2
}

# ── kill_publisher ID ──────────────────────────────────────────────────────────
# Sends SIGKILL immediately, simulating an encoder crash.
# AMS result: stream transitions to 'terminated_unexpectedly'.
kill_publisher() {
  local ID="$1"
  local CNAME="${_PUB_PREFIX}-${ID}"
  echo "[publisher] killing ${CNAME} (SIGKILL — encoder-crash simulation)" >&2
  sg docker -c "docker kill ${CNAME}" > /dev/null 2>&1 || true
  echo "[publisher] killed ${CNAME}" >&2
}

# ── start_bulk_publishers COUNT APP PREFIX KBPS ────────────────────────────────
# Starts COUNT parallel publishers with zero-padded 4-digit sequence IDs.
# Container names: pulse-pub-val-<PREFIX><NNNN>
# Waits for all docker run -d commands before returning.
#
# NOTE: "dispatched" means docker run -d returned, NOT that ffmpeg connected to
# AMS. AMS may reject RTMP connections with "current system resources not enough"
# when concurrent stream capacity is exceeded — the container will exit with
# code 1. Callers MUST verify against AMS /broadcasts/list after a short wait.
start_bulk_publishers() {
  local COUNT="${1:-5}"
  local APP="${2:-LiveApp}"
  local PREFIX="${3:-valtest}"
  local KBPS="${4:-500}"
  local i ID

  echo "[publisher] launching ${COUNT} publishers (prefix=${PREFIX}, app=${APP}, ${KBPS}k)" >&2
  for i in $(seq 1 "$COUNT"); do
    ID=$(printf "%s%04d" "$PREFIX" "$i")
    start_publisher "$ID" "$APP" "$KBPS" &
  done
  wait
  echo "[publisher] all ${COUNT} publisher containers dispatched (docker run -d; verify RTMP connections via AMS REST)" >&2
}
