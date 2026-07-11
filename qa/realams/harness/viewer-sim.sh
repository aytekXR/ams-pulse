#!/usr/bin/env bash
# qa/realams/harness/viewer-sim.sh
#
# Viewer simulation — HLS curl-loop viewers and Playwright WebRTC viewers.
# SOURCE this file from scenario scripts; do not execute directly.
#
# Functions exported:
#   start_hls_viewer     ID APP VIEWER_ID   — curl-loop HLS viewer (background subshell)
#   stop_hls_viewer      VIEWER_ID
#   stop_all_hls_viewers                    — kill every tracked HLS viewer
#   start_webrtc_viewer  ID APP             — Playwright headless Chromium WebRTC session
#   stop_webrtc_viewer   ID
#   ramp_hls_viewers     ID TARGET STEP INTERVAL
#
# PID / CID files: /tmp/claude-1000/   (never written to repo)
# Playwright image: mcr.microsoft.com/playwright:v1.61.1-noble  (pre-pulled)
# WebRTC container name pattern: val-wv-<ID>
#
# Player URL discovery (start_webrtc_viewer):
#   1st try: http://<AMS_HOST>:5080/<APP>/play.html?name=<ID>&playOrder=webrtc
#   2nd try: http://<AMS_HOST>:5080/<APP>/play.html?id=<ID>
#   Whichever returns HTTP 200 + non-empty body is used; result is printed.
#
# Requires env.sh to be sourced first (for AMS_URL, REPO_ROOT).
#
[[ -n "${_VIEWER_SIM_SH_LOADED:-}" ]] && return 0
_VIEWER_SIM_SH_LOADED=1

set -euo pipefail

# ── Bootstrap ─────────────────────────────────────────────────────────────────
_VS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./env.sh
[[ -n "${AMS_URL:-}" ]] || source "${_VS_DIR}/env.sh"

# ── Constants ─────────────────────────────────────────────────────────────────
_VIEWER_PID_DIR="/tmp/claude-1000"
_PLAYWRIGHT_IMAGE="mcr.microsoft.com/playwright:v1.61.1-noble"
_WEB_DIR="${REPO_ROOT}/web"

# Derive AMS HTTP host:port from AMS_URL (http://HOST:PORT → HOST:PORT)
_vs_url_strip="${AMS_URL#*://}"
_AMS_HTTP_HOSTPORT="${_vs_url_strip%%/*}"
unset _vs_url_strip

# ── start_hls_viewer ID APP VIEWER_ID ─────────────────────────────────────────
# Simulates a real HLS player: polls the playlist every 2 s and downloads
# each segment EXACTLY ONCE (never re-fetches an already-seen segment).
# This matches real-player semantics so AMS counts one viewer window per
# sim-viewer instead of multiplying by segments-per-poll.
#
# PID written to:       /tmp/claude-1000/hls-viewer-<VIEWER_ID>.pid
# Seen-segment log to:  /tmp/claude-1000/hls-viewer-<VIEWER_ID>.seen
start_hls_viewer() {
  local ID="$1"
  local APP="${2:-LiveApp}"
  local VIEWER_ID="${3:-viewer-001}"
  local PLAYLIST="${AMS_URL}/${APP}/streams/${ID}.m3u8"
  local PID_FILE="${_VIEWER_PID_DIR}/hls-viewer-${VIEWER_ID}.pid"
  local SEEN_FILE="${_VIEWER_PID_DIR}/hls-viewer-${VIEWER_ID}.seen"

  mkdir -p "$_VIEWER_PID_DIR"
  # Initialise an empty seen-segments registry for this viewer
  : > "$SEEN_FILE"

  (
    while true; do
      # Fetch playlist; extract ALL .ts segment names
      # shellcheck disable=SC2310
      SEGMENTS="$(curl -s -m 10 "$PLAYLIST" 2>/dev/null | grep '\.ts$' || true)"
      if [[ -n "$SEGMENTS" ]]; then
        # Base URL = playlist URL minus the filename
        BASE="${PLAYLIST%/*}"
        while IFS= read -r SEG; do
          [[ -z "$SEG" ]] && continue
          # Real-player rule: fetch each segment ONCE only
          if ! grep -qxF "$SEG" "$SEEN_FILE" 2>/dev/null; then
            echo "$SEG" >> "$SEEN_FILE"
            curl -s -m 30 -o /dev/null "${BASE}/${SEG}" 2>/dev/null || true
          fi
        done <<< "$SEGMENTS"
      fi
      sleep 2
    done
  ) &
  local PID=$!
  echo "$PID" > "$PID_FILE"
  echo "[viewer-hls] ${VIEWER_ID} started (PID ${PID}) → ${PLAYLIST}" >&2
}

# ── stop_hls_viewer VIEWER_ID ─────────────────────────────────────────────────
stop_hls_viewer() {
  local VIEWER_ID="$1"
  local PID_FILE="${_VIEWER_PID_DIR}/hls-viewer-${VIEWER_ID}.pid"
  local SEEN_FILE="${_VIEWER_PID_DIR}/hls-viewer-${VIEWER_ID}.seen"
  if [[ -f "$PID_FILE" ]]; then
    local PID
    PID="$(cat "$PID_FILE")"
    kill "$PID" 2>/dev/null || true
    rm -f "$PID_FILE" "$SEEN_FILE"
    echo "[viewer-hls] ${VIEWER_ID} stopped (PID ${PID})" >&2
  else
    echo "[viewer-hls] ${VIEWER_ID}: PID file not found (already stopped?)" >&2
  fi
}

# ── stop_all_hls_viewers ──────────────────────────────────────────────────────
# Kills every HLS viewer tracked by a PID file in the viewer PID dir.
# Also removes the corresponding .seen state files.
stop_all_hls_viewers() {
  local count=0
  local pid_file
  for pid_file in "${_VIEWER_PID_DIR}"/hls-viewer-*.pid; do
    [[ -f "$pid_file" ]] || continue
    local PID VNAME
    PID="$(cat "$pid_file")"
    VNAME="$(basename "$pid_file" .pid | sed 's/^hls-viewer-//')"
    kill "$PID" 2>/dev/null || true
    rm -f "$pid_file" "${_VIEWER_PID_DIR}/hls-viewer-${VNAME}.seen"
    echo "[viewer-hls] ${VNAME} stopped (PID ${PID})" >&2
    (( count++ )) || true
  done
  echo "[viewer-hls] stopped ${count} HLS viewer(s)" >&2
}

# ── start_webrtc_viewer ID APP ────────────────────────────────────────────────
# Launches a Playwright headless Chromium session that plays a WebRTC stream.
# Container name: val-wv-<ID>
# CID file: /tmp/claude-1000/webrtc-viewer-<ID>.cid  (contains container name)
#
# URL discovery: tries play.html?name=<ID>&playOrder=webrtc first; falls back
# to play.html?id=<ID>. Prints which form succeeded.
#
# The session holds for ~90 seconds so AMS webRTCViewerCount registers.
# Chromium flags: --autoplay-policy=no-user-gesture-required
#                 --use-fake-ui-for-media-stream
#                 --use-fake-device-for-media-stream
start_webrtc_viewer() {
  local ID="$1"
  local APP="${2:-LiveApp}"
  local AMS_BASE_URL="http://${_AMS_HTTP_HOSTPORT}"
  local URL1="${AMS_BASE_URL}/${APP}/play.html?name=${ID}&playOrder=webrtc"
  local URL2="${AMS_BASE_URL}/${APP}/play.html?id=${ID}"
  local PLAYER_URL=""

  # ── URL discovery ──────────────────────────────────────────────────────────
  local body
  body="$(curl -s -m 10 -o /dev/null -w '%{http_code}' "$URL1" 2>/dev/null || true)"
  if [[ "$body" == "200" ]]; then
    # Verify body is non-empty
    local content
    content="$(curl -s -m 10 "$URL1" 2>/dev/null || true)"
    if [[ -n "$content" ]]; then
      PLAYER_URL="$URL1"
      echo "[viewer-webrtc] player URL (form 1 — ?name=&playOrder=webrtc): ${PLAYER_URL}" >&2
    fi
  fi

  if [[ -z "$PLAYER_URL" ]]; then
    body="$(curl -s -m 10 -o /dev/null -w '%{http_code}' "$URL2" 2>/dev/null || true)"
    if [[ "$body" == "200" ]]; then
      local content2
      content2="$(curl -s -m 10 "$URL2" 2>/dev/null || true)"
      if [[ -n "$content2" ]]; then
        PLAYER_URL="$URL2"
        echo "[viewer-webrtc] player URL (form 2 — ?id=): ${PLAYER_URL}" >&2
      fi
    fi
  fi

  if [[ -z "$PLAYER_URL" ]]; then
    echo "[viewer-webrtc] WARNING: neither player URL returned 200+non-empty body." >&2
    echo "[viewer-webrtc]   Falling back to form 1: ${URL1}" >&2
    PLAYER_URL="$URL1"
  fi

  mkdir -p "$_VIEWER_PID_DIR"

  # ── Write Playwright JS to temp file (mounted into the container) ──────────
  local JS_FILE="${_VIEWER_PID_DIR}/webrtc-viewer-${ID}.js"
  cat > "$JS_FILE" <<JSEOF
const { chromium } = require('playwright');
(async () => {
  const browser = await chromium.launch({
    headless: true,
    args: [
      '--autoplay-policy=no-user-gesture-required',
      '--use-fake-ui-for-media-stream',
      '--use-fake-device-for-media-stream'
    ]
  });
  const page = await browser.newPage();
  try {
    console.log('[webrtc-viewer] navigating to ${PLAYER_URL}');
    await page.goto('${PLAYER_URL}');
    console.log('[webrtc-viewer] page loaded — holding 90 s for WebRTC stats');
    await page.waitForTimeout(90000);
    console.log('[webrtc-viewer] done');
  } catch (err) {
    console.error('[webrtc-viewer] error:', err.message);
  } finally {
    await browser.close();
  }
})();
JSEOF

  # ── Launch Playwright container ────────────────────────────────────────────
  local CNAME="val-wv-${ID}"
  local CID_FILE="${_VIEWER_PID_DIR}/webrtc-viewer-${ID}.cid"

  local run_cmd
  run_cmd="docker run -d --rm"
  run_cmd="${run_cmd} --name ${CNAME}"
  run_cmd="${run_cmd} --ipc=host --network host"
  run_cmd="${run_cmd} -w /work"
  run_cmd="${run_cmd} -v ${_WEB_DIR}:/work"
  run_cmd="${run_cmd} -v ${_VIEWER_PID_DIR}:${_VIEWER_PID_DIR}"
  # NODE_PATH lets the script resolve `require('playwright')` against the
  # web/ node_modules even though the JS file lives outside /work.
  # Without this, Node walks up from /tmp/claude-1000/ and never finds
  # playwright — the container exits immediately and webRTCViewerCount
  # never rises (root cause of TC-V-02 SKIP in S17).
  run_cmd="${run_cmd} -e NODE_PATH=/work/node_modules"
  run_cmd="${run_cmd} ${_PLAYWRIGHT_IMAGE}"
  run_cmd="${run_cmd} node ${JS_FILE}"

  sg docker -c "$run_cmd" > /dev/null
  echo "$CNAME" > "$CID_FILE"
  echo "[viewer-webrtc] started ${CNAME} (CID file: ${CID_FILE})" >&2
}

# ── stop_webrtc_viewer ID ─────────────────────────────────────────────────────
stop_webrtc_viewer() {
  local ID="$1"
  local CID_FILE="${_VIEWER_PID_DIR}/webrtc-viewer-${ID}.cid"
  if [[ -f "$CID_FILE" ]]; then
    local CNAME
    CNAME="$(cat "$CID_FILE")"
    sg docker -c "docker stop ${CNAME}" > /dev/null 2>&1 || true
    rm -f "$CID_FILE"
    rm -f "${_VIEWER_PID_DIR}/webrtc-viewer-${ID}.js"
    echo "[viewer-webrtc] stopped ${CNAME}" >&2
  else
    echo "[viewer-webrtc] CID file not found for ${ID} (already stopped?)" >&2
  fi
}

# ── ramp_hls_viewers ID TARGET STEP INTERVAL ─────────────────────────────────
# Gradually ramps up HLS viewers in STEP increments, pausing INTERVAL seconds
# between each step.
# TARGET must be a multiple of STEP for an exact ramp.
# Viewer IDs: viewer-ramp-<STEP_COUNT>-<WITHIN_STEP>
ramp_hls_viewers() {
  local ID="$1"
  local TARGET="$2"
  local STEP="${3:-5}"
  local INTERVAL="${4:-10}"
  local i j

  echo "[viewer-hls] ramping to ${TARGET} viewers (step=${STEP}, interval=${INTERVAL}s)" >&2
  for i in $(seq "$STEP" "$STEP" "$TARGET"); do
    for j in $(seq 1 "$STEP"); do
      start_hls_viewer "$ID" "LiveApp" "viewer-ramp-${i}-${j}"
    done
    echo "[viewer-hls] ramp: ${i}/${TARGET} HLS viewers active" >&2
    sleep "$INTERVAL"
  done
  echo "[viewer-hls] ramp complete — ${TARGET} viewers started" >&2
}
