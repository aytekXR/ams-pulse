#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-03-viewer-crosscheck.sh
#
# TC-V-03: Viewer count cross-check (AMS inline vs. Pulse)
#
# Assertion matrix row:
#   Setup:            1 publisher + 2 HLS viewers + 1 WebRTC viewer
#   AMS ground truth: hlsViewerCount + webRTCViewerCount (inline BroadcastDTO)
#   Pulse assertion:  GET /api/v1/live/streams → viewers within +-2 of AMS sum
#   Tolerance:        +-2 after 15 s convergence window (race between AMS poll and Pulse)
#                     Negative rtmpViewerCount clamped to 0 per scenario-matrix.
#   Exit:             0 PASS | 1 FAIL | 77 SKIP (publisher never broadcasting)
#
set -euo pipefail

SCENARIO="TC-V-03"
echo "=== ${SCENARIO}: Viewer count cross-check (AMS inline vs Pulse) ===" >&2

# ── Harness bootstrap ───────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"
# shellcheck source=../harness/publisher.sh
source "${_DIR}/../harness/publisher.sh"
# shellcheck source=../harness/viewer-sim.sh
source "${_DIR}/../harness/viewer-sim.sh"

# ── Per-run identifiers ─────────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-v03-${EPOCH}"
VIEWER_HLS_A="hls-v03a-${EPOCH}"
VIEWER_HLS_B="hls-v03b-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping all viewers and publisher ${STREAM_ID}"
  stop_hls_viewer "${VIEWER_HLS_A}" || true
  stop_hls_viewer "${VIEWER_HLS_B}" || true
  stop_webrtc_viewer "${STREAM_ID}" || true
  stop_publisher "${STREAM_ID}" || true
}
trap cleanup EXIT

# ── 1. Start publisher ──────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

# Wait for AMS to show broadcasting (up to 30 s)
log "Waiting for AMS broadcasting status (budget: 30 s)"
_broadcasting=0
_i=0
while [ "${_i}" -lt 15 ]; do
  _st="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"')"
  if [ "${_st}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( _i * 2 )) s"
    _broadcasting=1
    break
  fi
  sleep 2
  _i=$(( _i + 1 ))
done

if [ "${_broadcasting}" -eq 0 ]; then
  log "Publisher never reached broadcasting status in 30 s — SKIP"
  {
    echo "SKIP"
    echo "Publisher ${STREAM_ID} never reached AMS status=broadcasting in 30 s."
    echo "Cannot run viewer cross-check without a live stream."
  } > "${EVIDENCE_DIR}/verdict.txt"
  cat "${EVIDENCE_DIR}/verdict.txt" >&2
  exit 77
fi

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-viewers"

# ── 2. Start 2 HLS viewers + 1 WebRTC viewer ───────────────────────────────────
log "Starting HLS viewer ${VIEWER_HLS_A}"
start_hls_viewer "${STREAM_ID}" "LiveApp" "${VIEWER_HLS_A}"

log "Starting HLS viewer ${VIEWER_HLS_B}"
start_hls_viewer "${STREAM_ID}" "LiveApp" "${VIEWER_HLS_B}"

log "Starting WebRTC viewer for ${STREAM_ID}"
start_webrtc_viewer "${STREAM_ID}" "LiveApp"

# ── 3. Bounded poll: wait for AMS to see at least some viewers (budget: 90 s) ──
# HLS is slow to register; WebRTC is faster. We expect hlsViewerCount >= 1
# eventually, but we don't block on the exact count — we assert after 15 s window.
log "Polling AMS for viewers > 0 (budget: 90 s, interval: 3 s)"
_i=0
while [ "${_i}" -lt 30 ]; do
  _ams_total="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" | \
    jq '( (.hlsViewerCount // 0) + (.webRTCViewerCount // 0) + (.dashViewerCount // 0) )')"
  if [ "${_ams_total:-0}" -ge 1 ] 2>/dev/null; then
    log "AMS total viewers=${_ams_total} after $(( _i * 3 )) s — viewers registering"
    break
  fi
  log "AMS total viewers=${_ams_total:-0} (sample ${_i})"
  sleep 3
  _i=$(( _i + 1 ))
done

# ── 4. Wait 15 s tolerance window ───────────────────────────────────────────────
log "Waiting 15 s for Pulse poll-convergence window"
sleep 15

# ── 5. Capture final state ──────────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "with-viewers"
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "with-viewers"
log "Snapshots captured"

# ── 6. Read assertion values ────────────────────────────────────────────────────
_ams_broadcast="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}")"

# AMS sum: hls + webrtc + dash; clamp negative rtmpViewerCount to 0
_ams_hls="$(printf '%s' "${_ams_broadcast}" | jq '.hlsViewerCount // 0')"
_ams_wrtc="$(printf '%s' "${_ams_broadcast}" | jq '.webRTCViewerCount // 0')"
_ams_rtmp_raw="$(printf '%s' "${_ams_broadcast}" | jq '.rtmpViewerCount // 0')"
_ams_dash="$(printf '%s' "${_ams_broadcast}" | jq '.dashViewerCount // 0')"

# Clamp negative rtmp to 0 per scenario-matrix / AV-16
_ams_rtmp="$(awk -v r="${_ams_rtmp_raw}" 'BEGIN { print (r < 0) ? 0 : r }')"

_ams_sum="$(awk \
  -v h="${_ams_hls}" -v w="${_ams_wrtc}" -v r="${_ams_rtmp}" -v d="${_ams_dash}" \
  'BEGIN { print h + w + r + d }')"

log "AMS breakdown: hls=${_ams_hls} wrtc=${_ams_wrtc} rtmp_raw=${_ams_rtmp_raw} rtmp_clamped=${_ams_rtmp} dash=${_ams_dash} sum=${_ams_sum}"

_pulse_resp="$(curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp")"

_pulse_viewers="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    ( .items // .streams // [] )[]
    | select(.stream_id == $id)
    | ( .viewers // .viewer_count // 0 )
  ' | head -1)"
_pulse_viewers="${_pulse_viewers:-0}"

log "Pulse viewers=${_pulse_viewers}  AMS sum=${_ams_sum}  delta=$(( _pulse_viewers - _ams_sum ))"

# ── 7. Assertions ───────────────────────────────────────────────────────────────
# Viewer count tolerance: +-2 after 15 s (scenario-matrix Tolerance Windows)
# || true: prevent set -e from exiting before scenario_verdict aggregates all results
assert_within "${_pulse_viewers}" "${_ams_sum}" 2 \
  "Pulse viewers within +-2 of AMS inline sum for ${STREAM_ID}" || true

# Evidence: also call compare_viewer_count for parity log (already has || true guard)
compare_viewer_count "${STREAM_ID}" "LiveApp" || true

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
