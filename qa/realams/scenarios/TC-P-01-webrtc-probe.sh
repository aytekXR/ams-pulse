#!/usr/bin/env bash
# qa/realams/scenarios/TC-P-01-webrtc-probe.sh
#
# TC-P-01: WebRTC probe — live stream
#
# Assertion matrix row:
#   Steps:        1. Start publisher val-p01-<epoch> on LiveApp
#                 2. Create WebRTC probe → ws://<ams>/LiveApp/websocket?streamId=<id>
#                 3. Poll /api/v1/probes/{id}/results up to 180 s
#   AMS truth:    AMS sends signaling offer; ICE completes (connected)
#   Pulse assert: success=true, ice_state=connected, rtt_ms/jitter_ms/loss_pct keys present
#                 (D-075: key may be 0.0 but must NOT be absent)
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (probe creation failed)
#
set -euo pipefail

SCENARIO="TC-P-01"
echo "=== ${SCENARIO}: WebRTC probe — live stream ===" >&2

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
STREAM_ID="val-p01-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

PROBE_ID=""

# Derive AMS host:port for WebSocket URL from AMS_URL (http://HOST:PORT)
_AMS_HOSTPORT="${AMS_URL#*://}"
PROBE_WS_URL="ws://${_AMS_HOSTPORT}/LiveApp/websocket?streamId=${STREAM_ID}"

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping publisher ${STREAM_ID}"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
  if [ -n "${PROBE_ID}" ]; then
    log "CLEANUP: deleting probe ${PROBE_ID}"
    curl -s -m 10 -X DELETE \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/probes/${PROBE_ID}" > /dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  probe_url=${PROBE_WS_URL}"

# ── Start publisher ──────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 1000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 1000

# Wait for AMS to see the stream as broadcasting (up to 30 s, 3 s poll)
log "Polling AMS for broadcasting status (budget: 30 s)"
_ams_status="unknown"
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_status="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_status}" = "broadcasting" ]; then
    log "AMS: stream broadcasting after $(( (_i + 1) * 3 )) s"
    break
  fi
  log "AMS status=${_ams_status} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-probe-ams"

if [ "${_ams_status}" != "broadcasting" ]; then
  log "SKIP: AMS stream never reached broadcasting status — cannot test WebRTC probe"
  printf 'SKIP\nPrecondition unmet: AMS stream %s never reached broadcasting.\nFinal AMS status: %s\n' \
    "${STREAM_ID}" "${_ams_status}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Create WebRTC probe ──────────────────────────────────────────────────────
log "Creating WebRTC probe → ${PROBE_WS_URL}"
_probe_body="{\"name\":\"tc-p01-${STREAM_ID}\",\"url\":\"${PROBE_WS_URL}\",\"protocol\":\"webrtc\",\"interval_s\":30,\"timeout_s\":30,\"enabled\":true}"
_probe_resp="$(curl -s -m 20 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${_probe_body}" \
  "${PULSE_URL}/probes" 2>/dev/null || echo '{}')"

PROBE_ID="$(printf '%s' "${_probe_resp}" | jq -r '.id // empty' 2>/dev/null || true)"

if [ -z "${PROBE_ID}" ]; then
  log "SKIP: probe creation failed — response: ${_probe_resp}"
  printf 'SKIP\nPrecondition unmet: could not create WebRTC probe via Pulse API.\nResponse: %s\n' \
    "${_probe_resp}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

log "Probe created: id=${PROBE_ID}"
printf '%s' "${_probe_resp}" | jq . > "${EVIDENCE_DIR}/probe-create.json"

# ── Poll probe results (up to 180 s, 3 s interval) ──────────────────────────
log "Polling probe results (budget: 180 s)"
_result=""
_result_secs=999
_i=0
while [ "${_i}" -lt 60 ]; do
  sleep 3
  _results_resp="$(curl -s -m 15 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/probes/${PROBE_ID}/results" 2>/dev/null || echo '{}')"
  _result="$(printf '%s' "${_results_resp}" | jq '.items[0] // empty' 2>/dev/null || true)"
  if [ -n "${_result}" ]; then
    _result_secs=$(( (_i + 1) * 3 ))
    log "Got probe result after ${_result_secs} s"
    break
  fi
  _i=$(( _i + 1 ))
done

capture_pulse "/probes/${PROBE_ID}/results" "probe-results"

if [ -z "${_result}" ]; then
  log "FAIL: no probe result within 180 s"
  assert_eq "no_result" "result_present" "${SCENARIO} probe result appeared within 180 s" || true
  scenario_verdict
  exit 1
fi

printf '%s' "${_result}" | jq . > "${EVIDENCE_DIR}/probe-result-first.json"
log "Result: $(printf '%s' "${_result}" | jq -c '{success,ice_state,signaling_state,error_code}')"

# ── Assertions ───────────────────────────────────────────────────────────────
_success="$(printf '%s' "${_result}" | jq -r '.success // false')"
assert_eq "${_success}" "true" "${SCENARIO} success=true" || true

_signaling="$(printf '%s' "${_result}" | jq -r '.signaling_state // ""')"
assert_eq "${_signaling}" "offer_received" "${SCENARIO} signaling_state=offer_received" || true

_ice="$(printf '%s' "${_result}" | jq -r '.ice_state // "absent"')"
assert_eq "${_ice}" "connected" "${SCENARIO} ice_state=connected" || true

# D-075: rtt_ms / jitter_ms / loss_pct keys must exist (may be 0.0, never absent)
_has_rtt="$(printf '%s' "${_result}" | jq 'has("rtt_ms")' 2>/dev/null || echo false)"
assert_eq "${_has_rtt}" "true" "${SCENARIO} rtt_ms key present (D-075)" || true

_has_jitter="$(printf '%s' "${_result}" | jq 'has("jitter_ms")' 2>/dev/null || echo false)"
assert_eq "${_has_jitter}" "true" "${SCENARIO} jitter_ms key present (D-075)" || true

_has_loss="$(printf '%s' "${_result}" | jq 'has("loss_pct")' 2>/dev/null || echo false)"
assert_eq "${_has_loss}" "true" "${SCENARIO} loss_pct key present (D-075)" || true

log "result_convergence_s=${_result_secs}"
printf 'probe_result_convergence_s=%s\n' "${_result_secs}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
