#!/usr/bin/env bash
# qa/realams/scenarios/TC-I-01-normal-ingest.sh
#
# TC-I-01: Normal ingest metrics
#
# Assertion matrix row:
#   Setup:            2 Mbps publisher on LiveApp/val-i01-<epoch>
#   AMS ground truth: GET /LiveApp/rest/v2/broadcasts/{id} → bitrate ≈ 2000000 bits/sec
#   Pulse assertion:  GET /api/v1/qoe/ingest?stream=&app= →
#                       bitrate_kbps ≈ 2000 (within +-10%)
#                       health_score > 80
#   Numeric precision: AMS bitrate in bits/sec; Pulse divides by 1000 (normalize.go:79).
#                      Assert Pulse bitrate_kbps * 1000 == AMS bitrate within +-5% (ffmpeg approx).
#   Tolerance:         +-10% on bitrate (ffmpeg output approximate); health_score > 80.
#   Exit:              0 PASS | 1 FAIL | 77 SKIP (publisher never broadcasting)
#
set -euo pipefail

SCENARIO="TC-I-01"
echo "=== ${SCENARIO}: Normal ingest metrics ===" >&2

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
STREAM_ID="val-i01-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping publisher ${STREAM_ID}"
  stop_publisher "${STREAM_ID}" || true
}
trap cleanup EXIT

# ── 1. Start publisher at 2000 kbps ────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

# Wait for AMS to show broadcasting (up to 30 s)
log "Waiting for AMS broadcasting status (budget: 30 s)"
_broadcasting=0
_i=0
while [ "${_i}" -lt 15 ]; do
  _st="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"')"
  if [ "${_st}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( _i * 2 )) s"
    _broadcasting=1
    break
  fi
  sleep 2
  _i=$(( _i + 1 ))
done

if [ "${_broadcasting}" -eq 0 ]; then
  log "Publisher never reached broadcasting status in 30 s — SKIP"
  {
    echo "SKIP"
    echo "Publisher ${STREAM_ID} never reached AMS status=broadcasting in 30 s."
    echo "Cannot assert ingest metrics without an active ingest."
  } > "${EVIDENCE_DIR}/verdict.txt"
  cat "${EVIDENCE_DIR}/verdict.txt" >&2
  exit 77
fi

# ── 2. Wait 15 s for AMS bitrate to stabilise ───────────────────────────────────
log "Waiting 15 s for AMS bitrate to stabilise (PRD F4: metrics visible within 15 s)"
sleep 15

# ── 3. Capture AMS snapshot ─────────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "ingest-stable"
_ams_broadcast="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}")"
_ams_bitrate="$(printf '%s' "${_ams_broadcast}" | jq '.bitrate // 0')"
log "AMS bitrate=${_ams_bitrate} bits/sec"

# ── 4. Wait 5 more seconds for Pulse ClickHouse flush (2-5 s insert lag) ────────
log "Waiting 5 s additional for Pulse ClickHouse insert + MV propagation"
sleep 5

# ── 5. Capture Pulse /qoe/ingest ────────────────────────────────────────────────
# Use a 10-minute look-back window to capture the current session's ingest data
_NOW_S="$(date +%s)"
_FROM_MS=$(( (_NOW_S - 600) * 1000 ))
_TO_MS=$(( _NOW_S * 1000 ))

capture_pulse "/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" "ingest-health"

_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}")"

# Extract from IngestHealthResponse: streams[].{health_score, timeseries[-1].bitrate_kbps}
_pulse_health="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .health_score // 0
  ' | head -1)"
_pulse_health="${_pulse_health:-0}"

_pulse_bitrate_kbps="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].bitrate_kbps else 0 end
  ' | head -1)"
_pulse_bitrate_kbps="${_pulse_bitrate_kbps:-0}"

# Convert AMS bits/sec to kbps for comparison
_ams_bitrate_kbps="$(awk -v b="${_ams_bitrate}" 'BEGIN { printf "%.1f", b / 1000 }')"

log "AMS bitrate=${_ams_bitrate} bits/sec  (${_ams_bitrate_kbps} kbps)"
log "Pulse bitrate_kbps=${_pulse_bitrate_kbps}  health_score=${_pulse_health}"

# ── 6. Assertions ───────────────────────────────────────────────────────────────
# || true after each assert: prevent set -e from exiting before scenario_verdict
# can aggregate all check results into verdict.txt.

# AMS bitrate should be approximately 2000 kbps (ffmpeg at 2000k)
# Accept wide range since we're checking the Pulse assertion more precisely
assert_gte "${_ams_bitrate}" 100000 \
  "AMS bitrate >= 100000 bits/sec (publisher is producing data)" || true

# Pulse bitrate_kbps ≈ AMS bitrate/1000 within +-10% (normalize.go:79)
assert_approx "${_pulse_bitrate_kbps}" "${_ams_bitrate_kbps}" 10 \
  "Pulse bitrate_kbps (${_pulse_bitrate_kbps}) ≈ AMS bitrate/1000 (${_ams_bitrate_kbps}) within 10%" || true

# Pulse bitrate_kbps should be in the ~2000 kbps range (publisher at 2000k)
assert_approx "${_pulse_bitrate_kbps}" 2000 10 \
  "Pulse bitrate_kbps ≈ 2000 within 10% (publisher at 2000 kbps)" || true

# Health score must be > 80 for a healthy 2 Mbps stream
assert_gte "${_pulse_health}" 80 \
  "Pulse health_score (${_pulse_health}) > 80 for healthy ingest" || true

log "Ingest metrics: ams_kbps=${_ams_bitrate_kbps}  pulse_kbps=${_pulse_bitrate_kbps}  health=${_pulse_health}"

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
