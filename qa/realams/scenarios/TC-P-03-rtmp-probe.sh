#!/usr/bin/env bash
# qa/realams/scenarios/TC-P-03-rtmp-probe.sh
#
# TC-P-03: RTMP probe — handshake test
#
# Assertion matrix row:
#   Steps:        1. Create RTMP probe → rtmp://<ams>:1935/LiveApp
#                 2. Poll /api/v1/probes/{id}/results up to 180 s
#   AMS truth:    AMS completes C0/C1/S0/S1/S2/C2 RTMP handshake
#   Pulse assert: success=true, signaling_state=handshake_complete, connect_time_ms > 0
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (probe creation failed)
#
set -euo pipefail

SCENARIO="TC-P-03"
echo "=== ${SCENARIO}: RTMP probe — handshake ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EPOCH="$(date +%s)"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

PROBE_ID=""

# Derive AMS hostname from AMS_URL (http://HOST:PORT → HOST)
_AMS_HOSTPORT="${AMS_URL#*://}"
_AMS_HOST="${_AMS_HOSTPORT%%:*}"
PROBE_RTMP_URL="rtmp://${_AMS_HOST}:1935/LiveApp"

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────
cleanup() {
  if [ -n "${PROBE_ID}" ]; then
    log "CLEANUP: deleting probe ${PROBE_ID}"
    curl -s -m 10 -X DELETE \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/probes/${PROBE_ID}" > /dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

log "probe_rtmp_url=${PROBE_RTMP_URL}"

# ── Create RTMP probe ────────────────────────────────────────────────────────
log "Creating RTMP probe → ${PROBE_RTMP_URL}"
_probe_body="{\"name\":\"tc-p03-rtmp-${EPOCH}\",\"url\":\"${PROBE_RTMP_URL}\",\"protocol\":\"rtmp\",\"interval_s\":30,\"timeout_s\":20,\"enabled\":true}"
_probe_resp="$(curl -s -m 20 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${_probe_body}" \
  "${PULSE_URL}/probes" 2>/dev/null || echo '{}')"

PROBE_ID="$(printf '%s' "${_probe_resp}" | jq -r '.id // empty' 2>/dev/null || true)"

if [ -z "${PROBE_ID}" ]; then
  log "SKIP: probe creation failed — response: ${_probe_resp}"
  printf 'SKIP\nPrecondition unmet: could not create RTMP probe via Pulse API.\nResponse: %s\n' \
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
  assert_eq "no_result" "result_present" "${SCENARIO} RTMP probe result appeared within 180 s" || true
  scenario_verdict
  exit 1
fi

printf '%s' "${_result}" | jq . > "${EVIDENCE_DIR}/probe-result-first.json"
log "Result: $(printf '%s' "${_result}" | jq -c '{success,signaling_state,connect_time_ms,error_code}')"

# ── Assertions ───────────────────────────────────────────────────────────────
_success="$(printf '%s' "${_result}" | jq -r 'if .success == true then "true" else "false" end')"
assert_eq "${_success}" "true" "${SCENARIO} success=true" || true

_signaling="$(printf '%s' "${_result}" | jq -r '.signaling_state // ""')"
assert_eq "${_signaling}" "handshake_complete" "${SCENARIO} signaling_state=handshake_complete" || true

_connect_ms="$(printf '%s' "${_result}" | jq '.connect_time_ms // 0' 2>/dev/null || echo 0)"
assert_gte "${_connect_ms}" 1 "${SCENARIO} connect_time_ms > 0" || true

log "result_convergence_s=${_result_secs}"
printf 'probe_result_convergence_s=%s\nconnect_time_ms=%s\n' "${_result_secs}" "${_connect_ms}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
