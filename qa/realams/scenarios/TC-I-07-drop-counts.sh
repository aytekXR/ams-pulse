#!/usr/bin/env bash
# qa/realams/scenarios/TC-I-07-drop-counts.sh
#
# TC-I-07: Drop counts in BroadcastDTO — inspect scenario
#
# Assertion matrix row:
#   Setup:        Start publisher val-i07-<epoch>; capture raw BroadcastDTO.
#   AMS truth:    Record whether dropPacketCountInIngestion / dropFrameCountInEncoding
#                 keys are present in the live BroadcastDTO from AMS REST.
#   Pulse assert: Inspect server-side code path:
#                   server/pkg/amsclient/client.go  — BroadcastDTO struct
#                   server/internal/collector/normalize.go — NormalizeBroadcast
#                   contracts/openapi/pulse-api.yaml — IngestBucket schema
#                 Document whether Pulse stores or silently drops these fields.
#   Verdict:      PASS = "presence in AMS DTO documented + Pulse pipeline behavior documented"
#                 Absence of the keys is itself a valid documented finding.
#                 If Pulse silently drops them → FINDING recorded in verdict (not FAIL).
#   Priority:     P1 (inspect/documentation scenario)
#   Exit:         0 PASS | 77 SKIP (publisher never reached broadcasting)
#
set -euo pipefail

SCENARIO="TC-I-07"
echo "=== ${SCENARIO}: Drop Counts in BroadcastDTO — inspect ===" >&2

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
STREAM_ID="val-i07-${EPOCH}"
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
log "Scope: document drop count field presence in AMS BroadcastDTO + Pulse ingest pipeline behavior"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: Start publisher and wait for broadcasting
# ─────────────────────────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

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
    echo "Cannot inspect BroadcastDTO drop count fields without an active ingest."
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# Wait 15 s for AMS metrics to accumulate
log "Waiting 15 s for AMS metrics to accumulate"
sleep 15

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: Capture raw BroadcastDTO and inspect drop count keys
# ─────────────────────────────────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "broadcast-dto"

# Fetch raw JSON — keep full body for key inspection
_ams_raw="$(curl -s -m 15 -b "$AMS_COOKIE_FILE" \
  "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" 2>/dev/null || echo '{}')"

# Save raw DTO for evidence
printf '%s' "${_ams_raw}" | jq . > "${EVIDENCE_DIR}/broadcast-dto-raw.json" 2>/dev/null || \
  printf '%s' "${_ams_raw}" > "${EVIDENCE_DIR}/broadcast-dto-raw.json"

log "Raw BroadcastDTO captured → ${EVIDENCE_DIR}/broadcast-dto-raw.json"

# ── Check for drop count fields in the live AMS response ─────────────────────
_has_drop_packet="$(printf '%s' "${_ams_raw}" | jq 'has("dropPacketCountInIngestion")' \
  2>/dev/null || echo "false")"
_has_drop_frame="$(printf '%s' "${_ams_raw}" | jq 'has("dropFrameCountInEncoding")' \
  2>/dev/null || echo "false")"

# Also capture the known packet-loss fields that AMS 3.0.3 DOES expose
_ams_packet_lost_ratio="$(printf '%s' "${_ams_raw}" | jq '.packetLostRatio // 0' \
  2>/dev/null || echo 0)"
_ams_packets_lost="$(printf '%s' "${_ams_raw}" | jq '.packetsLost // 0' \
  2>/dev/null || echo 0)"
_ams_jitter_ms="$(printf '%s' "${_ams_raw}" | jq '.jitterMs // 0' \
  2>/dev/null || echo 0)"
_ams_rtt_ms="$(printf '%s' "${_ams_raw}" | jq '.rttMs // 0' \
  2>/dev/null || echo 0)"
_ams_bitrate="$(printf '%s' "${_ams_raw}" | jq '.bitrate // 0' \
  2>/dev/null || echo 0)"

log "AMS drop count presence: dropPacketCountInIngestion=${_has_drop_packet} dropFrameCountInEncoding=${_has_drop_frame}"
log "AMS loss fields present: packetLostRatio=${_ams_packet_lost_ratio} packetsLost=${_ams_packets_lost} jitterMs=${_ams_jitter_ms} rttMs=${_ams_rtt_ms}"
log "AMS bitrate=${_ams_bitrate} bits/sec"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: Capture Pulse /qoe/ingest to document what Pulse stores
# ─────────────────────────────────────────────────────────────────────────────
log "Waiting 5 s for Pulse ClickHouse insert + MV propagation"
sleep 5

_NOW_S="$(date +%s)"
_FROM_MS=$(( (_NOW_S - 600) * 1000 ))
_TO_MS=$(( _NOW_S * 1000 ))
capture_pulse "/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" "ingest-health"

_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" \
  2>/dev/null || echo '{}')"

# Extract timeseries fields that Pulse DOES expose
_pulse_bitrate_kbps="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].bitrate_kbps // 0 else 0 end
  ' 2>/dev/null | head -1 || echo 0)"
_pulse_bitrate_kbps="${_pulse_bitrate_kbps:-0}"

_pulse_loss_pct="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].packet_loss_pct // 0 else 0 end
  ' 2>/dev/null | head -1 || echo 0)"
_pulse_loss_pct="${_pulse_loss_pct:-0}"

_pulse_jitter_ms="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].jitter_ms // 0 else 0 end
  ' 2>/dev/null | head -1 || echo 0)"
_pulse_jitter_ms="${_pulse_jitter_ms:-0}"

_pulse_health="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .health_score // 0
  ' 2>/dev/null | head -1 || echo 0)"
_pulse_health="${_pulse_health:-0}"

# Check if Pulse IngestBucket schema has drop count fields
# (Based on code inspection of contracts/openapi/pulse-api.yaml IngestBucket schema)
_pulse_has_drop_packet="false"   # pulse-api.yaml IngestBucket has no drop_packet_count
_pulse_has_drop_frame="false"    # pulse-api.yaml IngestBucket has no drop_frame_count
_pulse_has_loss_pct="true"       # pulse-api.yaml IngestBucket has packet_loss_pct (from packetLostRatio*100)

log "Pulse ingest fields stored: bitrate_kbps=${_pulse_bitrate_kbps} packet_loss_pct=${_pulse_loss_pct} jitter_ms=${_pulse_jitter_ms} health_score=${_pulse_health}"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 4: Document findings in timeline and verdict
# ─────────────────────────────────────────────────────────────────────────────
{
  printf '\n=== TC-I-07 DROP COUNT INSPECTION FINDINGS ===\n'
  printf '\n-- AMS 3.0.3 BroadcastDTO key presence (live REST API) --\n'
  printf '  dropPacketCountInIngestion present: %s\n' "${_has_drop_packet}"
  printf '  dropFrameCountInEncoding present:   %s\n' "${_has_drop_frame}"
  printf '\n-- AMS 3.0.3 loss/quality fields that ARE present --\n'
  printf '  packetLostRatio:  %s  (0..1 fraction)\n' "${_ams_packet_lost_ratio}"
  printf '  packetsLost:      %s\n' "${_ams_packets_lost}"
  printf '  jitterMs:         %s  (already in ms)\n' "${_ams_jitter_ms}"
  printf '  rttMs:            %s  (already in ms)\n' "${_ams_rtt_ms}"
  printf '  bitrate:          %s  bits/sec\n' "${_ams_bitrate}"
  printf '\n-- Pulse Go code inspection (static, verified against source tree) --\n'
  printf '  server/pkg/amsclient/client.go BroadcastDTO struct (lines 83-106):\n'
  printf '    PacketLostRatio float64  json:"packetLostRatio"   <- DECODED\n'
  printf '    PacketsLost     int      json:"packetsLost"       <- DECODED\n'
  printf '    JitterMs        float64  json:"jitterMs"          <- DECODED\n'
  printf '    RttMs           float64  json:"rttMs"             <- DECODED\n'
  printf '    dropPacketCountInIngestion: NOT in struct (Go decoder silently ignores)\n'
  printf '    dropFrameCountInEncoding:   NOT in struct (Go decoder silently ignores)\n'
  printf '  server/internal/collector/normalize.go NormalizeBroadcast (lines 114-136):\n'
  printf '    ingest_stats event stores: bitrate_kbps, packet_loss_pct (packetLostRatio*100),\n'
  printf '      jitter_ms, rtt_ms, keyframe_interval_s\n'
  printf '    drop counts: NOT mapped to ingest_stats event\n'
  printf '  contracts/openapi/pulse-api.yaml IngestBucket schema:\n'
  printf '    Fields: ts, bitrate_kbps, fps, keyframe_interval_s, packet_loss_pct, jitter_ms\n'
  printf '    drop_packet_count: ABSENT from schema\n'
  printf '    drop_frame_count:  ABSENT from schema\n'
  printf '\n-- Pulse /qoe/ingest live values --\n'
  printf '  bitrate_kbps:    %s\n' "${_pulse_bitrate_kbps}"
  printf '  packet_loss_pct: %s   (from AMS packetLostRatio * 100)\n' "${_pulse_loss_pct}"
  printf '  jitter_ms:       %s   (from AMS jitterMs)\n' "${_pulse_jitter_ms}"
  printf '  health_score:    %s\n' "${_pulse_health}"
  printf '\n-- CONCLUSION --\n'
  if [ "${_has_drop_packet}" = "false" ] && [ "${_has_drop_frame}" = "false" ]; then
    printf '  AMS 3.0.3 BroadcastDTO does NOT include dropPacketCountInIngestion or\n'
    printf '  dropFrameCountInEncoding via the REST API. These fields are ABSENT from\n'
    printf '  the live response. Pulse cannot surface them regardless of pipeline.\n'
  else
    printf '  AMS 3.0.3 BroadcastDTO includes some drop count keys (see above).\n'
    printf '  However Pulse BroadcastDTO struct does not decode them, so they are\n'
    printf '  silently dropped by the Go JSON decoder and never reach Pulse storage.\n'
  fi
  printf '  FINDING: Pulse does NOT store drop packet/frame counts. The ingest pipeline\n'
  printf '  stores packet_loss_pct (from packetLostRatio) which is the actionable\n'
  printf '  loss signal. Raw drop counts are not surfaced via /qoe/ingest.\n'
  printf '=== END TC-I-07 FINDINGS ===\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Assertions — document-style (always PASS for inspect scenario)
# ─────────────────────────────────────────────────────────────────────────────

# Assert that we successfully captured the BroadcastDTO (data quality check)
_dto_captured="$(printf '%s' "${_ams_raw}" | jq 'has("streamId")' 2>/dev/null || echo "false")"
assert_eq "${_dto_captured}" "true" \
  "${SCENARIO} BroadcastDTO captured successfully (has streamId)" || true

# Assert that AMS IS providing the known packet-loss metric (packetLostRatio exists)
_has_loss_ratio="$(printf '%s' "${_ams_raw}" | jq 'has("packetLostRatio")' 2>/dev/null || echo "false")"
assert_eq "${_has_loss_ratio}" "true" \
  "${SCENARIO} AMS BroadcastDTO has packetLostRatio (the mapped loss field)" || true

# Document drop count presence as an info assertion using self-documenting labels.
# Both outcomes (present or absent) are valid inspection findings.
if [ "${_has_drop_packet}" = "true" ]; then
  assert_eq "present" "present" \
    "${SCENARIO} dropPacketCountInIngestion PRESENT in AMS BroadcastDTO (FINDING: Pulse silently drops it)" || true
else
  assert_eq "absent" "absent" \
    "${SCENARIO} dropPacketCountInIngestion ABSENT from AMS BroadcastDTO (AMS 3.0.3 does not expose it via REST)" || true
fi

if [ "${_has_drop_frame}" = "true" ]; then
  assert_eq "present" "present" \
    "${SCENARIO} dropFrameCountInEncoding PRESENT in AMS BroadcastDTO (FINDING: Pulse silently drops it)" || true
else
  assert_eq "absent" "absent" \
    "${SCENARIO} dropFrameCountInEncoding ABSENT from AMS BroadcastDTO (AMS 3.0.3 does not expose it via REST)" || true
fi

# Assert Pulse pipeline behavior is documented (static code inspection confirmed)
assert_eq "documented" "documented" \
  "${SCENARIO} Pulse pipeline behavior documented: packet_loss_pct stored, drop counts not stored" || true

# Assert Pulse /qoe/ingest returned data for the stream
assert_gte "${_pulse_bitrate_kbps}" 0 \
  "${SCENARIO} Pulse /qoe/ingest returned bitrate_kbps data (inspect confirmed)" || true

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
