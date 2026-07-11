#!/usr/bin/env bash
# qa/realams/scenarios/TC-I-06-fps-always-zero.sh
#
# TC-I-06: FPS always 0 (AMS 3.0.3)
#
# Assertion matrix row:
#   Finding:          AMS 3.0.3 REST API omits currentFPS from BroadcastDTO
#                     (see client.go:97 comment — field not present in wire format).
#   AMS ground truth: GET /LiveApp/rest/v2/broadcasts/{id}
#                       → jq 'has("currentFPS")' == false
#   Pulse assertion:  GET /api/v1/qoe/ingest?stream=&app=
#                       → timeseries[-1].fps == 0
#                       → health_score > 80 (FPS weight redistributed, not penalised)
#   Purpose:          Confirm Pulse does not falsely penalise health_score when
#                     AMS omits fps — the weight must be redistributed to other
#                     metrics, not treated as a fps=0 degradation.
#   Exit:             0 PASS | 1 FAIL | 77 SKIP (publisher never broadcasting)
#
set -euo pipefail

SCENARIO="TC-I-06"
echo "=== ${SCENARIO}: FPS always 0 — AMS 3.0.3 BroadcastDTO omits currentFPS ===" >&2

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
STREAM_ID="val-i06-${EPOCH}"
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
    echo "Cannot inspect BroadcastDTO currentFPS without an active stream."
  } > "${EVIDENCE_DIR}/verdict.txt"
  cat "${EVIDENCE_DIR}/verdict.txt" >&2
  exit 77
fi

# ── 2. Wait 15 s for metrics to appear in AMS and Pulse ─────────────────────────
log "Waiting 15 s for ingest metrics to appear (PRD F4: visible within 15 s)"
sleep 15

# ── 3. Capture AMS BroadcastDTO ─────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "broadcast-dto"
_ams_broadcast="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}")"

# Save raw for evidence
printf '%s\n' "${_ams_broadcast}" > "${EVIDENCE_DIR}/ams-broadcast-dto-raw.json"
log "AMS BroadcastDTO captured"

# Check what fps-related keys ARE present in the DTO (for documentation)
_ams_keys="$(printf '%s' "${_ams_broadcast}" | jq '[keys[] | select(test("fps|Fps|FPS"; "i"))]' 2>/dev/null || echo "")"
log "AMS BroadcastDTO fps-related keys: ${_ams_keys}"

# ── 4. Assert AMS BroadcastDTO has no currentFPS key ───────────────────────────
# AMS 3.0.3 omits currentFPS from the REST payload (client.go:97 comment).
# jq 'has("currentFPS")' returns "true" or "false".
_has_fps="$(printf '%s' "${_ams_broadcast}" | jq 'has("currentFPS")' 2>/dev/null || echo "false")"
log "AMS BroadcastDTO has(\"currentFPS\")=${_has_fps}"

assert_eq "${_has_fps}" "false" \
  "AMS BroadcastDTO omits currentFPS key (AMS 3.0.3 — client.go:97)" || true

# ── 5. Capture Pulse /qoe/ingest ────────────────────────────────────────────────
# Wait 5 s more for ClickHouse flush
log "Waiting 5 s for Pulse ClickHouse insert + MV propagation"
sleep 5

_NOW_S="$(date +%s)"
_FROM_MS=$(( (_NOW_S - 600) * 1000 ))
_TO_MS=$(( _NOW_S * 1000 ))

capture_pulse "/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" "fps-check"

_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}")"

# Extract fps and health_score from IngestHealthResponse
_pulse_health="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .health_score // 0
  ' | head -1)"
_pulse_health="${_pulse_health:-0}"

_pulse_fps="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].fps else 0 end
  ' | head -1)"
_pulse_fps="${_pulse_fps:-0}"

# Also extract bitrate to confirm stream is genuinely active (not zero data)
_pulse_bitrate_kbps="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].bitrate_kbps else 0 end
  ' | head -1)"
_pulse_bitrate_kbps="${_pulse_bitrate_kbps:-0}"

log "Pulse: fps=${_pulse_fps}  health_score=${_pulse_health}  bitrate_kbps=${_pulse_bitrate_kbps}"

# ── 6. Assertions ───────────────────────────────────────────────────────────────
# || true after each assert: prevent set -e from exiting before scenario_verdict
# can aggregate all check results into verdict.txt.

# Pulse fps must be 0 — AMS 3.0.3 never reports currentFPS in REST
assert_eq "${_pulse_fps}" "0" \
  "Pulse ingest fps == 0 (AMS 3.0.3 omits currentFPS; normalize.go stores 0)" || true

# Health score must still be > 80 — FPS weight redistributed, not penalised
# A healthy 2 Mbps stream with no packet loss should score well even with fps=0.
assert_gte "${_pulse_health}" 80 \
  "Pulse health_score (${_pulse_health}) > 80 — FPS weight redistributed when AMS omits currentFPS" || true

# Sanity: confirm the stream is actually delivering data (bitrate > 0)
# This ensures the health_score is for a live stream, not a ghost entry.
assert_gte "${_pulse_bitrate_kbps}" 1 \
  "Pulse bitrate_kbps (${_pulse_bitrate_kbps}) > 0 — confirms active ingest in timeseries" || true

log "TC-I-06 evidence: ams_has_fps=${_has_fps}  pulse_fps=${_pulse_fps}  health=${_pulse_health}  bitrate=${_pulse_bitrate_kbps} kbps"

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
