#!/usr/bin/env bash
# qa/realams/scenarios/TC-F-08-network-reconnect.sh
#
# TC-F-08: Network reconnection after Docker bridge disconnect
#
# Assertion matrix row:
#   Setup:        Publisher pulse-pub-val-val-f08-<epoch> on the docker default bridge.
#   Steps:        1. Start publisher; wait for AMS broadcasting
#                 2. docker network disconnect bridge <container> (no sudo needed)
#                 3. Wait 10 s; assert AMS shows terminal for the disconnected stream
#                 4. docker network reconnect bridge <container> (if still alive)
#                    OR restart publisher with same STREAM_ID (if container exited on --rm)
#                 5. Assert: AMS shows a NEW broadcasting session for the stream ID
#                 6. Assert: Pulse shows stream removal then reappearance
#                    (two lifecycle transitions — record observed timings)
#   AMS truth:    Terminal (finished|removed-404) during gap; then broadcasting again
#   Pulse assert: Stream disappears from /live/streams during gap; reappears after reconnect
#   Tolerance:    Both transition polls budget 30 s each
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (publisher never reached broadcasting)
#
set -euo pipefail

SCENARIO="TC-F-08"
echo "=== ${SCENARIO}: Network Reconnection After Docker Bridge Disconnect ===" >&2

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

# ── Per-run identifiers ─────────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-f08-${EPOCH}"
PUB_CNAME="pulse-pub-val-${STREAM_ID}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping publisher ${STREAM_ID} (both possible container states)"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
  # Also attempt kill in case the container is in an unknown state
  sg docker -c "docker kill ${PUB_CNAME}" > /dev/null 2>&1 || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PUB_CNAME=${PUB_CNAME}"
log "PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: Start publisher and confirm broadcasting (precondition)
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 1: starting publisher ${STREAM_ID} at 1000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 1000

log "Polling AMS for status=broadcasting (budget: 30 s)"
_ams_pre=""
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_pre="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_pre}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( (_i + 1) * 3 )) s"
    break
  fi
  log "AMS status=${_ams_pre} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

if [ "${_ams_pre}" != "broadcasting" ]; then
  log "SKIP: stream ${STREAM_ID} never reached broadcasting (precondition unmet)"
  {
    echo "SKIP"
    echo "Publisher ${STREAM_ID} never reached AMS status=broadcasting in 30 s."
    echo "Cannot test network reconnect without an established RTMP session."
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-disconnect"
capture_pulse "/live/streams" "pre-disconnect"

# Record Pulse pre-disconnect state
_pulse_pre_count="$(curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams" \
  | jq --arg id "${STREAM_ID}" '[(.items // [])[] | select(.stream_id == $id)] | length' \
  2>/dev/null || echo 0)"
log "Pulse pre-disconnect: stream_count=${_pulse_pre_count}"

# Allow 5 s of stable broadcasting before we disconnect
log "Allowing 5 s of stable broadcasting before disconnect"
sleep 5

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: Disconnect publisher from Docker bridge network
# ─────────────────────────────────────────────────────────────────────────────
_disconnect_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
log "Phase 2: disconnecting ${PUB_CNAME} from bridge network at ${_disconnect_ts}"

_disconnect_out="$(sg docker -c "docker network disconnect bridge ${PUB_CNAME}" \
  2>&1 || echo "DISCONNECT_FAILED")"
log "Disconnect result: ${_disconnect_out}"
{
  printf 'disconnect_ts=%s  disconnect_result=%s\n' "${_disconnect_ts}" "${_disconnect_out}"
} >> "${EVIDENCE_DIR}/timeline.txt"

if printf '%s' "${_disconnect_out}" | grep -q "DISCONNECT_FAILED"; then
  log "NOTE: disconnect failed — container may be on a different network; continuing with 10 s wait"
  # Still proceed — the publish might fail for other reasons; we'll assert terminal state
fi

# Wait 10 s for AMS to detect the lost connection
log "Waiting 10 s for AMS to detect disconnected publisher"
sleep 10

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: Assert AMS terminal state during gap
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 3: polling AMS for terminal state during gap (budget: 30 s)"
_ams_gap_status=""
_ams_gap_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _gap_body="/tmp/claude-1000/ams-f08-gap-$$.json"
  _http_code="$(curl -s -m 10 -o "${_gap_body}" -w '%{http_code}' \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" 2>/dev/null || echo 000)"
  if [ "${_http_code}" = "404" ]; then
    _ams_gap_status="removed"
  else
    _ams_gap_status="$(jq -r '.status // "unknown"' "${_gap_body}" 2>/dev/null || echo "curl_error")"
  fi
  rm -f "${_gap_body}"
  if [ "${_ams_gap_status}" = "removed" ] || \
     [ "${_ams_gap_status}" = "finished" ] || \
     [ "${_ams_gap_status}" = "terminated_unexpectedly" ]; then
    _ams_gap_conv=$(( (_i + 1) * 3 ))
    log "AMS terminal state=${_ams_gap_status} after ${_ams_gap_conv} s"
    break
  fi
  log "AMS status=${_ams_gap_status} http=${_http_code} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "gap-ams"

# Poll Pulse for stream removal during gap (budget: 30 s)
log "Polling Pulse for ${STREAM_ID} removal during gap (budget: 30 s)"
_pulse_gap_count=99
_pulse_gap_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _pulse_gap_count="$(curl -s -m 10 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/streams" \
    | jq --arg id "${STREAM_ID}" \
      '[(.items // [])[] | select(.stream_id == $id)] | length' \
    2>/dev/null || echo 99)"
  if [ "${_pulse_gap_count}" = "0" ]; then
    _pulse_gap_conv=$(( (_i + 1) * 3 ))
    log "Pulse stream absent after ${_pulse_gap_conv} s"
    break
  fi
  log "Pulse stream still in list (count=${_pulse_gap_count}, attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_pulse "/live/streams" "gap-pulse"

{
  printf 'gap: AMS status=%s  convergence=%ss\n' "${_ams_gap_status}" "${_ams_gap_conv}"
  printf 'gap: Pulse stream_count=%s  convergence=%ss\n' "${_pulse_gap_count}" "${_pulse_gap_conv}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 4: Reconnect or restart publisher
# ─────────────────────────────────────────────────────────────────────────────
# The ffmpeg container was launched with --rm, so if ffmpeg crashes on disconnect
# the container is gone. We need to either:
#   a) If still running: reconnect the bridge, wait for ffmpeg to reconnect (unlikely
#      since RTMP does not auto-reconnect)
#   b) If gone or reconnect not feasible: restart the publisher with the same STREAM_ID
#
log "Phase 4: checking if publisher container is still running"
_cname_alive="$(sg docker -c "docker ps --filter name=${PUB_CNAME} --format '{{.Names}}'" \
  2>/dev/null || echo "")"
log "Container alive check: '${_cname_alive}'"

_reconnect_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [ -n "${_cname_alive}" ]; then
  log "Container is still running — attempting bridge reconnect + publisher restart"
  # Reconnect to bridge (restores network access)
  _reconnect_out="$(sg docker -c "docker network connect bridge ${PUB_CNAME}" \
    2>&1 || echo "RECONNECT_FAILED")"
  log "Bridge reconnect result: ${_reconnect_out}"
  # RTMP doesn't auto-reconnect; stop the container and restart same stream ID
  log "Stopping existing container (RTMP won't auto-reconnect)"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
  sleep 2
fi

log "Restarting publisher ${STREAM_ID} at 1000 kbps (RTMP reconnect)"
start_publisher "${STREAM_ID}" "LiveApp" 1000
{
  printf 'reconnect_ts=%s\n' "${_reconnect_ts}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 5: Assert new broadcasting session appeared
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 5: polling AMS for NEW broadcasting session (budget: 30 s)"
_ams_new_status=""
_ams_new_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_new_status="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_new_status}" = "broadcasting" ]; then
    _ams_new_conv=$(( (_i + 1) * 3 ))
    log "AMS new broadcasting session detected after ${_ams_new_conv} s"
    break
  fi
  log "AMS status=${_ams_new_status} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "reconnected-ams"

# Poll Pulse for stream reappearance (budget: 30 s)
log "Polling Pulse for ${STREAM_ID} reappearance after reconnect (budget: 30 s)"
_pulse_new_count=0
_pulse_new_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _pulse_new_count="$(curl -s -m 10 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/streams" \
    | jq --arg id "${STREAM_ID}" \
      '[(.items // [])[] | select(.stream_id == $id)] | length' \
    2>/dev/null || echo 0)"
  if [ "${_pulse_new_count}" -ge 1 ] 2>/dev/null; then
    _pulse_new_conv=$(( (_i + 1) * 3 ))
    log "Pulse stream reappeared after ${_pulse_new_conv} s"
    break
  fi
  log "Pulse stream not yet visible (count=${_pulse_new_count}, attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_pulse "/live/streams" "reconnected-pulse"

{
  printf 'reconnect: AMS new_status=%s  convergence=%ss\n' "${_ams_new_status}" "${_ams_new_conv}"
  printf 'reconnect: Pulse stream_count=%s  convergence=%ss\n' "${_pulse_new_count}" "${_pulse_new_conv}"
  printf 'Two lifecycle transitions summary:\n'
  printf '  1. Disconnect→terminal: AMS=%s Pulse_removal=%ss\n' "${_ams_gap_status}" "${_pulse_gap_conv}"
  printf '  2. Reconnect→broadcasting: AMS=%ss Pulse_reappear=%ss\n' "${_ams_new_conv}" "${_pulse_new_conv}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Assertions
# ─────────────────────────────────────────────────────────────────────────────

# Gap assertions: AMS must have reached terminal during disconnect
_ams_gap_terminal="not_terminal"
case "${_ams_gap_status}" in
  finished|removed|terminated_unexpectedly) _ams_gap_terminal="terminal" ;;
esac
assert_eq "${_ams_gap_terminal}" "terminal" \
  "${SCENARIO} AMS terminal during gap (${_ams_gap_status}); convergence=${_ams_gap_conv}s" || true

# Pulse must have seen the stream disappear
assert_eq "${_pulse_gap_count}" "0" \
  "${SCENARIO} Pulse stream absent from /live/streams during gap (convergence=${_pulse_gap_conv}s)" || true
assert_lte "${_pulse_gap_conv}" 30 \
  "${SCENARIO} Pulse removal convergence ≤30 s" || true

# Reconnect assertions: AMS must show broadcasting again
assert_eq "${_ams_new_status}" "broadcasting" \
  "${SCENARIO} AMS shows new broadcasting session after reconnect (convergence=${_ams_new_conv}s)" || true
assert_lte "${_ams_new_conv}" 30 \
  "${SCENARIO} AMS new broadcasting convergence ≤30 s" || true

# Pulse must show the stream again
assert_gte "${_pulse_new_count}" 1 \
  "${SCENARIO} Pulse stream reappears in /live/streams after reconnect (convergence=${_pulse_new_conv}s)" || true
assert_lte "${_pulse_new_conv}" 30 \
  "${SCENARIO} Pulse reappearance convergence ≤30 s" || true

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
