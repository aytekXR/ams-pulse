#!/usr/bin/env bash
# qa/realams/scenarios/TC-L-05-simultaneous-start-stop.sh
#
# TC-L-05: Simultaneous start and stop
#
# Assertion matrix row:
#   Steps:         1. Start 10 publishers: val-l05a-0001..0005 (a-group) +
#                                          val-l05b-0001..0005 (b-group)
#                  2. After 5 s: stop a-group while simultaneously starting
#                                val-l05c-0001..0005 (c-group)
#                  3. Wait 15 s for convergence
#                  4. Assert a-group absent from Pulse
#                  5. Assert b-group + c-group present in Pulse (publisher_state=publishing)
#                  6. Per-stream checks only — never global-exact counts
#   AMS truth:     a-group transitions to terminal; b+c active
#   Pulse assert:  a-gone, b+c present, 10 of our streams visible within tolerance
#   Exit:          0 PASS | 1 FAIL | 77 SKIP
#
set -euo pipefail

SCENARIO="TC-L-05"
echo "=== ${SCENARIO}: Simultaneous start/stop transition ===" >&2

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
GROUP_SIZE=5

# ID generators matching start_bulk_publishers printf format (PREFIX + 4-digit seq)
_aid() { printf 'val-l05a-%04d' "$1"; }
_bid() { printf 'val-l05b-%04d' "$1"; }
_cid() { printf 'val-l05c-%04d' "$1"; }

EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap: stop all three groups (idempotent) ────────────────────────────
cleanup() {
  local _gi
  log "CLEANUP: stopping all three groups (a, b, c)"
  for _gi in $(seq 1 "${GROUP_SIZE}"); do
    stop_publisher "$(_aid "${_gi}")" 2>/dev/null || true
    stop_publisher "$(_bid "${_gi}")" 2>/dev/null || true
    stop_publisher "$(_cid "${_gi}")" 2>/dev/null || true
  done
}
trap cleanup EXIT

log "EPOCH=${EPOCH}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Phase 1: start a-group + b-group simultaneously ──────────────────────────────
log "Phase 1: starting a-group (val-l05a-0001..0005) + b-group (val-l05b-0001..0005)"
for _gi in $(seq 1 "${GROUP_SIZE}"); do
  start_publisher "$(_aid "${_gi}")" "LiveApp" 500 &
  start_publisher "$(_bid "${_gi}")" "LiveApp" 500 &
done
wait
log "Phase 1: 10 publishers started (a+b groups)"

capture_pulse "/live/overview" "phase1-baseline"

# ── Wait 5 s ────────────────────────────────────────────────────────────────────
log "Waiting 5 s before simultaneous stop(a) + start(c)"
sleep 5

# ── Phase 2: stop a-group while starting c-group ────────────────────────────────
log "Phase 2: stopping a-group + starting c-group simultaneously"
_transition_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
for _gi in $(seq 1 "${GROUP_SIZE}"); do
  stop_publisher "$(_aid "${_gi}")" &
  start_publisher "$(_cid "${_gi}")" "LiveApp" 500 &
done
wait
log "Phase 2: a-group stopped, c-group started at ${_transition_ts}"

# ── Wait 15 s for convergence ────────────────────────────────────────────────────
log "Waiting 15 s for Pulse convergence"
sleep 15

capture_pulse "/live/streams" "post-transition"
capture_pulse "/live/overview" "post-transition"
capture_ams "/LiveApp/rest/v2/broadcasts/list/0/100" "post-transition"

# ── Fetch Pulse streams once for per-stream checks ───────────────────────────────
_pulse_streams="$(curl -s -m 20 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams" 2>/dev/null || echo '{"items":[]}')"

# ── Assert a-group absent from Pulse ─────────────────────────────────────────────
log "Asserting a-group (val-l05a-0001..0005) absent from Pulse"
for _gi in $(seq 1 "${GROUP_SIZE}"); do
  _asid="$(_aid "${_gi}")"
  _a_count="$(printf '%s' "${_pulse_streams}" | \
    jq --arg id "${_asid}" \
      '[(.items // [])[] | select(.stream_id == $id)] | length' \
    2>/dev/null || echo 99)"
  log "Pulse a-stream ${_asid} count=${_a_count} (expect 0)"
  assert_eq "${_a_count}" "0" \
    "${SCENARIO} a-group stream ${_asid} absent from Pulse after stop" || true
done

# ── Assert b-group present in Pulse ──────────────────────────────────────────────
log "Asserting b-group (val-l05b-0001..0005) present in Pulse with publisher_state=publishing"
for _gi in $(seq 1 "${GROUP_SIZE}"); do
  _bsid="$(_bid "${_gi}")"
  _b_state="$(printf '%s' "${_pulse_streams}" | \
    jq -r --arg id "${_bsid}" \
      '(.items // [])[] | select(.stream_id == $id) | .publisher_state' \
    2>/dev/null | head -1 || true)"
  log "Pulse b-stream ${_bsid} publisher_state=${_b_state}"
  assert_eq "${_b_state}" "publishing" \
    "${SCENARIO} b-group stream ${_bsid} publisher_state=publishing" || true
done

# ── Assert c-group present in Pulse ──────────────────────────────────────────────
log "Asserting c-group (val-l05c-0001..0005) present in Pulse with publisher_state=publishing"
for _gi in $(seq 1 "${GROUP_SIZE}"); do
  _csid="$(_cid "${_gi}")"
  _c_state="$(printf '%s' "${_pulse_streams}" | \
    jq -r --arg id "${_csid}" \
      '(.items // [])[] | select(.stream_id == $id) | .publisher_state' \
    2>/dev/null | head -1 || true)"
  log "Pulse c-stream ${_csid} publisher_state=${_c_state}"
  assert_eq "${_c_state}" "publishing" \
    "${SCENARIO} c-group stream ${_csid} publisher_state=publishing" || true
done

# ── Summary count of our live b+c streams (per-stream checks above are canonical) ─
_our_live_count="$(printf '%s' "${_pulse_streams}" | \
  jq '[(.items // [])[] | select(
    (.stream_id | startswith("val-l05b-")) or
    (.stream_id | startswith("val-l05c-"))
  )] | length' 2>/dev/null || echo 0)"
log "Our live streams (b+c groups) in Pulse: ${_our_live_count} (expect 10)"
# Not a hard assert — per-stream checks above are authoritative;
# this is recorded in the timeline for human review.
printf 'transition_ts=%s  our_live_b_c=%s\n' "${_transition_ts}" "${_our_live_count}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ───────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
