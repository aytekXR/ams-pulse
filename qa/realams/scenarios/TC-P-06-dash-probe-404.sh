#!/usr/bin/env bash
# qa/realams/scenarios/TC-P-06-dash-probe-404.sh
#
# TC-P-06: DASH probe — AMS without DASH muxing (expect 404)
#
# Assertion matrix row:
#   Steps:        1. Create DASH probe → .mpd URL on test AMS (DASH muxing disabled)
#                 2. Poll /api/v1/probes/{id}/results up to 180 s
#   AMS truth:    AMS returns HTTP 404 for .mpd URL (no DASH muxing)
#   Pulse assert: success=false, error_code=http_4xx — documented expected behaviour
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (probe creation failed)
#
# Note: This is a documented-expected FAIL from AMS. The Pulse probe correctly
# surfaces the HTTP 404 as success=false + error_code=http_4xx, and must NOT crash.
#
set -euo pipefail

SCENARIO="TC-P-06"
echo "=== ${SCENARIO}: DASH probe — AMS without DASH (404) ===" >&2

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

# DASH MPD URL — AMS 3.0.3 does not serve DASH; this will return 404
_DASH_STREAM_ID="val-p06-dash-${EPOCH}"
PROBE_DASH_URL="${AMS_URL}/LiveApp/streams/${_DASH_STREAM_ID}/${_DASH_STREAM_ID}.mpd"

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

log "probe_dash_url=${PROBE_DASH_URL} (DASH disabled on AMS — expect 404)"

# Verify AMS returns 404 for this DASH URL
_ams_code="$(curl -s -m 10 -o /dev/null -w '%{http_code}' "${PROBE_DASH_URL}" 2>/dev/null || echo 0)"
log "AMS pre-check: HTTP ${_ams_code} for .mpd URL"
printf 'ams_mpd_http_code=%s\n' "${_ams_code}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Create DASH probe ────────────────────────────────────────────────────────
log "Creating DASH probe → ${PROBE_DASH_URL}"
_probe_body="{\"name\":\"tc-p06-dash-${EPOCH}\",\"url\":\"${PROBE_DASH_URL}\",\"protocol\":\"dash\",\"interval_s\":30,\"timeout_s\":15,\"enabled\":true}"
_probe_resp="$(curl -s -m 20 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${_probe_body}" \
  "${PULSE_URL}/probes" 2>/dev/null || echo '{}')"

PROBE_ID="$(printf '%s' "${_probe_resp}" | jq -r '.id // empty' 2>/dev/null || true)"

if [ -z "${PROBE_ID}" ]; then
  log "SKIP: probe creation failed — response: ${_probe_resp}"
  printf 'SKIP\nPrecondition unmet: could not create DASH probe via Pulse API.\nResponse: %s\n' \
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
  assert_eq "no_result" "result_present" "${SCENARIO} DASH probe result appeared within 180 s" || true
  scenario_verdict
  exit 1
fi

printf '%s' "${_result}" | jq . > "${EVIDENCE_DIR}/probe-result-first.json"
log "Result: $(printf '%s' "${_result}" | jq -c '{success,error_code,ttfb_ms}')"

# ── Assertions ───────────────────────────────────────────────────────────────
_success="$(printf '%s' "${_result}" | jq -r 'if .success == true then "true" else "false" end')"
assert_eq "${_success}" "false" "${SCENARIO} success=false (DASH not served by AMS 3.0.3)" || true

_error_code="$(printf '%s' "${_result}" | jq -r '.error_code // "null"')"
assert_eq "${_error_code}" "http_4xx" "${SCENARIO} error_code=http_4xx (documented expected)" || true

printf 'probe_result_convergence_s=%s\nnote=DASH_muxing_disabled_on_AMS_303_documented_expected\n' \
  "${_result_secs}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
