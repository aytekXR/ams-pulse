#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-01-hls-viewer-count.sh
#
# TC-V-01: HLS viewer count single viewer
#
# Assertion matrix row:
#   AMS ground truth: GET /LiveApp/rest/v2/broadcasts/{id} → hlsViewerCount >= 1
#   Pulse assertion:  GET /api/v1/live/streams → protocol_mix.hls >= 1 for stream;
#                     viewers >= 1
#   Tolerance:        HLS segment-based counting lags up to 60 s; assert after convergence.
#   Exit:             0 PASS | 1 FAIL | 77 SKIP (precondition unmet)
#
set -euo pipefail

SCENARIO="TC-V-01"
echo "=== ${SCENARIO}: HLS viewer count single viewer ===" >&2

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
STREAM_ID="val-v01-${EPOCH}"
VIEWER_ID="hls-v01-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping HLS viewer ${VIEWER_ID} and publisher ${STREAM_ID}"
  stop_hls_viewer "${VIEWER_ID}" || true
  stop_publisher "${STREAM_ID}" || true
}
trap cleanup EXIT

# ── 1. Start publisher ──────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

# Wait for AMS to show broadcasting (up to 30 s)
log "Waiting for AMS to show broadcasting status (budget: 30 s)"
_i=0
while [ "${_i}" -lt 15 ]; do
  _st="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")"
  if [ "${_st}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( _i * 2 )) s"
    break
  fi
  sleep 2
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-viewer"

# ── 2. Start 1 HLS viewer ───────────────────────────────────────────────────────
log "Starting HLS viewer ${VIEWER_ID} for ${STREAM_ID}"
start_hls_viewer "${STREAM_ID}" "LiveApp" "${VIEWER_ID}"

# ── 3. Bounded poll: wait for AMS hlsViewerCount >= 1 (budget: 60 s) ──────────
log "Polling AMS hlsViewerCount (budget: 60 s, interval: 2 s)"
_ams_hls=0
_converge_s=-1
_i=0
while [ "${_i}" -lt 30 ]; do
  _ams_hls="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
  if [ "${_ams_hls}" -ge 1 ] 2>/dev/null; then
    _converge_s=$(( _i * 2 ))
    log "AMS hlsViewerCount=${_ams_hls} — converged after ${_converge_s} s"
    break
  fi
  log "AMS hlsViewerCount=${_ams_hls} (sample ${_i})"
  sleep 2
  _i=$(( _i + 1 ))
done

if [ "${_converge_s}" -eq -1 ]; then
  log "AMS hlsViewerCount never reached 1 in 60 s — stream may not be live or HLS not ready"
  # Not a SKIP: the publisher is confirmed broadcasting; HLS count lag is real.
  # We still run assertions (they will record FAIL if AMS count is 0).
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
_ams_hls_final="$(printf '%s' "${_ams_broadcast}" | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"

_pulse_resp="$(curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp")"

# Defensive: accept either 'items' (OpenAPI) or 'streams' (alternate impl key)
_pulse_hls="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    ( .items // .streams // [] )[]
    | select(.stream_id == $id)
    | ( .protocol_mix.hls // .vc_hls // 0 )
  ' | head -1)"
_pulse_hls="${_pulse_hls:-0}"

_pulse_viewers="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    ( .items // .streams // [] )[]
    | select(.stream_id == $id)
    | ( .viewers // .viewer_count // 0 )
  ' | head -1)"
_pulse_viewers="${_pulse_viewers:-0}"

log "AMS hlsViewerCount=${_ams_hls_final}  Pulse protocol_mix.hls=${_pulse_hls}  Pulse viewers=${_pulse_viewers}"

# ── 7. Assertions ───────────────────────────────────────────────────────────────
# || true: assert failures return 1; prevent set -e from killing the script
# before scenario_verdict can aggregate all check results into verdict.txt.
assert_gte "${_ams_hls_final}" 1 "AMS hlsViewerCount >= 1 for ${STREAM_ID}" || true
assert_gte "${_pulse_hls}" 1 "Pulse vc_hls (protocol_mix.hls) >= 1 for ${STREAM_ID}" || true
assert_gte "${_pulse_viewers}" 1 "Pulse viewers >= 1 for ${STREAM_ID}" || true

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
