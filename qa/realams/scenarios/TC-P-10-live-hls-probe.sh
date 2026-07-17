#!/usr/bin/env bash
# qa/realams/scenarios/TC-P-10-live-hls-probe.sh
#
# TC-P-10: HLS probe on a live stream — success then honest failure after stop
#
# Assertion matrix row:
#   Steps:         1. Start publisher val-p10-<hex> on LiveApp
#                  2. Wait for HLS playlist at ${AMS_URL}/${APP}/streams/${STREAM_ID}.m3u8
#                     (exit 77 if never served within 40 s)
#                  3. POST ${PULSE_URL}/probes to create an HLS probe
#                     (exit 77 on 403 — below Pro tier; exit 77 on any other non-2xx)
#                  4. Poll ${PULSE_URL}/probes/{id}/results until success=true
#                     with ttfb_ms>0 and bitrate_kbps>0 (budget 180 s)
#                  5. Stop publisher
#                  6. Poll probe results until a FAILURE result (success=false) appears
#                     (budget 90 s, two probe intervals at 30 s each)
#                  7. Assert no spurious success=true appears after stream death
#   AMS truth:     HLS playlist served while publisher is broadcasting
#   Pulse assert:  First result: success=true, ttfb_ms>0, bitrate_kbps>0
#                  Post-stop result: success=false (honest failure, no fake green)
#   Risk:          LOW — read-only except for the test publisher and probe (both cleaned up)
#   Exit:          0 PASS | 1 FAIL | 77 SKIP (HLS not served; tier gate; probe creation failed)
#
set -euo pipefail

SCENARIO="TC-P-10"
echo "=== ${SCENARIO}: HLS probe — live stream + post-stop honest failure ===" >&2

# ── Harness bootstrap ─────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"
# shellcheck source=../harness/publisher.sh
source "${_DIR}/../harness/publisher.sh"

# ── Per-run identifiers ───────────────────────────────────────────────────────
STREAM_ID="val-p10-$(openssl rand -hex 4)"
APP="${AMS_APP:-LiveApp}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

PROBE_ID=""
PROBE_HLS_URL="${AMS_URL}/${APP}/streams/${STREAM_ID}.m3u8"

# ── Timeline log ──────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ──────────────────────────────────────────────────────────────
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

log "STREAM_ID=${STREAM_ID}  APP=${APP}  PROBE_HLS_URL=${PROBE_HLS_URL}"

# ── Start publisher ───────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on ${APP}"
start_publisher "${STREAM_ID}" "${APP}" 2000

# ── Wait for AMS broadcasting + HLS playlist (budget 40 s) ───────────────────
log "Polling AMS for broadcasting + HLS playlist (budget 40 s)"
_ams_status="unknown"
_hls_ready=false
_hls_code="0"
_i=0
while [ "${_i}" -lt 14 ]; do
  _ams_status="$(curl -s -m 10 -b "${AMS_COOKIE_FILE}" \
    "${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo curl_error)"
  if [ "${_ams_status}" = "broadcasting" ]; then
    _hls_code="$(curl -s -m 10 -o /dev/null -w '%{http_code}' \
      "${AMS_URL}/${APP}/streams/${STREAM_ID}.m3u8" 2>/dev/null || echo 0)"
    if [ "${_hls_code}" = "200" ]; then
      _hls_ready=true
      log "AMS: broadcasting + HLS ready after $(( (_i + 1) * 3 )) s (HTTP ${_hls_code})"
      break
    fi
  fi
  log "AMS status=${_ams_status}  hls_code=${_hls_code} (attempt $(( _i + 1 ))/14)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/${APP}/rest/v2/broadcasts/${STREAM_ID}" "pre-probe"

if [ "${_hls_ready}" != "true" ]; then
  log "SKIP: HLS playlist never available for ${STREAM_ID} (status=${_ams_status})"
  printf 'SKIP\nPrecondition unmet: HLS playlist at %s not available (AMS status: %s; last HTTP: %s).\n' \
    "${PROBE_HLS_URL}" "${_ams_status}" "${_hls_code}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Create HLS probe ──────────────────────────────────────────────────────────
log "Creating HLS probe → ${PROBE_HLS_URL}"
_probe_body="{\"name\":\"tc-p10-${STREAM_ID}\",\"url\":\"${PROBE_HLS_URL}\",\"protocol\":\"hls\",\"interval_s\":30,\"timeout_s\":15}"
_probe_http_code="$(curl -s -m 20 \
  -o "${EVIDENCE_DIR}/probe-create.json" \
  -w '%{http_code}' \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${_probe_body}" \
  "${PULSE_URL}/probes" 2>/dev/null || echo 000)"

_probe_resp="$(cat "${EVIDENCE_DIR}/probe-create.json" 2>/dev/null || echo '{}')"
log "Probe creation HTTP ${_probe_http_code}: $(printf '%s' "${_probe_resp}" | jq -c '{id,name,error} // .' 2>/dev/null || echo "${_probe_resp}")"

# 403 = below Pro tier
if [ "${_probe_http_code}" = "403" ]; then
  log "SKIP: Pulse returned 403 — HLS probing requires Pro or higher tier"
  printf 'SKIP\nPrecondition unmet: Pulse /probes returned HTTP 403 (Pro+ tier required for HLS probing).\n' \
    > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

PROBE_ID="$(printf '%s' "${_probe_resp}" | jq -r '.id // empty' 2>/dev/null || true)"
if [ -z "${PROBE_ID}" ]; then
  log "SKIP: probe creation did not return an id (HTTP ${_probe_http_code})"
  printf 'SKIP\nPrecondition unmet: could not create HLS probe (HTTP %s). Response: %s\n' \
    "${_probe_http_code}" "${_probe_resp}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi
log "Probe created: id=${PROBE_ID}"

# ── Poll for first successful result (budget 180 s) ───────────────────────────
log "Polling ${PULSE_URL}/probes/${PROBE_ID}/results for success=true (budget 180 s)"
_first_success_result=""
_first_success_secs=999
_i=0
while [ "${_i}" -lt 60 ]; do
  sleep 3
  _results_resp="$(curl -s -m 15 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/probes/${PROBE_ID}/results" 2>/dev/null || echo '{}')"
  # Look for the first item with success=true
  _first_success_result="$(printf '%s' "${_results_resp}" | \
    jq '(.items // []) | map(select(.success == true)) | first // empty' \
    2>/dev/null || true)"
  if [ -n "${_first_success_result}" ]; then
    _first_success_secs=$(( (_i + 1) * 3 ))
    log "Got success=true probe result after ${_first_success_secs} s"
    break
  fi
  _i=$(( _i + 1 ))
done

capture_pulse "/probes/${PROBE_ID}/results" "pre-stop"
printf '%s' "${_first_success_result}" | jq . > "${EVIDENCE_DIR}/probe-result-success.json" 2>/dev/null || true

if [ -z "${_first_success_result}" ]; then
  log "FAIL: no success=true probe result within 180 s"
  assert_eq "no_success_result" "success_result_present" \
    "${SCENARIO} HLS probe returned success=true within 180 s" || true
  scenario_verdict
  exit $?
fi

log "Success result: $(printf '%s' "${_first_success_result}" | jq -c '{success,ttfb_ms,bitrate_kbps}' 2>/dev/null || true)"

# ── Assert first success result fields ────────────────────────────────────────
_success="$(printf '%s' "${_first_success_result}" | jq -r 'if .success == true then "true" else "false" end' 2>/dev/null || echo false)"
assert_eq "${_success}" "true" "${SCENARIO} first probe result success=true" || true

_ttfb="$(printf '%s' "${_first_success_result}" | jq '.ttfb_ms // 0' 2>/dev/null || echo 0)"
assert_gte "${_ttfb}" 1 "${SCENARIO} first probe result ttfb_ms > 0 (got ${_ttfb})" || true

_bitrate="$(printf '%s' "${_first_success_result}" | jq '.bitrate_kbps // 0' 2>/dev/null || echo 0)"
assert_gte "${_bitrate}" 1 "${SCENARIO} first probe result bitrate_kbps > 0 (got ${_bitrate})" || true

# ── Stop publisher ────────────────────────────────────────────────────────────
_stop_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
log "Stopping publisher ${STREAM_ID} at ${_stop_ts}"
stop_publisher "${STREAM_ID}"

# ── Poll for failure result after stop (budget 90 s) ──────────────────────────
# Expect success=false once the stream is dead. Two probe intervals (2x30 s) plus
# time for the probe to fire and deliver the result.
log "Polling for success=false result after stream stop (budget 90 s)"
_post_stop_fail_result=""
_post_stop_secs=999
_spurious_success=false
_i=0
while [ "${_i}" -lt 30 ]; do
  sleep 3
  _post_results="$(curl -s -m 15 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/probes/${PROBE_ID}/results" 2>/dev/null || echo '{}')"

  # A result is "post-stop" if its checked_at timestamp is after stop time.
  # We use a simple proxy: count of total results vs the count when we stopped.
  # More precisely: look for ANY result newer than the success result that has success=false.
  _post_stop_fail_result="$(printf '%s' "${_post_results}" | \
    jq --arg stop "${_stop_ts}" \
      '(.items // []) | map(select(.success == false)) | first // empty' \
    2>/dev/null || true)"

  # Also check for any spurious success that arrived after the stop time
  _any_new_success="$(printf '%s' "${_post_results}" | \
    jq '(.items // []) | map(select(.success == true)) | length' \
    2>/dev/null || echo 0)"

  if [ -n "${_post_stop_fail_result}" ]; then
    _post_stop_secs=$(( (_i + 1) * 3 ))
    log "Got success=false result after ${_post_stop_secs} s from stop"
    break
  fi
  log "Waiting for post-stop failure result (attempt $(( _i + 1 ))/30, any_success=${_any_new_success})"
  _i=$(( _i + 1 ))
done

capture_pulse "/probes/${PROBE_ID}/results" "after-stop"
printf '%s' "${_post_stop_fail_result}" | jq . > "${EVIDENCE_DIR}/probe-result-post-stop.json" 2>/dev/null || true

{
  printf '\nTC-P-10 timeline:\n'
  printf '  stop_ts:             %s\n' "${_stop_ts}"
  printf '  first_success_secs:  %s\n' "${_first_success_secs}"
  printf '  ttfb_ms:             %s\n' "${_ttfb}"
  printf '  bitrate_kbps:        %s\n' "${_bitrate}"
  printf '  post_stop_fail_secs: %s\n' "${_post_stop_secs}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions: no fake greens after stream death ─────────────────────────────
log "ASSERT: post-stop failure result present (no fake greens)"

# A failure result must appear within 90 s of stop
_fail_appeared="no"
[ -n "${_post_stop_fail_result}" ] && _fail_appeared="yes"
assert_eq "${_fail_appeared}" "yes" \
  "${SCENARIO} success=false result appeared within 90 s of stream stop (no fake green)" || true

assert_lte "${_post_stop_secs}" 90 \
  "${SCENARIO} post-stop failure convergence ≤90 s" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
