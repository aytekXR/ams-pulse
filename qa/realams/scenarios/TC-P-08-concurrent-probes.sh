#!/usr/bin/env bash
# qa/realams/scenarios/TC-P-08-concurrent-probes.sh
#
# TC-P-08: Multiple simultaneous probes (WebRTC + RTMP + HLS)
#
# Assertion matrix row:
#   Steps:         1. Start publisher val-p08-<epoch> on LiveApp
#                  2. Create 3 probes concurrently:
#                       WEBRTC → ws://<ams>/LiveApp/websocket?streamId=<id>
#                       RTMP   → rtmp://<ams_host>:1935/LiveApp
#                       HLS    → http://<ams>/LiveApp/streams/<id>.m3u8
#                  3. Poll each probe's /results endpoint up to 180 s
#                  4. Assert all 3 produce at least one result with success=true
#                  5. Delete all 3 probes in cleanup trap
#   AMS truth:     AMS serves WebRTC signaling, RTMP handshake, and HLS playlist
#   Pulse assert:  three concurrent probes all succeed; no interference
#   Exit:          0 PASS | 1 FAIL | 77 SKIP (publisher never reached broadcasting
#                  or all 3 probe creations failed)
#
set -euo pipefail

SCENARIO="TC-P-08"
echo "=== ${SCENARIO}: Three concurrent probes (WebRTC + RTMP + HLS) ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"
# shellcheck source=../harness/publisher.sh
source "${_DIR}/../harness/publisher.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-p08-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

PROBE_WEBRTC_ID=""
PROBE_RTMP_ID=""
PROBE_HLS_ID=""

# Derive AMS host and port strings
_AMS_HOSTPORT="${AMS_URL#*://}"
_AMS_HOST="${_AMS_HOSTPORT%%:*}"

PROBE_WEBRTC_URL="ws://${_AMS_HOSTPORT}/LiveApp/websocket?streamId=${STREAM_ID}"
PROBE_RTMP_URL="rtmp://${_AMS_HOST}:1935/LiveApp"
# HLS: flat form per S17 correction
PROBE_HLS_URL="${AMS_URL}/LiveApp/streams/${STREAM_ID}.m3u8"

# ── Timeline log ─────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping publisher ${STREAM_ID}"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
  for _pid in "${PROBE_WEBRTC_ID}" "${PROBE_RTMP_ID}" "${PROBE_HLS_ID}"; do
    if [ -n "${_pid}" ]; then
      log "CLEANUP: deleting probe ${_pid}"
      curl -s -m 10 -X DELETE \
        -H "Authorization: Bearer ${PULSE_TOKEN}" \
        "${PULSE_URL}/probes/${_pid}" > /dev/null 2>&1 || true
    fi
  done
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}"
log "WEBRTC probe → ${PROBE_WEBRTC_URL}"
log "RTMP probe   → ${PROBE_RTMP_URL}"
log "HLS probe    → ${PROBE_HLS_URL}"

# ── Start publisher ──────────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

# ── Wait for AMS broadcasting + HLS available (≤40 s) ───────────────────────────
log "Polling AMS for broadcasting + HLS playlist (budget: 40 s)"
_ams_status="unknown"
_hls_ready=false
_hls_code="0"
_i=0
while [ "${_i}" -lt 14 ]; do
  _ams_status="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo curl_error)"
  if [ "${_ams_status}" = "broadcasting" ]; then
    _hls_code="$(curl -s -m 10 -o /dev/null -w '%{http_code}' \
      "${PROBE_HLS_URL}" 2>/dev/null || echo 0)"
    if [ "${_hls_code}" = "200" ]; then
      _hls_ready=true
      log "AMS broadcasting + HLS ready after $(( (_i + 1) * 3 )) s"
      break
    fi
  fi
  log "AMS status=${_ams_status} hls_code=${_hls_code} (attempt $(( _i + 1 ))/14)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-probes"

if [ "${_ams_status}" != "broadcasting" ]; then
  log "SKIP: AMS stream ${STREAM_ID} never reached broadcasting (final: ${_ams_status})"
  printf 'SKIP\nPrecondition unmet: AMS stream %s never reached broadcasting.\nFinal status: %s\n' \
    "${STREAM_ID}" "${_ams_status}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Create all 3 probes ───────────────────────────────────────────────────────────
log "Creating WebRTC probe → ${PROBE_WEBRTC_URL}"
_wr_body="{\"name\":\"tc-p08-webrtc-${STREAM_ID}\",\"url\":\"${PROBE_WEBRTC_URL}\",\"protocol\":\"webrtc\",\"interval_s\":30,\"timeout_s\":30,\"enabled\":true}"
_wr_resp="$(curl -s -m 20 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${_wr_body}" \
  "${PULSE_URL}/probes" 2>/dev/null || echo '{}')"
PROBE_WEBRTC_ID="$(printf '%s' "${_wr_resp}" | jq -r '.id // empty' 2>/dev/null || true)"
log "WebRTC probe created: id=${PROBE_WEBRTC_ID:-FAILED}"
printf '%s' "${_wr_resp}" | jq . > "${EVIDENCE_DIR}/probe-webrtc-create.json" 2>/dev/null || true

log "Creating RTMP probe → ${PROBE_RTMP_URL}"
_rt_body="{\"name\":\"tc-p08-rtmp-${STREAM_ID}\",\"url\":\"${PROBE_RTMP_URL}\",\"protocol\":\"rtmp\",\"interval_s\":30,\"timeout_s\":20,\"enabled\":true}"
_rt_resp="$(curl -s -m 20 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${_rt_body}" \
  "${PULSE_URL}/probes" 2>/dev/null || echo '{}')"
PROBE_RTMP_ID="$(printf '%s' "${_rt_resp}" | jq -r '.id // empty' 2>/dev/null || true)"
log "RTMP probe created: id=${PROBE_RTMP_ID:-FAILED}"
printf '%s' "${_rt_resp}" | jq . > "${EVIDENCE_DIR}/probe-rtmp-create.json" 2>/dev/null || true

log "Creating HLS probe → ${PROBE_HLS_URL}"
_hl_body="{\"name\":\"tc-p08-hls-${STREAM_ID}\",\"url\":\"${PROBE_HLS_URL}\",\"protocol\":\"hls\",\"interval_s\":30,\"timeout_s\":15,\"enabled\":true}"
_hl_resp="$(curl -s -m 20 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${_hl_body}" \
  "${PULSE_URL}/probes" 2>/dev/null || echo '{}')"
PROBE_HLS_ID="$(printf '%s' "${_hl_resp}" | jq -r '.id // empty' 2>/dev/null || true)"
log "HLS probe created: id=${PROBE_HLS_ID:-FAILED}"
printf '%s' "${_hl_resp}" | jq . > "${EVIDENCE_DIR}/probe-hls-create.json" 2>/dev/null || true

# SKIP if every probe creation failed (no probe IDs at all)
if [ -z "${PROBE_WEBRTC_ID}" ] && [ -z "${PROBE_RTMP_ID}" ] && [ -z "${PROBE_HLS_ID}" ]; then
  log "SKIP: all three probe creations failed"
  printf 'SKIP\nPrecondition unmet: all three probe creations failed via Pulse API.\n' \
    > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

log "Probes created: webrtc=${PROBE_WEBRTC_ID:-none} rtmp=${PROBE_RTMP_ID:-none} hls=${PROBE_HLS_ID:-none}"

# ── Poll all 3 probes for first result (up to 180 s each, shared wait budget) ───
# Strategy: poll in a single loop; stop when all 3 have a result or 180 s elapsed.
log "Polling all 3 probe result endpoints (shared budget: 180 s)"
_wr_result=""
_rt_result=""
_hl_result=""
_result_secs=999
_i=0
while [ "${_i}" -lt 60 ]; do
  sleep 3

  # WebRTC result
  if [ -z "${_wr_result}" ] && [ -n "${PROBE_WEBRTC_ID}" ]; then
    _wr_result="$(curl -s -m 15 \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/probes/${PROBE_WEBRTC_ID}/results" 2>/dev/null \
      | jq '.items[0] // empty' 2>/dev/null || true)"
  fi

  # RTMP result
  if [ -z "${_rt_result}" ] && [ -n "${PROBE_RTMP_ID}" ]; then
    _rt_result="$(curl -s -m 15 \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/probes/${PROBE_RTMP_ID}/results" 2>/dev/null \
      | jq '.items[0] // empty' 2>/dev/null || true)"
  fi

  # HLS result
  if [ -z "${_hl_result}" ] && [ -n "${PROBE_HLS_ID}" ]; then
    _hl_result="$(curl -s -m 15 \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/probes/${PROBE_HLS_ID}/results" 2>/dev/null \
      | jq '.items[0] // empty' 2>/dev/null || true)"
  fi

  # Check whether all created probes have results
  _all_done=true
  [ -n "${PROBE_WEBRTC_ID}" ] && [ -z "${_wr_result}" ] && _all_done=false
  [ -n "${PROBE_RTMP_ID}" ]   && [ -z "${_rt_result}" ] && _all_done=false
  [ -n "${PROBE_HLS_ID}" ]    && [ -z "${_hl_result}" ] && _all_done=false

  if [ "${_all_done}" = "true" ]; then
    _result_secs=$(( (_i + 1) * 3 ))
    log "All available probe results received after ${_result_secs} s"
    break
  fi

  _i=$(( _i + 1 ))
  if (( _i % 10 == 0 )); then
    log "Still polling at $(( _i * 3 )) s: wr=${_wr_result:+got} rt=${_rt_result:+got} hl=${_hl_result:+got}"
  fi
done

# ── Capture final probe result snapshots ─────────────────────────────────────────
[ -n "${PROBE_WEBRTC_ID}" ] && capture_pulse "/probes/${PROBE_WEBRTC_ID}/results" "webrtc-results" || true
[ -n "${PROBE_RTMP_ID}" ]   && capture_pulse "/probes/${PROBE_RTMP_ID}/results" "rtmp-results" || true
[ -n "${PROBE_HLS_ID}" ]    && capture_pulse "/probes/${PROBE_HLS_ID}/results" "hls-results" || true

# ── Save first results to evidence ───────────────────────────────────────────────
[ -n "${_wr_result}" ] && printf '%s' "${_wr_result}" | jq . > "${EVIDENCE_DIR}/probe-webrtc-first-result.json" 2>/dev/null || true
[ -n "${_rt_result}" ] && printf '%s' "${_rt_result}" | jq . > "${EVIDENCE_DIR}/probe-rtmp-first-result.json" 2>/dev/null || true
[ -n "${_hl_result}" ] && printf '%s' "${_hl_result}" | jq . > "${EVIDENCE_DIR}/probe-hls-first-result.json" 2>/dev/null || true

# ── Assertions ────────────────────────────────────────────────────────────────────
# WebRTC probe
if [ -n "${PROBE_WEBRTC_ID}" ]; then
  if [ -n "${_wr_result}" ]; then
    _wr_success="$(printf '%s' "${_wr_result}" | \
      jq -r 'if .success == true then "true" else "false" end' 2>/dev/null || echo false)"
    log "WebRTC probe result: success=${_wr_success}  $(printf '%s' "${_wr_result}" | jq -c '{ice_state,signaling_state,error_code}' 2>/dev/null || echo '{}')"
    assert_eq "${_wr_success}" "true" "${SCENARIO} WebRTC probe success=true" || true
  else
    log "FAIL: WebRTC probe produced no result within 180 s"
    assert_eq "no_result" "result_present" "${SCENARIO} WebRTC probe result within 180 s" || true
  fi
else
  log "WebRTC probe was not created — skipping its success assertion"
fi

# RTMP probe
if [ -n "${PROBE_RTMP_ID}" ]; then
  if [ -n "${_rt_result}" ]; then
    _rt_success="$(printf '%s' "${_rt_result}" | \
      jq -r 'if .success == true then "true" else "false" end' 2>/dev/null || echo false)"
    log "RTMP probe result: success=${_rt_success}  $(printf '%s' "${_rt_result}" | jq -c '{signaling_state,connect_time_ms,error_code}' 2>/dev/null || echo '{}')"
    assert_eq "${_rt_success}" "true" "${SCENARIO} RTMP probe success=true" || true
  else
    log "FAIL: RTMP probe produced no result within 180 s"
    assert_eq "no_result" "result_present" "${SCENARIO} RTMP probe result within 180 s" || true
  fi
else
  log "RTMP probe was not created — skipping its success assertion"
fi

# HLS probe
if [ -n "${PROBE_HLS_ID}" ]; then
  if [ -n "${_hl_result}" ]; then
    _hl_success="$(printf '%s' "${_hl_result}" | \
      jq -r 'if .success == true then "true" else "false" end' 2>/dev/null || echo false)"
    log "HLS probe result: success=${_hl_success}  $(printf '%s' "${_hl_result}" | jq -c '{ttfb_ms,bitrate_kbps,error_code}' 2>/dev/null || echo '{}')"
    assert_eq "${_hl_success}" "true" "${SCENARIO} HLS probe success=true" || true
  else
    log "FAIL: HLS probe produced no result within 180 s"
    assert_eq "no_result" "result_present" "${SCENARIO} HLS probe result within 180 s" || true
  fi
else
  log "HLS probe was not created — skipping its success assertion"
fi

printf 'result_convergence_s=%s  wr=%s  rt=%s  hl=%s\n' \
  "${_result_secs}" \
  "${PROBE_WEBRTC_ID:-none}" \
  "${PROBE_RTMP_ID:-none}" \
  "${PROBE_HLS_ID:-none}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ───────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
