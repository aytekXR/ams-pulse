#!/usr/bin/env bash
# qa/realams/scenarios/TC-WH-02-poll-path-latency.sh
#
# TC-WH-02: Poll path covers webhook gap — latency measurement
#
# Assertion matrix row:
#   Steps:        1. Start publisher val-wh02-<epoch>
#                 2. Poll Pulse /live/streams until stream appears; record latency
#                 3. Stop publisher; poll until stream disappears; record latency
#   AMS truth:    AMS stream status transitions in REST poll
#   Pulse assert: stream appears within 10 s (PRD budget) of start;
#                 stream disappears within 10 s of stop
#   Exit:         0 PASS | 1 FAIL
#
set -euo pipefail

SCENARIO="TC-WH-02"
echo "=== ${SCENARIO}: Poll path latency — start/end detection ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"
# shellcheck source=../harness/publisher.sh
source "${_DIR}/../harness/publisher.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-wh02-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping publisher ${STREAM_ID}"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PULSE_URL=${PULSE_URL}"

# ── Phase 1: Start publisher — measure Pulse detection latency ────────────────
log "Starting publisher ${STREAM_ID} at 1000 kbps on LiveApp"
_pub_start_ts="$(date +%s)"
start_publisher "${STREAM_ID}" "LiveApp" 1000
log "Publisher started at epoch ${_pub_start_ts}"

# Poll Pulse /live/streams for up to 30 s (PRD budget: 10 s)
log "Polling Pulse /live/streams for stream appearance (budget: 30 s)"
_start_latency_s=999
_i=0
while [ "${_i}" -lt 10 ]; do
  sleep 3
  _in_pulse="$(curl -s -m 10 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/streams" \
    | jq --arg id "${STREAM_ID}" \
      '[(.items // [])[] | select(.stream_id == $id)] | length' \
    2>/dev/null || echo 0)"
  if [ "${_in_pulse}" -gt 0 ]; then
    _now="$(date +%s)"
    _start_latency_s=$(( _now - _pub_start_ts ))
    log "Pulse detected stream start after ${_start_latency_s} s (poll attempt $(( _i + 1 )))"
    break
  fi
  log "Stream not yet in Pulse (attempt $(( _i + 1 ))/10)"
  _i=$(( _i + 1 ))
done

capture_pulse "/live/streams" "phase1-stream-visible"

if [ "${_start_latency_s}" -eq 999 ]; then
  log "FAIL: Pulse did not detect stream start within 30 s"
  _start_latency_s=30
fi

assert_lte "${_start_latency_s}" 10 "${SCENARIO} stream start detected by Pulse within 10 s (PRD budget)" || true

log "start_detection_latency=${_start_latency_s}s"
printf 'stream_start_latency_s=%s\n' "${_start_latency_s}" >> "${EVIDENCE_DIR}/timeline.txt"

# Hold the stream for a moment so AMS fully registers it
sleep 3

# ── Phase 2: Stop publisher — measure Pulse end detection latency ─────────────
_stop_ts="$(date +%s)"
log "Stopping publisher ${STREAM_ID} gracefully at epoch ${_stop_ts}"
stop_publisher "${STREAM_ID}"
log "Publisher stopped"

# Poll Pulse /live/streams for up to 30 s for stream disappearance
log "Polling Pulse /live/streams for stream disappearance (budget: 30 s)"
_end_latency_s=999
_i=0
while [ "${_i}" -lt 10 ]; do
  sleep 3
  _still_in_pulse="$(curl -s -m 10 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/streams" \
    | jq --arg id "${STREAM_ID}" \
      '[(.items // [])[] | select(.stream_id == $id)] | length' \
    2>/dev/null || echo 1)"
  if [ "${_still_in_pulse}" -eq 0 ]; then
    _now="$(date +%s)"
    _end_latency_s=$(( _now - _stop_ts ))
    log "Pulse detected stream end after ${_end_latency_s} s (poll attempt $(( _i + 1 )))"
    break
  fi
  log "Stream still in Pulse (count=${_still_in_pulse}, attempt $(( _i + 1 ))/10)"
  _i=$(( _i + 1 ))
done

capture_pulse "/live/streams" "phase2-stream-gone"

if [ "${_end_latency_s}" -eq 999 ]; then
  log "FAIL: Pulse did not detect stream end within 30 s"
  _end_latency_s=30
fi

assert_lte "${_end_latency_s}" 10 "${SCENARIO} stream end detected by Pulse within 10 s (PRD budget)" || true

log "end_detection_latency=${_end_latency_s}s"
{
  printf 'stream_start_latency_s=%s\nstream_end_latency_s=%s\n' "${_start_latency_s}" "${_end_latency_s}"
  printf 'note=poll_path_covers_webhook_gap_for_lifecycle_events\n'
  printf 'note=AMS_303_unsigned_hooks_bypassed_by_REST_poll\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
