#!/usr/bin/env bash
# qa/realams/scenarios/TC-P-07-probe-interval.sh
#
# TC-P-07: Probe interval and scheduling
#
# Assertion matrix row:
#   Steps:         1. Start publisher val-p07-<epoch> on LiveApp
#                  2. Create HLS probe at flat /<app>/streams/<id>.m3u8
#                     with minimum legal interval (30 s per OpenAPI minimum: 30)
#                  3. Wait ~5 intervals (180 s) for results to accumulate
#                  4. GET /probes/{id}/results → assert row count in [3, 7]
#                  5. Parse ts fields (Unix epoch ms); assert inter-result
#                     spacing consistent (each gap in [10 000 ms, 90 000 ms])
#                  6. Delete probe in cleanup trap
#   AMS truth:     HLS playlist served at LiveApp/streams/<id>.m3u8
#   Pulse assert:  result rows accumulate at ~30 s intervals; no missing intervals
#   Exit:          0 PASS | 1 FAIL | 77 SKIP (publisher or probe creation failed)
#
set -euo pipefail

SCENARIO="TC-P-07"
echo "=== ${SCENARIO}: Probe interval and scheduling ===" >&2

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
STREAM_ID="val-p07-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

PROBE_ID=""
# HLS URL: flat form per S17 correction — /<app>/streams/<id>.m3u8
PROBE_HLS_URL="${AMS_URL}/LiveApp/streams/${STREAM_ID}.m3u8"
# Minimum legal interval per OpenAPI schema (interval_s minimum: 30)
PROBE_INTERVAL_S=30

# ── Timeline log ─────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────────
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

log "STREAM_ID=${STREAM_ID}  probe_interval_s=${PROBE_INTERVAL_S}  probe_url=${PROBE_HLS_URL}"

# ── Start publisher ──────────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

# ── Wait for AMS broadcasting + HLS playlist available (≤40 s) ──────────────────
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
      "${AMS_URL}/LiveApp/streams/${STREAM_ID}.m3u8" 2>/dev/null || echo 0)"
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

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-probe"

if [ "${_hls_ready}" != "true" ]; then
  log "SKIP: HLS playlist not available for ${STREAM_ID} (AMS status: ${_ams_status})"
  printf 'SKIP\nPrecondition unmet: HLS playlist not available for %s (AMS status: %s).\n' \
    "${STREAM_ID}" "${_ams_status}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Create HLS probe with minimum interval ───────────────────────────────────────
log "Creating HLS probe → ${PROBE_HLS_URL} (interval_s=${PROBE_INTERVAL_S})"
_probe_body="{\"name\":\"tc-p07-${STREAM_ID}\",\"url\":\"${PROBE_HLS_URL}\",\"protocol\":\"hls\",\"interval_s\":${PROBE_INTERVAL_S},\"timeout_s\":15,\"enabled\":true}"
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

log "Probe created: id=${PROBE_ID}  interval_s=${PROBE_INTERVAL_S}"
printf '%s' "${_probe_resp}" | jq . > "${EVIDENCE_DIR}/probe-create.json" 2>/dev/null || true

# ── Wait ~5 probe intervals (180 s) for results to accumulate ────────────────────
_wait_s=$(( PROBE_INTERVAL_S * 6 ))
log "Waiting ${_wait_s} s (~6 × ${PROBE_INTERVAL_S} s intervals) for results"
sleep "${_wait_s}"

# ── Fetch all probe results ───────────────────────────────────────────────────────
log "Fetching probe results from /probes/${PROBE_ID}/results"
_results_resp="$(curl -s -m 20 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/probes/${PROBE_ID}/results" 2>/dev/null || echo '{}')"

printf '%s' "${_results_resp}" | jq . > "${EVIDENCE_DIR}/probe-results-all.json" 2>/dev/null || true
capture_pulse "/probes/${PROBE_ID}/results" "interval-check"

_result_count="$(printf '%s' "${_results_resp}" | \
  jq '(.items // []) | length' 2>/dev/null || echo 0)"
log "Probe result count: ${_result_count} (expect 3-7 for ${_wait_s} s wait at ${PROBE_INTERVAL_S} s interval)"

# ── Assert result count in expected band [3, 7] ──────────────────────────────────
assert_gte "${_result_count}" 3 \
  "${SCENARIO} result count >= 3 (accumulated over ${_wait_s} s)" || true
assert_lte "${_result_count}" 7 \
  "${SCENARIO} result count <= 7 (not more than ~6 intervals)" || true

# ── Parse ts fields and assert inter-result spacing is consistent ─────────────────
# Extract ts values (Unix epoch ms) in ascending order.
# Assert each consecutive gap is in [10 000 ms, 90 000 ms]
# (30 s interval ± generous tolerance for scheduling variance).
log "Checking inter-result spacing (expect each gap 10 000–90 000 ms for ${PROBE_INTERVAL_S} s interval)"

_ts_json="$(printf '%s' "${_results_resp}" | \
  jq '[(.items // []) | sort_by(.ts) | .[].ts]' 2>/dev/null || echo '[]')"
printf '%s\n' "${_ts_json}" > "${EVIDENCE_DIR}/probe-ts-values.json"

_spacing_verdict="PASS"
_spacing_details=""
_gap_count=0
_bad_gap_count=0
_dup_gap_count=0
_prev_ts=""

# Read ts values into shell; jq emits one number per line.
# Near-duplicate detection: the Pulse probe scheduler can produce two result
# rows within ≤ 1 ms of each other at periodic intervals (BUG-003).  Gaps
# < 1000 ms are treated as scheduler duplicates — recorded, NOT counted as
# bad spacing.  Only gaps outside [10 000 ms, 90 000 ms] that are NOT
# near-duplicates count toward _bad_gap_count.
while IFS= read -r _ts_val; do
  [ -z "${_ts_val}" ] && continue
  if [ -n "${_prev_ts}" ]; then
    _gap="$(awk -v a="${_ts_val}" -v b="${_prev_ts}" 'BEGIN { print (a - b) }')"
    _gap_count=$(( _gap_count + 1 ))
    # Near-duplicate: gap < 1000 ms  (BUG-003 scheduler artifact)
    _is_dup="$(awk -v g="${_gap}" 'BEGIN { print (g < 1000) ? "yes" : "no" }')"
    if [ "${_is_dup}" = "yes" ]; then
      _dup_gap_count=$(( _dup_gap_count + 1 ))
      _spacing_details="${_spacing_details} gap${_gap_count}=${_gap}ms(dup/BUG-003)"
      log "Gap ${_gap_count}: ${_gap} ms [prev=${_prev_ts} cur=${_ts_val}] → near-duplicate (BUG-003, excluded from band check)"
    else
      _ok="$(awk -v g="${_gap}" 'BEGIN { print (g >= 10000 && g <= 90000) ? "ok" : "bad" }')"
      _spacing_details="${_spacing_details} gap${_gap_count}=${_gap}ms(${_ok})"
      if [ "${_ok}" = "bad" ]; then
        _bad_gap_count=$(( _bad_gap_count + 1 ))
        _spacing_verdict="FAIL"
      fi
      log "Gap ${_gap_count}: ${_gap} ms [prev=${_prev_ts} cur=${_ts_val}] → ${_ok}"
    fi
  fi
  _prev_ts="${_ts_val}"
done < <(printf '%s' "${_ts_json}" | jq '.[]' 2>/dev/null || true)

log "Spacing summary: gaps=${_gap_count} bad_gaps=${_bad_gap_count} dup_gaps=${_dup_gap_count} verdict=${_spacing_verdict}"
log "Details:${_spacing_details}"
if [ "${_dup_gap_count}" -gt 0 ]; then
  log "NOTE: ${_dup_gap_count} near-duplicate result(s) detected (gap < 1000 ms) — see BUG-003 (probe scheduler duplicate results)"
fi
printf 'result_count=%s  gap_count=%s  bad_gaps=%s  dup_gaps=%s  spacing=%s\n' \
  "${_result_count}" "${_gap_count}" "${_bad_gap_count}" "${_dup_gap_count}" "${_spacing_details}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

_real_gap_count=$(( _gap_count - _dup_gap_count ))
if [ "${_real_gap_count}" -gt 0 ]; then
  # Assert only on real inter-result gaps (near-duplicate gaps excluded per BUG-003).
  assert_eq "${_spacing_verdict}" "PASS" \
    "${SCENARIO} inter-result spacing consistent (${_bad_gap_count}/${_real_gap_count} real gaps out of band; ${_dup_gap_count} dup excluded — BUG-003)" || true
else
  # Only one (or all-duplicate) results — cannot compute real gaps
  log "Only ${_result_count} result(s) with ${_dup_gap_count} dup gap(s) — not enough real gaps to band-check (count check above is authoritative)"
fi

# ── Verify at least one result has success=true ───────────────────────────────────
_success_count="$(printf '%s' "${_results_resp}" | \
  jq '[(.items // [])[] | select(.success == true)] | length' 2>/dev/null || echo 0)"
log "Results with success=true: ${_success_count}"
assert_gte "${_success_count}" 1 \
  "${SCENARIO} at least 1 probe result with success=true" || true

# ── Verdict ───────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
