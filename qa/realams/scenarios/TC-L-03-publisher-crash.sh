#!/usr/bin/env bash
# qa/realams/scenarios/TC-L-03-publisher-crash.sh
#
# TC-L-03: Publisher crash (SIGKILL) → AMS terminated_unexpectedly → Pulse stream gone
#
# Assertion matrix row:
#   Steps:         1. Start publisher, confirm AMS=broadcasting
#                  2. SIGKILL the ffmpeg container (docker kill, no grace period)
#                  3. Poll AMS for terminated_unexpectedly — budget: 120 s (not 15 s),
#                     AMS may take >15 s to detect abrupt disconnect; record latency
#                  4. Poll Pulse /live/streams for stream removal — budget: 30 s
#   AMS truth:     status=terminated_unexpectedly
#   Pulse assert:  stream absent from GET /api/v1/live/streams
#   API note:      scenario-matrix also asserts server_events event_type=stream_publish_end.
#                  The Pulse public API (/api/v1) has NO server_events query endpoint
#                  (checked contracts/openapi/pulse-api.yaml). Assertion substituted by
#                  live-list disappearance. Limitation noted in verdict.txt.
#   Exit:          0 PASS | 1 FAIL | 77 SKIP (precondition: never reached broadcasting)
#
set -euo pipefail

SCENARIO="TC-L-03"
echo "=== ${SCENARIO}: Publisher Crash (SIGKILL) ===" >&2

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
STREAM_ID="val-l03-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: ensuring publisher ${STREAM_ID} is gone"
  kill_publisher "${STREAM_ID}" 2>/dev/null || true
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Start publisher and wait for broadcasting (precondition) ──────────────────────
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

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-kill"
capture_pulse "/live/streams" "pre-kill"

# ── SIGKILL the publisher ─────────────────────────────────────────────────────────
_kill_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
log "SIGKILL publisher ${STREAM_ID} at ${_kill_ts}"
kill_publisher "${STREAM_ID}"

# ── Poll AMS for terminated_unexpectedly (up to 120 s, 3 s interval = 40 iterations) ──
# AMS may take well over 15 s after an abrupt disconnect to mark the stream.
log "Polling AMS for terminated_unexpectedly (budget: 120 s)"
_ams_status_after=""
_ams_crash_conv=999   # sentinel: exceeded 120 s budget
_i=0
while [ "${_i}" -lt 40 ]; do
  _ams_status_after="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_status_after}" = "terminated_unexpectedly" ]; then
    _ams_crash_conv=$(( (_i + 1) * 3 ))
    log "AMS status=terminated_unexpectedly — observed latency ${_ams_crash_conv} s"
    break
  fi
  log "AMS status=${_ams_status_after} (attempt $(( _i + 1 ))/40)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "after-kill"

# ── Poll Pulse /live/streams for removal (≤30 s) ─────────────────────────────────
log "Polling Pulse /live/streams for ${STREAM_ID} removal (budget: 30 s)"
_pulse_gone=99
_pulse_crash_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _pulse_gone="$(curl -s -m 10 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/streams" \
    | jq --arg id "${STREAM_ID}" \
      '[(.items // [])[] | select(.stream_id == $id)] | length' \
    2>/dev/null || echo 99)"
  if [ "${_pulse_gone}" = "0" ]; then
    _pulse_crash_conv=$(( (_i + 1) * 3 ))
    log "Pulse stream absent from /live/streams after ${_pulse_crash_conv} s"
    break
  fi
  log "Pulse stream still in list (count=${_pulse_gone}, attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_pulse "/live/streams" "after-kill"

# ── Assertions ────────────────────────────────────────────────────────────────────
log "ASSERT: AMS terminated_unexpectedly  Pulse stream gone"

assert_eq "${_ams_status_after}" "terminated_unexpectedly" \
  "${SCENARIO} AMS status=terminated_unexpectedly after SIGKILL" || true
assert_eq "${_pulse_gone}" "0" \
  "${SCENARIO} Pulse stream absent from /live/streams after crash" || true

# ── server_events limitation note ────────────────────────────────────────────────
{
  printf '\n--- server_events API coverage note ---\n'
  printf 'Scenario matrix asserts: server_events event_type=stream_publish_end\n'
  printf 'LIMITATION: Pulse /api/v1 has no server_events query endpoint.\n'
  printf '  Checked: contracts/openapi/pulse-api.yaml — no /server_events, /events,\n'
  printf '  or /stream_events path found. The collector emits stream_publish_end\n'
  printf '  internally to ClickHouse but it is not exposed via the public REST API.\n'
  printf 'Substituted assertion: stream_id absent from GET /live/streams.\n'
  printf 'Kill timestamp:                 %s\n' "${_kill_ts}"
  printf 'AMS terminated_unexpectedly at: %s s after kill (budget: 120 s)\n' "${_ams_crash_conv}"
  printf 'Pulse live-list removal at:     %s s after kill\n' "${_pulse_crash_conv}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ───────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
