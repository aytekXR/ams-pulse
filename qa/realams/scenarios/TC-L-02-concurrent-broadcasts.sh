#!/usr/bin/env bash
# qa/realams/scenarios/TC-L-02-concurrent-broadcasts.sh
#
# TC-L-02: Multiple concurrent broadcasts (5 publishers)
#
# Assertion matrix row:
#   Steps:         1. Record baseline total_publishers in Pulse
#                  2. Start 5 publishers simultaneously on LiveApp
#                  3. Wait 15 s for convergence
#                  4. Assert all 5 stream IDs broadcasting in AMS
#                  5. Assert all 5 stream IDs present in Pulse /live/streams
#                  6. Assert Pulse total_publishers delta >= 5 (before/after)
#                  7. Stop all 5 publishers
#   AMS truth:     GET /LiveApp/rest/v2/broadcasts/list/0/100 → 5 new streams broadcasting
#   Pulse assert:  all 5 stream IDs in GET /api/v1/live/streams with publisher_state=publishing;
#                  GET /api/v1/live/overview total_publishers >= baseline + 5
#   Note:          No compare_viewer_count (0 viewers); presence + status only.
#                  Never assert global counts directly — use before/after delta.
#   Exit:          0 PASS | 1 FAIL | 77 SKIP
#
set -euo pipefail

SCENARIO="TC-L-02"
echo "=== ${SCENARIO}: Multiple Concurrent Broadcasts (5 streams) ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"
# shellcheck source=../harness/publisher.sh
source "${_DIR}/../harness/publisher.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────────
EPOCH="$(date +%s)"
PREFIX="val-l02-${EPOCH}"
COUNT=5

# Derive stream IDs matching start_bulk_publishers printf format: <PREFIX>NNNN
_stream_id() { printf '%s%04d' "${PREFIX}" "$1"; }

EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap: stop all publishers ────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping all ${COUNT} publishers (prefix=${PREFIX})"
  local _ci
  for _ci in $(seq 1 "${COUNT}"); do
    stop_publisher "$(_stream_id "${_ci}")" 2>/dev/null || true
  done
}
trap cleanup EXIT

log "PREFIX=${PREFIX}  COUNT=${COUNT}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Baseline: record current total_publishers (teststream + others may be active) ──
log "Capturing baseline total_publishers from Pulse /live/overview"
_baseline_resp="$(curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/overview" 2>/dev/null || true)"
BASELINE_PUB="$(printf '%s' "${_baseline_resp}" | jq '.total_publishers // 0' 2>/dev/null || echo 0)"
log "Baseline total_publishers=${BASELINE_PUB}"
capture_pulse "/live/overview" "pre-baseline"
capture_ams "/LiveApp/rest/v2/broadcasts/list/0/100" "pre-baseline"

# ── Start 5 publishers simultaneously ────────────────────────────────────────────
log "Starting ${COUNT} publishers with prefix=${PREFIX} at 500 kbps each"
start_bulk_publishers "${COUNT}" "LiveApp" "${PREFIX}" 500
log "All ${COUNT} publishers dispatched"

# ── Wait 15 s for AMS + Pulse convergence ────────────────────────────────────────
log "Waiting 15 s for convergence"
sleep 15

# ── Capture post-start state ──────────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/list/0/100" "after-start"
capture_pulse "/live/streams" "after-start"
capture_pulse "/live/overview" "after-start"

# ── Fetch AMS broadcast list and Pulse streams list once for all per-stream checks ─
_ams_list="$(curl -s -m 15 \
  "${AMS_URL}/LiveApp/rest/v2/broadcasts/list/0/100" \
  2>/dev/null || true)"
_pulse_streams="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams" \
  2>/dev/null || true)"

# ── Per-stream assertions: AMS broadcasting ───────────────────────────────────────
log "Asserting all ${COUNT} streams broadcasting in AMS"
for _si in $(seq 1 "${COUNT}"); do
  _sid="$(_stream_id "${_si}")"
  _ams_st="$(printf '%s' "${_ams_list}" | \
    jq -r --arg id "${_sid}" \
      '.[] | select(.streamId == $id) | .status' \
    2>/dev/null | head -1 || true)"
  log "AMS stream=${_sid} status=${_ams_st}"
  assert_eq "${_ams_st}" "broadcasting" "${SCENARIO} AMS ${_sid}=broadcasting" || true
done

# ── Per-stream assertions: Pulse publisher_state=publishing ──────────────────────
log "Asserting all ${COUNT} stream IDs present in Pulse /live/streams"
for _si in $(seq 1 "${COUNT}"); do
  _sid="$(_stream_id "${_si}")"
  _pulse_st="$(printf '%s' "${_pulse_streams}" | \
    jq -r --arg id "${_sid}" \
      '(.items // [])[] | select(.stream_id == $id) | .publisher_state' \
    2>/dev/null | head -1 || true)"
  log "Pulse stream=${_sid} publisher_state=${_pulse_st}"
  assert_eq "${_pulse_st}" "publishing" "${SCENARIO} Pulse ${_sid}=publishing" || true
done

# ── total_publishers delta assertion (before/after, not absolute) ─────────────────
log "Asserting Pulse total_publishers delta >= ${COUNT}"
_after_pub="$(curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/overview" \
  | jq '.total_publishers // 0' 2>/dev/null || echo 0)"
_expected_min=$(( BASELINE_PUB + COUNT ))
log "total_publishers: baseline=${BASELINE_PUB} after=${_after_pub} expected_min=${_expected_min}"
assert_gte "${_after_pub}" "${_expected_min}" \
  "${SCENARIO} Pulse total_publishers >= baseline(${BASELINE_PUB})+${COUNT}" || true

# ── Verdict ───────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
