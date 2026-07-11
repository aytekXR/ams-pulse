#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-05-viewer-ramp.sh
#
# TC-V-05: Viewer count ramp — 10 → 30 HLS viewers
#
# Assertion matrix row (scenario-matrix.md):
#   Steps:   1. Start publisher val-v05-<epoch> on LiveApp
#            2. Ramp 10 HLS viewers via ramp_hls_viewers (step=5, interval=10s)
#            3. Bounded-poll AMS hlsViewerCount — plateau 1 (budget: 90 s)
#            4. Assert Pulse viewer_count ≈ AMS hlsViewerCount ±5% (HLS tolerance)
#            5. Add 20 more HLS viewers (total 30), ramp in two batches of 10
#            6. Bounded-poll AMS hlsViewerCount — plateau 2 (budget: 90 s)
#            7. Assert Pulse viewer_count ≈ AMS hlsViewerCount ±5% (plateau 2)
#   AMS truth:    hlsViewerCount approaches 10 then 30 (HLS counts are approximate)
#   Pulse assert: viewer_count tracks AMS within ±5% at each plateau
#   SKIP:    If AMS hlsViewerCount never reaches >=1 at plateau 1 (premise unmet)
#   Exit:    0 PASS | 1 FAIL | 77 SKIP
#
# BINDING RULES (memory: shell-harness-false-green-patterns):
#   - every assert_* ends with || true
#   - every curl|jq inside $() carries 2>/dev/null || echo <safe-default>
#   - SKIP: write verdict.txt manually then exit 77
#   - stop_all_hls_viewers called in cleanup trap
#
set -euo pipefail

SCENARIO="TC-V-05"
echo "=== ${SCENARIO}: Viewer count ramp (10 → 30 HLS viewers) ===" >&2

# ── Harness bootstrap ──────────────────────────────────────────────────────────
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

# ── Per-run identifiers ────────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-v05-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ───────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ───────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping all HLS viewers and publisher ${STREAM_ID}"
  stop_all_hls_viewers || true
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── 1. Start publisher ─────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

log "Waiting for AMS broadcasting status (budget: 30 s, interval: 2 s)"
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

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-ramp"

# ── 2. Ramp to 10 HLS viewers ─────────────────────────────────────────────────
# ramp_hls_viewers(ID, TARGET=10, STEP=5, INTERVAL=10s)
# Creates: viewer-ramp-5-{1..5}, viewer-ramp-10-{1..5}  (~20 s elapsed)
log "Ramp phase 1: starting 10 HLS viewers (step=5, interval=10 s)"
ramp_hls_viewers "${STREAM_ID}" 10 5 10

# ── 3. Bounded poll AMS hlsViewerCount — plateau 1 (budget: 90 s) ─────────────
log "Polling AMS hlsViewerCount for plateau 1 (budget: 90 s, interval: 3 s)"
_p1_ams_hls=0
_p1_converge_s=-1
_i=0
while [ "${_i}" -lt 30 ]; do
  _p1_ams_hls="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
  _p1_ams_hls="${_p1_ams_hls:-0}"
  if [ "${_p1_ams_hls}" -ge 1 ] 2>/dev/null; then
    _p1_converge_s=$(( _i * 3 ))
    log "Plateau 1: AMS hlsViewerCount=${_p1_ams_hls} after ${_p1_converge_s} s"
    break
  fi
  log "Plateau 1 poll: AMS hlsViewerCount=${_p1_ams_hls} (attempt $(( _i + 1 ))/30)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "plateau1-ams"
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "plateau1-pulse"

if [ "${_p1_converge_s}" -eq -1 ]; then
  log "SKIP: AMS hlsViewerCount never reached >=1 within 90 s — HLS may not be serving segments"
  printf 'SKIP\nPrecondition unmet: AMS hlsViewerCount for %s never reached >=1 within 90 s.\nFinal AMS hlsViewerCount: %s\nHLS is segment-request based; stream may not have produced segments yet.\n' \
    "${STREAM_ID}" "${_p1_ams_hls}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── 4. Assert Pulse viewer_count ≈ AMS at plateau 1 ──────────────────────────
log "Waiting 15 s for Pulse poll-convergence window (plateau 1)"
sleep 15

_p1_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp" \
  2>/dev/null || echo '{"items":[]}')"
_p1_pulse_vc="$(printf '%s' "${_p1_pulse_resp}" | jq \
  --arg id "${STREAM_ID}" \
  '(.items // .streams // []) | map(select(.stream_id == $id)) | first | (.viewers // .viewer_count // 0)' \
  2>/dev/null || echo 0)"
_p1_pulse_vc="${_p1_pulse_vc:-0}"

log "Plateau 1: AMS hlsViewerCount=${_p1_ams_hls}  Pulse viewer_count=${_p1_pulse_vc}"
printf 'plateau1: ams_hls=%s  pulse_vc=%s  converge_s=%s\n' \
  "${_p1_ams_hls}" "${_p1_pulse_vc}" "${_p1_converge_s}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# HLS tolerance: ±5% (scenario-matrix.md §Viewer Count Tolerance)
assert_approx "${_p1_pulse_vc}" "${_p1_ams_hls}" 5 \
  "${SCENARIO} plateau1 Pulse viewer_count ≈ AMS hlsViewerCount (±5% HLS tolerance)" || true

# ── 5. Ramp from 10 → 30 (add 20 more viewers with distinct IDs) ──────────────
# Using unique IDs v05-ext-{11..30} so they don't collide with ramp_hls_viewers naming
log "Ramp phase 2: adding 20 more HLS viewers to reach 30 total"
_vn=11
while [ "${_vn}" -le 20 ]; do
  start_hls_viewer "${STREAM_ID}" "LiveApp" "v05-ext-${_vn}"
  _vn=$(( _vn + 1 ))
done
log "First extra batch (11-20) started; sleeping 10 s"
sleep 10
while [ "${_vn}" -le 30 ]; do
  start_hls_viewer "${STREAM_ID}" "LiveApp" "v05-ext-${_vn}"
  _vn=$(( _vn + 1 ))
done
log "Second extra batch (21-30) started (30 total HLS viewers active)"

# ── 6. Bounded poll AMS hlsViewerCount — plateau 2 (budget: 90 s) ─────────────
# HLS counting is approximate; poll for growth above plateau 1.
# If AMS count does not grow, record the finding but still compare Pulse to AMS.
log "Polling AMS hlsViewerCount for plateau 2 (budget: 90 s, interval: 3 s)"
_p2_ams_hls="${_p1_ams_hls}"
_p2_converge_s=-1
_i=0
while [ "${_i}" -lt 30 ]; do
  _curr="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
  _curr="${_curr:-0}"
  _p2_ams_hls="${_curr}"
  if [ "${_curr}" -gt "${_p1_ams_hls}" ] 2>/dev/null; then
    _p2_converge_s=$(( _i * 3 ))
    log "Plateau 2: AMS hlsViewerCount=${_curr} (grew past plateau1=${_p1_ams_hls}) at ${_p2_converge_s} s"
    break
  fi
  log "Plateau 2 poll: AMS hlsViewerCount=${_curr} (attempt $(( _i + 1 ))/30)"
  sleep 3
  _i=$(( _i + 1 ))
done

if [ "${_p2_converge_s}" -eq -1 ]; then
  log "AMS-FINDING: hlsViewerCount did not grow above plateau1=${_p1_ams_hls} within 90 s after ramp to 30 viewers."
  log "This may reflect HLS CDN caching or slow segment-expiry semantics (doc gap TC-DOC-01)."
  log "Using final AMS count=${_p2_ams_hls} for plateau 2 Pulse comparison."
fi

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "plateau2-ams"
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "plateau2-pulse"

# ── 7. Assert Pulse viewer_count ≈ AMS at plateau 2 ──────────────────────────
log "Waiting 15 s for Pulse poll-convergence window (plateau 2)"
sleep 15

_p2_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp" \
  2>/dev/null || echo '{"items":[]}')"
_p2_pulse_vc="$(printf '%s' "${_p2_pulse_resp}" | jq \
  --arg id "${STREAM_ID}" \
  '(.items // .streams // []) | map(select(.stream_id == $id)) | first | (.viewers // .viewer_count // 0)' \
  2>/dev/null || echo 0)"
_p2_pulse_vc="${_p2_pulse_vc:-0}"

log "Plateau 2: AMS hlsViewerCount=${_p2_ams_hls}  Pulse viewer_count=${_p2_pulse_vc}"
printf 'plateau2: ams_hls=%s  pulse_vc=%s  converge_s=%s\n' \
  "${_p2_ams_hls}" "${_p2_pulse_vc}" "${_p2_converge_s}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# HLS tolerance: ±5% (scenario-matrix.md §Viewer Count Tolerance)
assert_approx "${_p2_pulse_vc}" "${_p2_ams_hls}" 5 \
  "${SCENARIO} plateau2 Pulse viewer_count ≈ AMS hlsViewerCount (±5% HLS tolerance)" || true

# ── Verdict ────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
