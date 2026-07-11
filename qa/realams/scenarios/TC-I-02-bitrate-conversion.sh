#!/usr/bin/env bash
# qa/realams/scenarios/TC-I-02-bitrate-conversion.sh
#
# TC-I-02: Bitrate conversion check (AMS bits/sec → Pulse kbps)
#
# Assertion matrix row:
#   Design note:      This scenario validates the exact bits/sec → kbps conversion
#                     in normalize.go:79. Run alongside / after TC-I-01; uses its
#                     own publisher so it can be run independently.
#   AMS ground truth: GET /LiveApp/rest/v2/broadcasts/{id} → bitrate (bits/sec, e.g. 2048000)
#   Pulse assertion:  GET /api/v1/qoe/ingest → bitrate_kbps == ams_bitrate / 1000
#                     (within +-5% since ffmpeg output is approximate)
#   Numeric precision: AMS reports bits/sec; Pulse divides by 1000 (normalize.go:79).
#                      Assert exact transformation within floating-point tolerance.
#   Tolerance:         +-5% on conversion (tighter than TC-I-01's +-10% AMS range).
#   Exit:              0 PASS | 1 FAIL | 77 SKIP (publisher never broadcasting)
#
set -euo pipefail

SCENARIO="TC-I-02"
echo "=== ${SCENARIO}: Bitrate conversion check (bits/sec → kbps) ===" >&2

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
STREAM_ID="val-i02-${EPOCH}"
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
  log "Publisher never reached broadcasting status in 30 s — SKIP"
  {
    echo "SKIP"
    echo "Publisher ${STREAM_ID} never reached AMS status=broadcasting in 30 s."
    echo "Cannot assert bitrate conversion without an active ingest."
  } > "${EVIDENCE_DIR}/verdict.txt"
  cat "${EVIDENCE_DIR}/verdict.txt" >&2
  exit 77
fi

# ── 2. Wait 15 s for bitrate to stabilise in AMS ───────────────────────────────
log "Waiting 15 s for AMS bitrate to stabilise"
sleep 15

# ── 3. Capture AMS snapshot ─────────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "bitrate-stable"
_ams_broadcast="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}")"
_ams_bitrate="$(printf '%s' "${_ams_broadcast}" | jq '.bitrate // 0' 2>/dev/null || echo 0)"
_ams_bitrate_kbps="$(awk -v b="${_ams_bitrate}" 'BEGIN { printf "%.3f", b / 1000 }')"
log "AMS bitrate=${_ams_bitrate} bits/sec  expected_pulse_kbps=${_ams_bitrate_kbps}"

# ── 4. Wait for Pulse ClickHouse flush ─────────────────────────────────────────
log "Waiting 5 s for Pulse ClickHouse insert + MV propagation"
sleep 5

# ── 5. Capture Pulse /qoe/ingest ────────────────────────────────────────────────
_NOW_S="$(date +%s)"
_FROM_MS=$(( (_NOW_S - 600) * 1000 ))
_TO_MS=$(( _NOW_S * 1000 ))

capture_pulse "/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" "bitrate-conversion"

_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}")"

# Extract the most-recent bitrate_kbps bucket from IngestHealthResponse
_pulse_bitrate_kbps="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].bitrate_kbps else null end
  ' | head -1)"

if [ -z "${_pulse_bitrate_kbps}" ] || [ "${_pulse_bitrate_kbps}" = "null" ]; then
  log "Pulse returned no timeseries data yet for ${STREAM_ID} — retrying after 10 s"
  sleep 10
  _pulse_resp="$(curl -s -m 15 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}")"
  _pulse_bitrate_kbps="$(printf '%s' "${_pulse_resp}" | \
    jq --arg id "${STREAM_ID}" '
      .streams[]?
      | select(.stream_id == $id)
      | .timeseries
      | if length > 0 then .[-1].bitrate_kbps else 0 end
    ' | head -1)"
  _pulse_bitrate_kbps="${_pulse_bitrate_kbps:-0}"
fi

log "AMS bitrate_raw=${_ams_bitrate} bits/sec  AMS_kbps=${_ams_bitrate_kbps}  Pulse bitrate_kbps=${_pulse_bitrate_kbps}"

# ── 6. Assertions ───────────────────────────────────────────────────────────────
# || true after each assert: prevent set -e from exiting before scenario_verdict
# can aggregate all check results into verdict.txt.
#
# Core conversion check: Pulse bitrate_kbps == AMS bitrate / 1000 (within +-5%)
# The tolerance exists because:
#   - ffmpeg output bitrate is approximate (target vs actual)
#   - AMS may measure bitrate slightly differently across poll cycles
# A systematic factor-of-1000 error (e.g., Pulse returning bits/sec instead of
# kbps, or kbps instead of bps) would show up as a >100% deviation — easy to catch.
assert_approx "${_pulse_bitrate_kbps}" "${_ams_bitrate_kbps}" 5 \
  "Pulse bitrate_kbps (${_pulse_bitrate_kbps}) == AMS bitrate / 1000 (${_ams_bitrate_kbps}) within 5%" || true

# Sanity: both sides should be in a realistic kbps range for a 2000 kbps publisher
assert_gte "${_ams_bitrate_kbps}" 500 \
  "AMS bitrate_kbps >= 500 (publisher producing data; 2000 kbps target)" || true
assert_gte "${_pulse_bitrate_kbps}" 500 \
  "Pulse bitrate_kbps >= 500 (not returned as raw bits/sec or zero)" || true

# Check the inverse: pulse_kbps * 1000 must be approximately ams_bitrate
_pulse_bits_equiv="$(awk -v k="${_pulse_bitrate_kbps}" 'BEGIN { printf "%.0f", k * 1000 }')"
log "Reverse check: pulse_kbps * 1000 = ${_pulse_bits_equiv} bits/sec  AMS bitrate = ${_ams_bitrate} bits/sec"
assert_approx "${_pulse_bits_equiv}" "${_ams_bitrate}" 5 \
  "Pulse bitrate_kbps * 1000 (${_pulse_bits_equiv}) ≈ AMS bitrate (${_ams_bitrate}) within 5%" || true

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
