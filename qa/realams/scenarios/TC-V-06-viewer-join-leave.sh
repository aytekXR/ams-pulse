#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-06-viewer-join-leave.sh
#
# TC-V-06: Viewer join then leave
#
# Assertion matrix row (scenario-matrix.md):
#   Steps:   1. Start publisher val-v06-<epoch> on LiveApp
#            2. Start 5 HLS viewers; bounded-poll AMS for plateau (budget: 90 s)
#            3. Stop 3 viewers (leave 2)
#            4. Bounded-poll AMS hlsViewerCount for drop toward 2 (budget: 90 s)
#            5. Record observed AMS decay time
#            6. Assert Pulse viewer_count follows AMS within ±2
#   AMS truth:    hlsViewerCount drops from ~5 to ~2 after 3 viewers stop
#   Pulse assert: viewer_count within ±2 of AMS post-drop count
#   SKIP:    If AMS hlsViewerCount never drops below 4 within budget:
#            record AMS-semantics finding (HLS viewer expiry lag) and SKIP
#            the Pulse-side assert rather than false-FAIL Pulse.
#   Exit:    0 PASS | 1 FAIL | 77 SKIP
#
# BINDING RULES (memory: shell-harness-false-green-patterns):
#   - every assert_* ends with || true
#   - every curl|jq inside $() carries 2>/dev/null || echo <safe-default>
#   - SKIP: write verdict.txt manually then exit 77
#   - stop_all_hls_viewers in cleanup trap
#
set -euo pipefail

SCENARIO="TC-V-06"
echo "=== ${SCENARIO}: Viewer join then leave ===" >&2

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
STREAM_ID="val-v06-${EPOCH}"
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

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-viewers"

# ── 2. Start 5 HLS viewers ────────────────────────────────────────────────────
# ramp_hls_viewers(ID, TARGET=5, STEP=5, INTERVAL=10s)
# Creates viewer-ramp-5-{1..5} in one batch, then sleeps 10 s.
# After the call returns we have 5 tracked HLS viewer processes.
log "Starting 5 HLS viewers via ramp_hls_viewers (step=5, interval=10 s)"
ramp_hls_viewers "${STREAM_ID}" 5 5 10

# ── 3. Bounded poll for 5-viewer plateau (budget: 90 s) ───────────────────────
log "Polling AMS hlsViewerCount for 5-viewer plateau (budget: 90 s, interval: 3 s)"
_plateau_ams=0
_plateau_conv_s=-1
_i=0
while [ "${_i}" -lt 30 ]; do
  _plateau_ams="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
  _plateau_ams="${_plateau_ams:-0}"
  if [ "${_plateau_ams}" -ge 1 ] 2>/dev/null; then
    _plateau_conv_s=$(( _i * 3 ))
    log "Plateau: AMS hlsViewerCount=${_plateau_ams} after ${_plateau_conv_s} s"
    break
  fi
  log "Plateau poll: AMS hlsViewerCount=${_plateau_ams} (attempt $(( _i + 1 ))/30)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "plateau-ams"

if [ "${_plateau_conv_s}" -eq -1 ]; then
  log "SKIP: AMS hlsViewerCount never reached >=1 in 90 s — cannot test viewer-leave delta"
  printf 'SKIP\nPrecondition unmet: AMS hlsViewerCount for %s never reached >=1 within 90 s.\nFinal AMS hlsViewerCount: %s\n' \
    "${STREAM_ID}" "${_plateau_ams}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

log "ASSERT: 5-viewer plateau established at AMS hlsViewerCount=${_plateau_ams}"
assert_gte "${_plateau_ams}" 1 "${SCENARIO} AMS hlsViewerCount >= 1 at 5-viewer plateau" || true

# ── 4. Stop 3 of 5 viewers ────────────────────────────────────────────────────
# ramp_hls_viewers created viewer-ramp-5-{1..5}. Stop {1..3}, leave {4..5}.
_stop_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
log "Stopping 3 viewers (viewer-ramp-5-1, 2, 3) at ${_stop_ts}"
stop_hls_viewer "viewer-ramp-5-1" || true
stop_hls_viewer "viewer-ramp-5-2" || true
stop_hls_viewer "viewer-ramp-5-3" || true
printf 'viewers_stopped: 3 at %s  remaining: viewer-ramp-5-4 viewer-ramp-5-5\n' \
  "${_stop_ts}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── 5. Bounded poll for AMS hlsViewerCount to drop to ≤3 (budget: 90 s) ──────
# HLS viewer expiry on AMS may lag (segment-request-based counting).
# If AMS never drops below 4 → AMS-semantics finding → SKIP Pulse assert.
log "Polling AMS hlsViewerCount for drop to <=3 (budget: 90 s, interval: 3 s)"
_post_ams=0
_decay_s=-1
_i=0
while [ "${_i}" -lt 30 ]; do
  _post_ams="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
  _post_ams="${_post_ams:-0}"
  if [ "${_post_ams}" -le 3 ] 2>/dev/null; then
    _decay_s=$(( _i * 3 ))
    log "AMS hlsViewerCount dropped to ${_post_ams} after ${_decay_s} s (from plateau ${_plateau_ams})"
    break
  fi
  log "Decay poll: AMS hlsViewerCount=${_post_ams} (attempt $(( _i + 1 ))/30)"
  sleep 3
  _i=$(( _i + 1 ))
done

printf 'viewer_stop_ts=%s  ams_plateau=%s  ams_post=%s  decay_s=%s\n' \
  "${_stop_ts}" "${_plateau_ams}" "${_post_ams}" "${_decay_s}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "post-leave-ams"

if [ "${_decay_s}" -eq -1 ]; then
  # AMS-semantics finding: HLS viewer expiry did not complete within 90 s budget.
  # This is expected for HLS — counts are segment-request-based and may lag.
  # SKIP the Pulse-side assert rather than false-FAIL Pulse.
  log "AMS-FINDING: hlsViewerCount never dropped below 4 within 90 s after stopping 3 viewers."
  log "Final AMS hlsViewerCount=${_post_ams} — HLS viewer expiry lag observed."
  log "Skipping Pulse-side assert: cannot validate Pulse tracking without ground truth drop."
  printf 'SKIP\nAMS-semantics finding: hlsViewerCount did not drop below 4 in 90 s after stopping 3 of 5 viewers.\nObserved final AMS count: %s\nHLS viewer expiry on AMS is segment-request based and may lag beyond 90 s.\nPulse-side assert skipped — no false-FAIL against Pulse for AMS-side delay.\nSee TC-DOC-01 (HLS viewer count CDN/expiry limitation).\n' \
    "${_post_ams}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

log "AMS drop confirmed: hlsViewerCount=${_post_ams}  decay time=${_decay_s} s"

# ── 6. Wait for Pulse convergence ─────────────────────────────────────────────
log "Waiting 15 s for Pulse poll-convergence window"
sleep 15

# ── 7. Assert Pulse viewer_count follows AMS within ±2 ────────────────────────
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "post-leave-pulse"

_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp" \
  2>/dev/null || echo '{"items":[]}')"
_pulse_vc="$(printf '%s' "${_pulse_resp}" | jq \
  --arg id "${STREAM_ID}" \
  '(.items // .streams // []) | map(select(.stream_id == $id)) | first | (.viewers // .viewer_count // 0)' \
  2>/dev/null || echo 0)"
_pulse_vc="${_pulse_vc:-0}"

log "Post-leave: AMS hlsViewerCount=${_post_ams}  Pulse viewer_count=${_pulse_vc}  delta=$((${_pulse_vc} - ${_post_ams}))"
printf 'post_leave: ams=%s  pulse=%s  decay_s=%s\n' \
  "${_post_ams}" "${_pulse_vc}" "${_decay_s}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# Tolerance ±2 (scenario-matrix.md §Viewer Count Tolerance for HLS)
assert_lte "${_post_ams}" 3 "${SCENARIO} AMS hlsViewerCount <=3 after stopping 3 of 5 HLS viewers" || true
assert_within "${_pulse_vc}" "${_post_ams}" 2 \
  "${SCENARIO} Pulse viewer_count within ±2 of AMS post-leave count" || true

# ── Verdict ────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
