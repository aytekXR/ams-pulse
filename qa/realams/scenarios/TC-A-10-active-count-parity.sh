#!/usr/bin/env bash
# qa/realams/scenarios/TC-A-10-active-count-parity.sh
#
# TC-A-10: Active stream count parity — AMS oracle vs Pulse /live/overview
#
# Assertion matrix row:
#   Steps:         1. Capture AMS + Pulse baseline active-stream counts
#                  2. Start one controlled publisher (val-a10-<hex>)
#                  3. Poll AMS active-live-stream-count until delta=+1 (budget 30 s)
#                  4. Poll Pulse /live/overview total_publishers until delta=+1 (budget 30 s)
#                  5. Assert both DELTAS equal 1 and equal each other
#                  6. Capture JSON evidence (pre-live and live snapshots)
#                  7. Stop publisher; poll Pulse until total_publishers returns to baseline
#   AMS oracle:    GET ${AMS_URL}/${APP}/rest/v2/broadcasts/active-live-stream-count
#                    → .number // .totalActiveBroadcastCount // 0
#   Pulse assert:  GET ${PULSE_URL}/live/overview → .total_publishers // 0
#                  delta(live - baseline) == 1 for both; deltas equal each other
#                  after stop, Pulse total_publishers <= baseline (within 30 s)
#   Risk:          LOW — read-only except for the single test publisher
#   Exit:          0 PASS | 1 FAIL | 77 SKIP (publisher never reached broadcasting)
#
set -euo pipefail

SCENARIO="TC-A-10"
echo "=== ${SCENARIO}: Active-stream count parity (AMS vs Pulse) ===" >&2

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
STREAM_ID="val-a10-$(openssl rand -hex 4)"
APP="${AMS_APP:-LiveApp}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ──────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ──────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping publisher ${STREAM_ID}"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  APP=${APP}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Helper: read AMS active-live-stream-count ──────────────────────────────────
# AMS 3.x field name varies: .number on newer builds, .totalActiveBroadcastCount
# on some older ones. Fall through both; default 0.
_ams_active_count() {
  curl -s -m 10 -b "${AMS_COOKIE_FILE}" \
    "${AMS_URL}/${APP}/rest/v2/broadcasts/active-live-stream-count" \
    2>/dev/null \
    | jq '.number // .totalActiveBroadcastCount // 0' 2>/dev/null \
    || echo 0
}

# ── Helper: read Pulse total_publishers ───────────────────────────────────────
_pulse_pub_count() {
  curl -s -m 10 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/overview" \
    2>/dev/null \
    | jq '.total_publishers // 0' 2>/dev/null \
    || echo 0
}

# ── Baseline captures (before starting the test publisher) ────────────────────
log "Capturing baseline counts"
_ams_base="$(_ams_active_count)"
_pulse_base="$(_pulse_pub_count)"
log "Baseline: AMS active=${_ams_base}  Pulse total_publishers=${_pulse_base}"

capture_ams "/${APP}/rest/v2/broadcasts/active-live-stream-count" "pre-start"
capture_pulse "/live/overview" "pre-start"

# Save raw baseline JSON
curl -s -m 10 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/${APP}/rest/v2/broadcasts/active-live-stream-count" \
  2>/dev/null | jq . > "${EVIDENCE_DIR}/ams-active-count-baseline.json" 2>/dev/null || true

curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/overview" \
  2>/dev/null | jq . > "${EVIDENCE_DIR}/pulse-overview-baseline.json" 2>/dev/null || true

# ── Start publisher ───────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 1000 kbps on ${APP}"
start_publisher "${STREAM_ID}" "${APP}" 1000

# ── Poll AMS until delta=+1 (budget 30 s, 3 s intervals) ──────────────────────
log "Polling AMS active-live-stream-count for delta=+1 (budget 30 s)"
_ams_live=0
_ams_delta=0
_ams_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_live="$(_ams_active_count)"
  _ams_delta=$(( _ams_live - _ams_base ))
  log "AMS active count=${_ams_live} delta=${_ams_delta} (attempt $(( _i + 1 ))/10)"
  if [ "${_ams_delta}" -ge 1 ]; then
    _ams_conv=$(( (_i + 1) * 3 ))
    log "AMS delta=+${_ams_delta} after ${_ams_conv} s"
    break
  fi
  sleep 3
  _i=$(( _i + 1 ))
done

# Capture live AMS snapshot regardless of poll outcome
capture_ams "/${APP}/rest/v2/broadcasts/active-live-stream-count" "live"
curl -s -m 10 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/${APP}/rest/v2/broadcasts/active-live-stream-count" \
  2>/dev/null | jq . > "${EVIDENCE_DIR}/ams-active-count-live.json" 2>/dev/null || true

# AMS must show +1 before we can validate Pulse parity
if [ "${_ams_delta}" -lt 1 ]; then
  log "SKIP: AMS active-live-stream-count never incremented after starting ${STREAM_ID}"
  printf 'SKIP\nPrecondition unmet: AMS active-live-stream-count did not increment.\nbaseline=%s  live=%s  delta=%s\n' \
    "${_ams_base}" "${_ams_live}" "${_ams_delta}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Poll Pulse until delta=+1 (budget 30 s, 3 s intervals) ───────────────────
log "Polling Pulse /live/overview for total_publishers delta=+1 (budget 30 s)"
_pulse_live=0
_pulse_delta=0
_pulse_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _pulse_live="$(_pulse_pub_count)"
  _pulse_delta=$(( _pulse_live - _pulse_base ))
  log "Pulse total_publishers=${_pulse_live} delta=${_pulse_delta} (attempt $(( _i + 1 ))/10)"
  if [ "${_pulse_delta}" -ge 1 ]; then
    _pulse_conv=$(( (_i + 1) * 3 ))
    log "Pulse delta=+${_pulse_delta} after ${_pulse_conv} s"
    break
  fi
  sleep 3
  _i=$(( _i + 1 ))
done

capture_pulse "/live/overview" "live"
curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/overview" \
  2>/dev/null | jq . > "${EVIDENCE_DIR}/pulse-overview-live.json" 2>/dev/null || true

# ── Confirm stream visible in Pulse /live/streams ────────────────────────────
_pulse_stream_count="$(curl -s -m 10 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams" \
  2>/dev/null \
  | jq --arg id "${STREAM_ID}" \
    '[(.items // [])[] | select(.stream_id == $id)] | length' \
  2>/dev/null || echo 0)"
log "Pulse /live/streams presence for ${STREAM_ID}: count=${_pulse_stream_count}"

# ── Stop publisher ────────────────────────────────────────────────────────────
log "Stopping publisher ${STREAM_ID}"
stop_publisher "${STREAM_ID}"

# ── Poll Pulse until total_publishers returns to baseline (budget 30 s) ───────
log "Polling Pulse for total_publishers to return to baseline (budget 30 s)"
_pulse_after=0
_pulse_returned_conv=999
_i=0
while [ "${_i}" -lt 10 ]; do
  _pulse_after="$(_pulse_pub_count)"
  if [ "${_pulse_after}" -le "${_pulse_base}" ]; then
    _pulse_returned_conv=$(( (_i + 1) * 3 ))
    log "Pulse total_publishers back to ${_pulse_after} (baseline=${_pulse_base}) after ${_pulse_returned_conv} s"
    break
  fi
  log "Pulse total_publishers=${_pulse_after} (baseline=${_pulse_base}, attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_pulse "/live/overview" "after-stop"
capture_ams "/${APP}/rest/v2/broadcasts/active-live-stream-count" "after-stop"

# Save summary
{
  printf 'TC-A-10 count-parity run summary:\n'
  printf '  stream_id:          %s\n' "${STREAM_ID}"
  printf '  app:                %s\n' "${APP}"
  printf '  ams_baseline:       %s\n' "${_ams_base}"
  printf '  ams_live:           %s\n' "${_ams_live}"
  printf '  ams_delta:          %s\n' "${_ams_delta}"
  printf '  ams_convergence_s:  %s\n' "${_ams_conv}"
  printf '  pulse_baseline:     %s\n' "${_pulse_base}"
  printf '  pulse_live:         %s\n' "${_pulse_live}"
  printf '  pulse_delta:        %s\n' "${_pulse_delta}"
  printf '  pulse_convergence_s:%s\n' "${_pulse_conv}"
  printf '  pulse_after_stop:   %s\n' "${_pulse_after}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ────────────────────────────────────────────────────────────────
log "ASSERT: AMS delta=1, Pulse delta=1, deltas equal, Pulse returned to baseline"

# AMS delta must equal exactly 1 (our one new publisher, no more)
assert_eq "${_ams_delta}" "1" "${SCENARIO} AMS active-live-stream-count delta=+1" || true

# Pulse delta must equal exactly 1
assert_eq "${_pulse_delta}" "1" "${SCENARIO} Pulse total_publishers delta=+1" || true

# Deltas must be equal to each other (parity)
assert_eq "${_ams_delta}" "${_pulse_delta}" "${SCENARIO} AMS delta == Pulse delta (parity)" || true

# Pulse must have seen the stream in /live/streams during publish window
assert_eq "${_pulse_stream_count}" "1" "${SCENARIO} Pulse /live/streams contains ${STREAM_ID}" || true

# AMS convergence must be within 30 s
assert_lte "${_ams_conv}" 30 "${SCENARIO} AMS delta convergence ≤30 s" || true

# Pulse convergence must be within 30 s
assert_lte "${_pulse_conv}" 30 "${SCENARIO} Pulse delta convergence ≤30 s" || true

# After stop, Pulse total_publishers must return to baseline within 30 s
assert_lte "${_pulse_after}" "${_pulse_base}" "${SCENARIO} Pulse total_publishers returned to baseline after stop" || true

assert_lte "${_pulse_returned_conv}" 30 "${SCENARIO} Pulse baseline-return convergence ≤30 s" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
