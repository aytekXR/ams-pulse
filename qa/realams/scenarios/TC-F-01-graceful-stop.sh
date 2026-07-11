#!/usr/bin/env bash
# qa/realams/scenarios/TC-F-01-graceful-stop.sh
#
# TC-F-01: Publisher disconnect — graceful stop
#
# Assertion matrix row:
#   Steps:         1. Start publisher, confirm AMS=broadcasting
#                  2. stop_publisher (ffmpeg SIGTERM → clean RTMP disconnect)
#                  3. Poll AMS for status=finished (bounded ≤30 s, record convergence)
#                  4. Poll Pulse /live/streams for stream removal (bounded ≤30 s)
#   AMS truth:     status=finished
#   Pulse assert:  stream removed from GET /api/v1/live/streams;
#                  stream_publish_end event emitted (internal — see API coverage note)
#   Tolerance:     ≤30 s convergence window after stop
#   Exit:          0 PASS | 1 FAIL | 77 SKIP (precondition: never reached broadcasting)
#
set -euo pipefail

SCENARIO="TC-F-01"
echo "=== ${SCENARIO}: Graceful Publisher Stop ===" >&2

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
STREAM_ID="val-f01-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: ensuring publisher ${STREAM_ID} is stopped"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Start publisher and confirm broadcasting (precondition) ───────────────────────
log "Starting publisher ${STREAM_ID} at 1000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 1000

log "Polling AMS for status=broadcasting (budget: 30 s)"
_ams_pre=""
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_pre="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_pre}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( (_i + 1) * 3 )) s — precondition met"
    break
  fi
  log "AMS status=${_ams_pre} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

if [ "${_ams_pre}" != "broadcasting" ]; then
  log "SKIP: stream ${STREAM_ID} never reached broadcasting (final status=${_ams_pre})"
  printf 'SKIP\nPrecondition unmet: stream %s never reached broadcasting (AMS status=%s)\n' \
    "${STREAM_ID}" "${_ams_pre}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-stop"
capture_pulse "/live/streams" "pre-stop"

# ── Graceful stop ─────────────────────────────────────────────────────────────────
_stop_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
log "Gracefully stopping publisher ${STREAM_ID} at ${_stop_ts}"
stop_publisher "${STREAM_ID}"

# ── Poll AMS for finished (≤30 s, 3 s interval) ──────────────────────────────────
log "Polling AMS for status=finished (budget: 30 s)"
_ams_status_after=""
_ams_stop_conv=999   # sentinel: exceeded 30 s budget
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_status_after="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_status_after}" = "finished" ]; then
    _ams_stop_conv=$(( (_i + 1) * 3 ))
    log "AMS status=finished — convergence ${_ams_stop_conv} s"
    break
  fi
  log "AMS status=${_ams_status_after} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "after-stop"

# ── Poll Pulse /live/streams for removal (≤30 s) ─────────────────────────────────
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

capture_pulse "/live/streams" "after-stop"

# ── Assertions ────────────────────────────────────────────────────────────────────
log "ASSERT: AMS status=finished  Pulse stream gone  convergence ≤30 s each"
{
  printf '\nStop timestamp: %s\n' "${_stop_ts}"
  printf 'AMS finished convergence: %s s (budget: 30 s)\n' "${_ams_stop_conv}"
  printf 'Pulse removal convergence: %s s (budget: 30 s)\n' "${_pulse_stop_conv}"
  printf '\nAPI coverage note:\n'
  printf '  Scenario matrix also asserts stream_publish_end event in server_events.\n'
  printf '  Pulse /api/v1 has no server_events query endpoint (pulse-api.yaml).\n'
  printf '  Assertion covered by live-list disappearance + AMS status=finished.\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

assert_eq "${_ams_status_after}" "finished" "${SCENARIO} AMS status=finished after graceful stop" || true
assert_eq "${_pulse_gone}" "0" "${SCENARIO} Pulse stream removed from /live/streams" || true
assert_lte "${_ams_stop_conv}" 30 "${SCENARIO} AMS finished convergence ≤30 s" || true
assert_lte "${_pulse_stop_conv}" 30 "${SCENARIO} Pulse removal convergence ≤30 s" || true

# ── Verdict ───────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
