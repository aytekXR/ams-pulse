#!/usr/bin/env bash
# qa/realams/scenarios/TC-L-01-broadcast-lifecycle.sh
#
# TC-L-01: Normal broadcast lifecycle
#
# Assertion matrix row:
#   Steps:         1. Start ffmpeg RTMP publisher to LiveApp/val-stream
#                  2. Poll AMS for status=broadcasting (bounded ≤30 s, record convergence)
#                  3. Poll Pulse /live/streams for stream visible (bounded ≤30 s)
#                  4. Stop publisher (graceful)
#                  5. Poll AMS for status=finished (bounded ≤30 s)
#                  6. Poll Pulse for stream gone (bounded ≤30 s)
#   AMS truth:     status=broadcasting then status=finished
#   Pulse assert:  stream appears with publisher_state=publishing; disappears after stop
#   Tolerance:     15 s convergence window (scenario-matrix §Tolerance Windows)
#   Exit:          0 PASS | 1 FAIL | 77 SKIP (precondition: AMS never reaches broadcasting)
#
set -euo pipefail

SCENARIO="TC-L-01"
echo "=== ${SCENARIO}: Broadcast Lifecycle ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"
# shellcheck source=../harness/publisher.sh
source "${_DIR}/../harness/publisher.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-l01-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping publisher ${STREAM_ID} (if still running)"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Baseline capture ──────────────────────────────────────────────────────────────
capture_pulse "/live/overview" "pre-baseline"

# ── Phase 1: Publisher UP ─────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 1000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 1000

# ── Poll AMS for broadcasting (≤30 s, 3 s interval = 10 iterations) ──────────────
log "Polling AMS for status=broadcasting (budget: 30 s)"
_ams_status=""
_ams_conv_s=999   # sentinel: exceeded budget
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_status="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_status}" = "broadcasting" ]; then
    _ams_conv_s=$(( (_i + 1) * 3 ))
    log "AMS status=broadcasting after ${_ams_conv_s} s"
    break
  fi
  log "AMS status=${_ams_status} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "phase1-ams"

# ── Poll Pulse /live/streams for stream visibility (≤30 s) ───────────────────────
log "Polling Pulse /live/streams for ${STREAM_ID} (budget: 30 s)"
_pulse_state=""
_pulse_conv_s=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _pulse_state="$(curl -s -m 10 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/streams" \
    | jq -r --arg id "${STREAM_ID}" \
      '(.items // [])[] | select(.stream_id == $id) | .publisher_state' \
    2>/dev/null | head -1 || true)"
  if [ "${_pulse_state}" = "publishing" ]; then
    _pulse_conv_s=$(( (_i + 1) * 3 ))
    log "Pulse publisher_state=publishing after ${_pulse_conv_s} s"
    break
  fi
  log "Pulse publisher_state='${_pulse_state}' (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_pulse "/live/streams" "phase1-pulse"

# ── Phase 1 assertions ────────────────────────────────────────────────────────────
log "ASSERT: AMS status=broadcasting  Pulse publisher_state=publishing"
log "ASSERT: convergence AMS=${_ams_conv_s}s Pulse=${_pulse_conv_s}s (budget each: 30s)"
assert_eq "${_ams_status}" "broadcasting" "${SCENARIO} AMS status=broadcasting after publish" || true
assert_eq "${_pulse_state}" "publishing" "${SCENARIO} Pulse publisher_state=publishing" || true
assert_lte "${_ams_conv_s}" 30 "${SCENARIO} AMS broadcasting convergence ≤30 s" || true
assert_lte "${_pulse_conv_s}" 30 "${SCENARIO} Pulse visible convergence ≤30 s" || true

# ── Phase 2: Publisher STOP ───────────────────────────────────────────────────────
_stop_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
log "Stopping publisher ${STREAM_ID} gracefully at ${_stop_ts}"
stop_publisher "${STREAM_ID}"

# ── Poll AMS for finished (≤30 s) ────────────────────────────────────────────────
log "Polling AMS for status=finished (budget: 30 s)"
_ams_status_after=""
_ams_stop_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_status_after="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_status_after}" = "finished" ]; then
    _ams_stop_conv=$(( (_i + 1) * 3 ))
    log "AMS status=finished after ${_ams_stop_conv} s"
    break
  fi
  log "AMS status=${_ams_status_after} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "phase2-ams"

# ── Poll Pulse for stream gone (≤30 s) ───────────────────────────────────────────
log "Polling Pulse /live/streams for ${STREAM_ID} removal (budget: 30 s)"
_pulse_gone=99
_pulse_stop_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _pulse_gone="$(curl -s -m 10 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/streams" \
    | jq --arg id "${STREAM_ID}" \
      '[(.items // [])[] | select(.stream_id == $id)] | length' \
    2>/dev/null || echo 99)"
  if [ "${_pulse_gone}" = "0" ]; then
    _pulse_stop_conv=$(( (_i + 1) * 3 ))
    log "Pulse stream absent from /live/streams after ${_pulse_stop_conv} s"
    break
  fi
  log "Pulse stream still in list (count=${_pulse_gone}, attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_pulse "/live/streams" "phase2-pulse"

# ── Phase 2 assertions ────────────────────────────────────────────────────────────
log "ASSERT: AMS status=finished  Pulse stream gone  convergence ≤30 s each"
{
  printf 'Stop timestamp: %s\n' "${_stop_ts}"
  printf 'AMS finished convergence: %s s (budget: 30 s)\n' "${_ams_stop_conv}"
  printf 'Pulse removal convergence: %s s (budget: 30 s)\n' "${_pulse_stop_conv}"
} >> "${EVIDENCE_DIR}/timeline.txt"

assert_eq "${_ams_status_after}" "finished" "${SCENARIO} AMS status=finished after stop" || true
assert_eq "${_pulse_gone}" "0" "${SCENARIO} Pulse stream removed from /live/streams after stop" || true
assert_lte "${_ams_stop_conv}" 30 "${SCENARIO} AMS finished convergence ≤30 s" || true
assert_lte "${_pulse_stop_conv}" 30 "${SCENARIO} Pulse removal convergence ≤30 s" || true

# ── Verdict ───────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
