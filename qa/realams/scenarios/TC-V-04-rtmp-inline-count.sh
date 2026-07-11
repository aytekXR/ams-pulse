#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-04-rtmp-inline-count.sh
#
# TC-V-04: RTMP viewer count — inline poll path (AV-16)
#
# Assertion matrix row:
#   Purpose:          Sample the LIVE inline rtmpViewerCount field from AMS BroadcastDTO
#                     (the actual poll path Pulse uses, NOT the dead BroadcastStatistics
#                     endpoint). Confirm never negative. Confirm Pulse vc_rtmp == 0.
#   AMS endpoint:     GET /LiveApp/rest/v2/broadcasts/{id}  (app-scope, no auth needed)
#   Pulse assertion:  GET /api/v1/live/streams → protocol_mix.rtmp == 0 for val-v04 stream
#   AV-16 evidence:   Record every sampled value in timeline.txt
#   Tolerance:        rtmpViewerCount should be 0 (no RTMP pull viewers); never -1.
#                     If ANY sample is negative → file AV-16 bug (normalize.go sums without clamping).
#   Exit:             0 PASS | 1 FAIL | 77 SKIP (publisher never broadcasting)
#
set -euo pipefail

SCENARIO="TC-V-04"
echo "=== ${SCENARIO}: RTMP viewer count — inline poll path (AV-16) ===" >&2

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
STREAM_ID="val-v04-${EPOCH}"
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

# ── 1. Start publisher ──────────────────────────────────────────────────────────
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
    echo "Cannot sample inline rtmpViewerCount without a live stream."
  } > "${EVIDENCE_DIR}/verdict.txt"
  cat "${EVIDENCE_DIR}/verdict.txt" >&2
  exit 77
fi

# ── 2. Sampling loop: 10 samples ~5 s apart for val-v04 AND teststream ──────────
# AV-16 evidence: record every sampled value in timeline.txt
# App-scope GET — no auth cookie required (remoteAllowedCIDR open)
log "=== AV-16 RTMP inline count sampling begins (10 samples, 5 s interval) ==="
log "stream=${STREAM_ID}  teststream=teststream"

# Track negative occurrences for AV-16 bug detection
_val_negatives=0
_test_negatives=0

_sample=0
while [ "${_sample}" -lt 10 ]; do
  _sample=$(( _sample + 1 ))

  # val-v04 stream
  _val_dto="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}")"
  _val_rtmp="$(printf '%s' "${_val_dto}" | jq '.rtmpViewerCount // 0' 2>/dev/null || echo 0)"

  # teststream (constant publisher — always live)
  _test_dto="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/teststream")"
  _test_rtmp="$(printf '%s' "${_test_dto}" | jq '.rtmpViewerCount // 0' 2>/dev/null || echo 0)"

  log "sample ${_sample}/10: val-v04.rtmpViewerCount=${_val_rtmp}  teststream.rtmpViewerCount=${_test_rtmp}"

  # Record each sample in checks; any negative is an AV-16 trigger
  # || true: prevent set -e from exiting the sampling loop on any single failed check
  assert_gte "${_val_rtmp}" 0 "sample_${_sample}: val-v04 rtmpViewerCount >= 0" || true
  assert_gte "${_test_rtmp}" 0 "sample_${_sample}: teststream rtmpViewerCount >= 0" || true

  if [ "${_val_rtmp}" -lt 0 ] 2>/dev/null; then
    _val_negatives=$(( _val_negatives + 1 ))
    log "AV-16 TRIGGER: val-v04 rtmpViewerCount=${_val_rtmp} at sample ${_sample}"
  fi
  if [ "${_test_rtmp}" -lt 0 ] 2>/dev/null; then
    _test_negatives=$(( _test_negatives + 1 ))
    log "AV-16 TRIGGER: teststream rtmpViewerCount=${_test_rtmp} at sample ${_sample}"
  fi

  if [ "${_sample}" -lt 10 ]; then
    sleep 5
  fi
done

log "=== Sampling complete: val_negatives=${_val_negatives}  test_negatives=${_test_negatives} ==="

if [ "${_val_negatives}" -gt 0 ] || [ "${_test_negatives}" -gt 0 ]; then
  log "AV-16: Negative rtmpViewerCount detected — normalize.go sums without clamping at this path"
  log "AV-16: File bug doc in docs/assessment/bugs/ per evidence rules"
fi

# ── 3. Capture final AMS + Pulse snapshots ──────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "final"
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "final"
log "Final snapshots captured"

# ── 4. Pulse assertion: vc_rtmp == 0 ────────────────────────────────────────────
log "Waiting 15 s for Pulse poll-convergence window before final assertion"
sleep 15

_pulse_resp="$(curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp")"

_pulse_rtmp="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    ( .items // .streams // [] )[]
    | select(.stream_id == $id)
    | ( .protocol_mix.rtmp // .vc_rtmp // 0 )
  ' | head -1)"
_pulse_rtmp="${_pulse_rtmp:-0}"

log "Pulse protocol_mix.rtmp=${_pulse_rtmp} for ${STREAM_ID}"

# Assert Pulse shows 0 RTMP pull viewers (no RTMP pull clients connected)
# || true: prevent set -e from exiting before scenario_verdict aggregates all results
assert_eq "${_pulse_rtmp}" "0" "Pulse vc_rtmp (protocol_mix.rtmp) == 0 for ${STREAM_ID}" || true

# Also assert viewer_count is not corrupted by a negative rtmp value
_pulse_viewers="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    ( .items // .streams // [] )[]
    | select(.stream_id == $id)
    | ( .viewers // .viewer_count // 0 )
  ' | head -1)"
_pulse_viewers="${_pulse_viewers:-0}"

assert_gte "${_pulse_viewers}" 0 "Pulse viewers >= 0 (not corrupted by negative rtmp) for ${STREAM_ID}" || true

log "Final: Pulse viewers=${_pulse_viewers}  Pulse rtmp=${_pulse_rtmp}"

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
