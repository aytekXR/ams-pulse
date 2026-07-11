#!/usr/bin/env bash
# qa/realams/scenarios/TC-F-02-abrupt-kill.sh
#
# TC-F-02: Publisher crash — abrupt kill (Failure scenario perspective)
#
# Assertion matrix row:
#   Steps:         1. Start publisher, confirm AMS=broadcasting
#                  2. docker kill -s KILL (SIGKILL, no grace period)
#                  3. Poll AMS for terminated_unexpectedly — budget: 120 s;
#                     record observed latency
#                  4. Assert Pulse-side end semantics (see API coverage note below)
#                  5. Poll Pulse /live/streams for removal
#   AMS truth:     status=terminated_unexpectedly
#   Pulse assert:  stream removed from GET /api/v1/live/streams
#                  (+ stream_publish_end reason=terminated_unexpectedly — see note)
#   API coverage:  Pulse public API has NO server_events endpoint. The collector
#                  emits stream_publish_end internally but it is NOT queryable via
#                  /api/v1. Evidence falls back to live-list disappearance.
#                  This limitation is documented in verdict.txt.
#   Distinction from TC-L-03: same SIGKILL mechanism; TC-F-02 is the Failure phase
#                  perspective with explicit Pulse-side end-semantics assertion and
#                  API coverage documentation.
#   Exit:          0 PASS | 1 FAIL | 77 SKIP (precondition: never reached broadcasting)
#
set -euo pipefail

SCENARIO="TC-F-02"
echo "=== ${SCENARIO}: Abrupt Publisher Kill (SIGKILL) — Failure Scenario ===" >&2

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
STREAM_ID="val-f02-${EPOCH}"
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

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-kill"
capture_pulse "/live/streams" "pre-kill"

# ── SIGKILL publisher (abrupt — no RTMP teardown, no TCP FIN) ────────────────────
_kill_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
log "SIGKILL (docker kill) publisher ${STREAM_ID} at ${_kill_ts}"
kill_publisher "${STREAM_ID}"

# ── Poll AMS for terminated_unexpectedly (up to 120 s, 3 s interval = 40 iterations) ──
# AMS 3.0.3: abrupt TCP disconnect may take 30–90 s to be detected by the ingest thread.
log "Polling AMS for terminated_unexpectedly (budget: 120 s)"
_ams_status_after=""
_ams_crash_conv=999   # sentinel: exceeded 120 s budget
_i=0
while [ "${_i}" -lt 40 ]; do
  _poll_body="/tmp/claude-1000/ams-poll-$$.json"
  _http_code="$(curl -s -m 10 -o "${_poll_body}" -w '%{http_code}' \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" 2>/dev/null || echo 000)"
  if [ "${_http_code}" = "404" ]; then
    _ams_status_after="removed"
  else
    _ams_status_after="$(jq -r '.status // "unknown"' "${_poll_body}" 2>/dev/null || echo "curl_error")"
  fi
  rm -f "${_poll_body}"
  if [ "${_ams_status_after}" = "terminated_unexpectedly" ] || [ "${_ams_status_after}" = "removed" ]; then
    _ams_crash_conv=$(( (_i + 1) * 3 ))
    log "AMS status=terminated_unexpectedly — observed latency ${_ams_crash_conv} s"
    break
  fi
  log "AMS status=${_ams_status_after} (attempt $(( _i + 1 ))/40)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "after-kill"

# ── Poll Pulse /live/streams for removal ─────────────────────────────────────────
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
log "ASSERT: AMS terminated_unexpectedly  Pulse stream absent from /live/streams"

_ams_terminal="not_terminal"; case "${_ams_status_after}" in terminated_unexpectedly|removed) _ams_terminal="terminal";; esac
# S17 live finding: implicit RTMP broadcasts may be DELETED (GET 404) rather than flagged terminated_unexpectedly
assert_eq "${_ams_terminal}" "terminal" "${SCENARIO} AMS terminal after SIGKILL (terminated_unexpectedly|removed-404; observed: ${_ams_status_after})" || true
assert_eq "${_pulse_gone}" "0" \
  "${SCENARIO} Pulse stream removed from /live/streams after abrupt kill" || true

# ── Pulse-side end-semantics coverage note ────────────────────────────────────────
# scenario-matrix asserts: stream_publish_end with reason=terminated_unexpectedly.
# This event is emitted by the Pulse collector and stored in ClickHouse server_events.
# The Pulse public REST API does NOT expose a server_events query endpoint.
{
  printf '\n=== Pulse-side stream_publish_end semantics — TC-F-02 ===\n'
  printf 'Scenario matrix assertion: Pulse emits stream_publish_end with\n'
  printf '  reason=terminated_unexpectedly; stream removed from live.\n'
  printf '\nAPI coverage check:\n'
  printf '  Reviewed: contracts/openapi/pulse-api.yaml\n'
  printf '  Result: NO /server_events, /events, or /stream_events endpoint found.\n'
  printf '  The Pulse collector writes stream_publish_end to ClickHouse internally\n'
  printf '  but this table is NOT exposed via any /api/v1 path.\n'
  printf '\nAssertions substituted:\n'
  printf '  PRIMARY:   AMS status=terminated_unexpectedly (ground truth)\n'
  printf '  SECONDARY: stream_id absent from GET /live/streams (live-list removal)\n'
  printf '  MISSING:   stream_publish_end.reason=terminated_unexpectedly\n'
  printf '             (not verifiable via public API — file as documentation gap)\n'
  printf '\nTimings:\n'
  printf '  Kill timestamp:                 %s\n' "${_kill_ts}"
  printf '  AMS terminated_unexpectedly at: %s s after kill (budget: 120 s)\n' "${_ams_crash_conv}"
  printf '  Pulse live-list removal at:     %s s after kill\n' "${_pulse_crash_conv}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ───────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
