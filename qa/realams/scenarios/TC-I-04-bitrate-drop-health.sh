#!/usr/bin/env bash
# qa/realams/scenarios/TC-I-04-bitrate-drop-health.sh
#
# TC-I-04: Bitrate drop — health score degradation
#
# Assertion matrix row:
#   Setup:         Start publisher val-i04-<epoch> at 2000 kbps on LiveApp.
#                  Capture Pulse /qoe/ingest health_score baseline (expect >80).
#                  Stop; re-publish SAME stream id at 200 kbps; wait for ingest rows.
#   AMS truth:     bitrate drops from ~2000000 to ~200000 bits/sec
#   Pulse assert:  bitrate_kbps ≈ 200 (±20 pct of 200)
#                  health_score dropped ≥30 points from baseline
#   NOTE:          Do NOT assert exact score=50 — target-bitrate config on the realams
#                  stack may differ from the default 2000 kbps target; record the
#                  observed pair (baseline, degraded).
#   Tolerance:     ±20 pct on bitrate_kbps; absolute drop ≥30 on health_score.
#   Exit:          0 PASS | 1 FAIL | 77 SKIP (publisher never reached broadcasting)
#
set -euo pipefail

SCENARIO="TC-I-04"
echo "=== ${SCENARIO}: Bitrate Drop — Health Score Degradation ===" >&2

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
STREAM_ID="val-i04-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping publisher ${STREAM_ID}"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: Publish at 2000 kbps — establish baseline health score
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 1: starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

# Poll AMS for broadcasting (up to 30 s, 2 s interval)
log "Polling AMS for status=broadcasting (budget: 30 s)"
_broadcasting=0
_i=0
while [ "${_i}" -lt 15 ]; do
  _st="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")"
  if [ "${_st}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( _i * 2 )) s"
    _broadcasting=1
    break
  fi
  sleep 2
  _i=$(( _i + 1 ))
done

if [ "${_broadcasting}" -eq 0 ]; then
  log "SKIP: publisher never reached broadcasting (precondition unmet)"
  {
    echo "SKIP"
    echo "Publisher ${STREAM_ID} never reached AMS status=broadcasting in 30 s."
    echo "Cannot assert health score without an active ingest baseline."
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# Wait 15 s for AMS bitrate to stabilise + Pulse ingest flush
log "Waiting 15 s for AMS bitrate to stabilise"
sleep 15
log "Waiting 5 s for Pulse ClickHouse insert and MV propagation"
sleep 5

# Capture baseline from AMS
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "baseline-ams"
_ams_baseline_raw="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
  2>/dev/null || echo '{}')"
_ams_bitrate_baseline="$(printf '%s' "${_ams_baseline_raw}" | jq '.bitrate // 0' 2>/dev/null || echo 0)"
log "AMS baseline bitrate=${_ams_bitrate_baseline} bits/sec"

# Capture baseline from Pulse /qoe/ingest
_NOW_S="$(date +%s)"
_FROM_MS=$(( (_NOW_S - 600) * 1000 ))
_TO_MS=$(( _NOW_S * 1000 ))
capture_pulse "/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" "baseline-pulse"

_pulse_baseline_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" \
  2>/dev/null || echo '{}')"

_baseline_health="$(printf '%s' "${_pulse_baseline_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .health_score // 0
  ' 2>/dev/null | head -1 || echo 0)"
_baseline_health="${_baseline_health:-0}"

_baseline_bitrate_kbps="$(printf '%s' "${_pulse_baseline_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].bitrate_kbps else 0 end
  ' 2>/dev/null | head -1 || echo 0)"
_baseline_bitrate_kbps="${_baseline_bitrate_kbps:-0}"

log "Baseline: Pulse health_score=${_baseline_health}  bitrate_kbps=${_baseline_bitrate_kbps}"
{
  printf 'Phase 1 baseline: health_score=%s  bitrate_kbps=%s\n' "${_baseline_health}" "${_baseline_bitrate_kbps}"
  printf 'AMS baseline bitrate=%s bits/sec\n' "${_ams_bitrate_baseline}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# Baseline assertion: health should be > 80 at 2000 kbps
assert_gte "${_baseline_health}" 80 \
  "${SCENARIO} baseline health_score (${_baseline_health}) > 80 at 2000 kbps" || true

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: Stop and re-publish at 200 kbps — degrade ingest quality
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 2: stopping publisher (graceful)"
stop_publisher "${STREAM_ID}"

# Brief pause for AMS to register the stop
log "Waiting 5 s for AMS to register stop"
sleep 5

log "Re-publishing ${STREAM_ID} at 200 kbps (10x bitrate reduction)"
start_publisher "${STREAM_ID}" "LiveApp" 200

# Poll AMS for broadcasting again (up to 30 s)
log "Polling AMS for status=broadcasting at degraded bitrate (budget: 30 s)"
_broadcasting2=0
_i=0
while [ "${_i}" -lt 15 ]; do
  _st2="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")"
  if [ "${_st2}" = "broadcasting" ]; then
    log "AMS status=broadcasting (degraded) after $(( _i * 2 )) s"
    _broadcasting2=1
    break
  fi
  sleep 2
  _i=$(( _i + 1 ))
done

if [ "${_broadcasting2}" -eq 0 ]; then
  log "FAIL: re-publisher never reached broadcasting at 200 kbps"
  assert_eq "not_broadcasting" "broadcasting" \
    "${SCENARIO} re-publisher reached broadcasting at 200 kbps" || true
  scenario_verdict
  exit $?
fi

# Wait 20 s for AMS metrics to stabilise + Pulse flush
log "Waiting 20 s for AMS 200 kbps metrics to stabilise"
sleep 20

# Capture AMS at degraded state
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "degraded-ams"
_ams_degraded_raw="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
  2>/dev/null || echo '{}')"
_ams_bitrate_degraded="$(printf '%s' "${_ams_degraded_raw}" | jq '.bitrate // 0' 2>/dev/null || echo 0)"
log "AMS degraded bitrate=${_ams_bitrate_degraded} bits/sec"

# Wait extra 5 s for Pulse ClickHouse flush
log "Waiting 5 s for Pulse ClickHouse insert and MV propagation"
sleep 5

# Capture Pulse at degraded state
_NOW_S2="$(date +%s)"
_FROM_MS2=$(( (_NOW_S2 - 600) * 1000 ))
_TO_MS2=$(( _NOW_S2 * 1000 ))
capture_pulse "/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS2}&to=${_TO_MS2}" "degraded-pulse"

_pulse_degraded_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS2}&to=${_TO_MS2}" \
  2>/dev/null || echo '{}')"

_degraded_health="$(printf '%s' "${_pulse_degraded_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .health_score // 0
  ' 2>/dev/null | head -1 || echo 0)"
_degraded_health="${_degraded_health:-0}"

_degraded_bitrate_kbps="$(printf '%s' "${_pulse_degraded_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].bitrate_kbps else 0 end
  ' 2>/dev/null | head -1 || echo 0)"
_degraded_bitrate_kbps="${_degraded_bitrate_kbps:-0}"

log "Degraded: Pulse health_score=${_degraded_health}  bitrate_kbps=${_degraded_bitrate_kbps}"
{
  printf 'Phase 2 degraded: health_score=%s  bitrate_kbps=%s\n' "${_degraded_health}" "${_degraded_bitrate_kbps}"
  printf 'AMS degraded bitrate=%s bits/sec\n' "${_ams_bitrate_degraded}"
  printf 'Health score drop: %s -> %s\n' "${_baseline_health}" "${_degraded_health}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Assertions
# ─────────────────────────────────────────────────────────────────────────────

# AMS bitrate at degraded state should be approximately 200000 bits/sec
assert_gte "${_ams_bitrate_degraded}" 50000 \
  "${SCENARIO} AMS degraded bitrate >= 50000 bits/sec (publisher is sending data)" || true

# Pulse bitrate_kbps ≈ 200 (±20 pct of 200 = ±40)
assert_approx "${_degraded_bitrate_kbps}" 200 20 \
  "${SCENARIO} Pulse degraded bitrate_kbps (${_degraded_bitrate_kbps}) ≈ 200 (±20 pct)" || true

# Health score must have dropped ≥30 points from baseline
# Compute drop = baseline - degraded (awk for numeric safety)
_health_drop="$(awk -v b="${_baseline_health}" -v d="${_degraded_health}" \
  'BEGIN { printf "%.1f", b - d }' 2>/dev/null || echo 0)"
log "Health score drop: ${_health_drop} points (${_baseline_health} → ${_degraded_health})"

assert_gte "${_health_drop}" 30 \
  "${SCENARIO} health_score dropped ${_health_drop} pts (≥30 required; ${_baseline_health}→${_degraded_health})" || true

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
