#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-02-webrtc-viewer-count.sh
#
# TC-V-02: WebRTC viewer count
#
# Assertion matrix row:
#   AMS ground truth: GET /LiveApp/rest/v2/broadcasts/{id} → webRTCViewerCount == 1
#   Pulse assertion:  GET /api/v1/live/streams → protocol_mix.webrtc == 1 for stream
#   Tolerance:        WebRTC counts most accurate; assert exact match after 15 s.
#   Bounded poll:     AMS webRTCViewerCount, budget 90 s.
#   Skip condition:   If AMS-side webRTCViewerCount never materialises in 90 s,
#                     exit 77 SKIP — never a false FAIL against Pulse when the
#                     ground truth itself never appeared.
#   Exit:             0 PASS | 1 FAIL | 77 SKIP (precondition unmet)
#
set -euo pipefail

SCENARIO="TC-V-02"
echo "=== ${SCENARIO}: WebRTC viewer count ===" >&2

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
STREAM_ID="val-v02-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping WebRTC viewer and publisher ${STREAM_ID}"
  stop_webrtc_viewer "${STREAM_ID}" || true
  stop_publisher "${STREAM_ID}" || true
}
trap cleanup EXIT

# ── 1. Start publisher ──────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

# Wait for AMS to show broadcasting (up to 30 s)
log "Waiting for AMS broadcasting status (budget: 30 s)"
_i=0
while [ "${_i}" -lt 15 ]; do
  _st="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"')"
  if [ "${_st}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( _i * 2 )) s"
    break
  fi
  sleep 2
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-viewer"

# ── 2. Start WebRTC viewer (Playwright headless) ────────────────────────────────
log "Starting WebRTC viewer for ${STREAM_ID} via Playwright"
start_webrtc_viewer "${STREAM_ID}" "LiveApp"

# ── 3. Bounded poll: wait for AMS webRTCViewerCount == 1 (budget: 90 s) ────────
log "Polling AMS webRTCViewerCount (budget: 90 s, interval: 3 s)"
_ams_wrtc=0
_converge_s=-1
_i=0
while [ "${_i}" -lt 30 ]; do
  _ams_wrtc="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq '.webRTCViewerCount // 0')"
  if [ "${_ams_wrtc}" -ge 1 ] 2>/dev/null; then
    _converge_s=$(( _i * 3 ))
    log "AMS webRTCViewerCount=${_ams_wrtc} — converged after ${_converge_s} s"
    break
  fi
  log "AMS webRTCViewerCount=${_ams_wrtc} (sample ${_i}, elapsed $(( _i * 3 )) s)"
  sleep 3
  _i=$(( _i + 1 ))
done

# ── Skip guard: if AMS never showed a WebRTC viewer, exit 77 ───────────────────
if [ "${_converge_s}" -eq -1 ]; then
  log "AMS webRTCViewerCount never reached 1 in 90 s"
  log "Possible causes: Playwright container failed to load player page, WebRTC ICE negotiation timed out, AMS WebRTC endpoint not reachable"
  {
    echo "SKIP"
    echo "AMS webRTCViewerCount never materialised in 90 s — cannot assert Pulse without ground truth."
    echo "Diagnostics: AMS final webRTCViewerCount=${_ams_wrtc}"
    echo "Playwright viewer container: val-wv-${STREAM_ID} (CID file: /tmp/claude-1000/webrtc-viewer-${STREAM_ID}.cid)"
  } > "${EVIDENCE_DIR}/verdict.txt"
  cat "${EVIDENCE_DIR}/verdict.txt" >&2
  # Cleanup runs via trap
  exit 77
fi

# ── 4. Wait 15 s tolerance window for Pulse convergence ─────────────────────────
log "Waiting 15 s for Pulse poll-convergence window"
sleep 15

# ── 5. Capture final state ──────────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "with-viewer"
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "with-viewer"
log "Snapshots captured"

# ── 6. Read assertion values ────────────────────────────────────────────────────
_ams_broadcast="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}")"
_ams_wrtc_final="$(printf '%s' "${_ams_broadcast}" | jq '.webRTCViewerCount // 0')"

_pulse_resp="$(curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp")"

_pulse_wrtc="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    ( .items // .streams // [] )[]
    | select(.stream_id == $id)
    | ( .protocol_mix.webrtc // .vc_webrtc // 0 )
  ' | head -1)"
_pulse_wrtc="${_pulse_wrtc:-0}"

log "AMS webRTCViewerCount=${_ams_wrtc_final}  Pulse protocol_mix.webrtc=${_pulse_wrtc}"

# ── 7. Assertions ───────────────────────────────────────────────────────────────
# WebRTC counts are most accurate — assert exact match (tolerance +-0 after 15s)
# || true: prevent set -e from exiting before scenario_verdict aggregates all results
assert_eq "${_ams_wrtc_final}" "1" "AMS webRTCViewerCount == 1 for ${STREAM_ID}" || true
assert_eq "${_pulse_wrtc}" "1" "Pulse vc_webrtc (protocol_mix.webrtc) == 1 for ${STREAM_ID}" || true

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
