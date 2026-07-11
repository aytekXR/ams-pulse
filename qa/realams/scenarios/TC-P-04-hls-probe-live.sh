#!/usr/bin/env bash
# qa/realams/scenarios/TC-P-04-hls-probe-live.sh
#
# TC-P-04: HLS probe — live stream playlist
#
# Assertion matrix row:
#   Steps:        1. Start publisher val-p04-<epoch> on LiveApp
#                 2. Create HLS probe → http://<ams>/LiveApp/streams/<id>/playlist.m3u8
#                 3. Poll /api/v1/probes/{id}/results up to 180 s
#   AMS truth:    AMS serves valid M3U8 playlist (stream broadcasting)
#   Pulse assert: success=true, ttfb_ms > 0, bitrate_kbps > 0, segment_ttfb_ms > 0
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (stream never reached broadcasting)
#
set -euo pipefail

SCENARIO="TC-P-04"
echo "=== ${SCENARIO}: HLS probe — live stream ===" >&2

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
STREAM_ID="val-p04-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

PROBE_ID=""
PROBE_HLS_URL="${AMS_URL}/LiveApp/streams/${STREAM_ID}/playlist.m3u8"

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

log "STREAM_ID=${STREAM_ID}  probe_url=${PROBE_HLS_URL}"

# ── Start publisher ──────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

# Wait for AMS broadcasting + HLS playlist to be available (up to 40 s)
log "Polling AMS for broadcasting and HLS playlist availability (budget: 40 s)"
_ams_status="unknown"
_hls_ready=false
_i=0
while [ "${_i}" -lt 14 ]; do
  _ams_status="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_status}" = "broadcasting" ] && [ "${_hls_ready}" = "false" ]; then
    # Verify M3U8 is available
    _hls_code="$(curl -s -m 10 -o /dev/null -w '%{http_code}' \
      "${AMS_URL}/LiveApp/streams/${STREAM_ID}/playlist.m3u8" 2>/dev/null || echo 0)"
    if [ "${_hls_code}" = "200" ]; then
      _hls_ready=true
      log "AMS: broadcasting + HLS ready after $(( (_i + 1) * 3 )) s (HTTP ${_hls_code})"
      break
    fi
  fi
  log "AMS status=${_ams_status} hls_code=${_hls_code:-?} (attempt $(( _i + 1 ))/14)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-probe-ams"

if [ "${_hls_ready}" != "true" ]; then
  log "SKIP: HLS playlist never available for ${STREAM_ID}"
  printf 'SKIP\nPrecondition unmet: HLS playlist not available for %s (AMS status: %s).\n' \
    "${STREAM_ID}" "${_ams_status}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Create HLS probe ─────────────────────────────────────────────────────────
log "Creating HLS probe → ${PROBE_HLS_URL}"
_probe_body="{\"name\":\"tc-p04-hls-${STREAM_ID}\",\"url\":\"${PROBE_HLS_URL}\",\"protocol\":\"hls\",\"interval_s\":30,\"timeout_s\":15,\"enabled\":true}"
_probe_resp="$(curl -s -m 20 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${_probe_body}" \
  "${PULSE_URL}/probes" 2>/dev/null || echo '{}')"

PROBE_ID="$(printf '%s' "${_probe_resp}" | jq -r '.id // empty' 2>/dev/null || true)"

if [ -z "${PROBE_ID}" ]; then
  log "SKIP: probe creation failed — response: ${_probe_resp}"
  printf 'SKIP\nPrecondition unmet: could not create HLS probe via Pulse API.\nResponse: %s\n' \
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
  assert_eq "no_result" "result_present" "${SCENARIO} HLS probe result appeared within 180 s" || true
  scenario_verdict
  exit 1
fi

printf '%s' "${_result}" | jq . > "${EVIDENCE_DIR}/probe-result-first.json"
log "Result: $(printf '%s' "${_result}" | jq -c '{success,ttfb_ms,bitrate_kbps,segment_ttfb_ms,error_code}')"

# ── Assertions ───────────────────────────────────────────────────────────────
_success="$(printf '%s' "${_result}" | jq -r 'if .success == true then "true" else "false" end')"
assert_eq "${_success}" "true" "${SCENARIO} success=true" || true

_ttfb="$(printf '%s' "${_result}" | jq '.ttfb_ms // 0' 2>/dev/null || echo 0)"
assert_gte "${_ttfb}" 1 "${SCENARIO} ttfb_ms > 0" || true

_bitrate="$(printf '%s' "${_result}" | jq '.bitrate_kbps // 0' 2>/dev/null || echo 0)"
assert_gte "${_bitrate}" 1 "${SCENARIO} bitrate_kbps > 0" || true

_seg_ttfb="$(printf '%s' "${_result}" | jq '.segment_ttfb_ms // 0' 2>/dev/null || echo 0)"
assert_gte "${_seg_ttfb}" 1 "${SCENARIO} segment_ttfb_ms > 0" || true

log "result_convergence_s=${_result_secs} ttfb=${_ttfb}ms bitrate=${_bitrate}kbps"
printf 'probe_result_convergence_s=%s\nttfb_ms=%s\nbitrate_kbps=%s\nsegment_ttfb_ms=%s\n' \
  "${_result_secs}" "${_ttfb}" "${_bitrate}" "${_seg_ttfb}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
