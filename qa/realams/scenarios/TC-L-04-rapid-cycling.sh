#!/usr/bin/env bash
# qa/realams/scenarios/TC-L-04-rapid-cycling.sh
#
# TC-L-04: Rapid start/stop cycling
#
# Assertion matrix row:
#   Steps:         1. 5 cycles of start → sleep 5 s → stop on unique IDs val-l04-<epoch>-<n>
#                  2. After each stop: poll AMS for terminal (finished|removed-404) ≤30 s
#                  3. After each stop: poll Pulse for no phantom stream ≤30 s
#                  4. Final assert: zero val-l04-<epoch>-* streams in Pulse
#   AMS truth:     status transitions to finished or object removed-404 each cycle
#   Pulse assert:  no phantom streams persist after each cycle; clean slate at end
#   Exit:          0 PASS | 1 FAIL | 77 SKIP
#
set -euo pipefail

SCENARIO="TC-L-04"
echo "=== ${SCENARIO}: Rapid start/stop cycling (5 cycles) ===" >&2

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
CYCLES=5

# Stream IDs: val-l04-<epoch>-1 … val-l04-<epoch>-5
_sid() { printf 'val-l04-%s-%s' "${EPOCH}" "$1"; }

EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap: stop all cycle streams (idempotent) ────────────────────────────
cleanup() {
  local _n
  for _n in $(seq 1 "${CYCLES}"); do
    stop_publisher "$(_sid "${_n}")" 2>/dev/null || true
  done
}
trap cleanup EXIT

log "EPOCH=${EPOCH}  CYCLES=${CYCLES}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Cycle loop ───────────────────────────────────────────────────────────────────
for _n in $(seq 1 "${CYCLES}"); do
  _CYCLE_ID="$(_sid "${_n}")"
  log "--- Cycle ${_n}/${CYCLES}: stream=${_CYCLE_ID} ---"

  # Start publisher
  start_publisher "${_CYCLE_ID}" "LiveApp" 500

  # Hold ~5 s — enough for AMS to register, short enough to stress the cleanup path
  sleep 5

  # Graceful stop
  _stop_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  log "Cycle ${_n}: stopping ${_CYCLE_ID} at ${_stop_ts}"
  stop_publisher "${_CYCLE_ID}"

  # ── Poll AMS for terminal state (≤30 s, 3 s interval) ─────────────────────
  log "Cycle ${_n}: polling AMS for terminal state (finished|removed-404) ≤30 s"
  _ams_terminal_state=""
  _ams_conv=999
  _i=0
  while [ "${_i}" -lt 10 ]; do
    _poll_body="${EVIDENCE_DIR}/ams-cycle${_n}-poll-${_i}.json"
    _http_code="$(curl -s -m 10 \
      -o "${_poll_body}" -w '%{http_code}' \
      "${AMS_URL}/LiveApp/rest/v2/broadcasts/${_CYCLE_ID}" 2>/dev/null || echo 000)"
    if [ "${_http_code}" = "404" ]; then
      _ams_terminal_state="removed"
    else
      _ams_terminal_state="$(jq -r '.status // "unknown"' "${_poll_body}" 2>/dev/null || echo unknown)"
    fi
    rm -f "${_poll_body}"
    if [ "${_ams_terminal_state}" = "finished" ] || [ "${_ams_terminal_state}" = "removed" ]; then
      _ams_conv=$(( (_i + 1) * 3 ))
      log "Cycle ${_n}: AMS terminal=${_ams_terminal_state} after ${_ams_conv} s"
      break
    fi
    log "Cycle ${_n}: AMS status=${_ams_terminal_state} http=${_http_code} (attempt $(( _i + 1 ))/10)"
    sleep 3
    _i=$(( _i + 1 ))
  done

  # Map to terminal/not_terminal for assert
  _ams_is_terminal="not_terminal"
  case "${_ams_terminal_state}" in finished|removed) _ams_is_terminal="terminal" ;; esac

  assert_eq "${_ams_is_terminal}" "terminal" \
    "${SCENARIO} cycle${_n} AMS terminal (finished|removed-404; observed: ${_ams_terminal_state})" || true
  assert_lte "${_ams_conv}" 30 "${SCENARIO} cycle${_n} AMS terminal convergence ≤30 s" || true

  # ── Poll Pulse for no phantom stream (≤30 s, 3 s interval) ────────────────
  log "Cycle ${_n}: polling Pulse for no phantom stream ${_CYCLE_ID} (≤30 s)"
  _pulse_phantom_count=99
  _pulse_conv=999
  _i=0
  while [ "${_i}" -lt 10 ]; do
    _pulse_phantom_count="$(curl -s -m 10 \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/live/streams" 2>/dev/null \
      | jq --arg id "${_CYCLE_ID}" \
          '[(.items // [])[] | select(.stream_id == $id)] | length' \
          2>/dev/null || echo 99)"
    if [ "${_pulse_phantom_count}" = "0" ]; then
      _pulse_conv=$(( (_i + 1) * 3 ))
      log "Cycle ${_n}: Pulse stream absent after ${_pulse_conv} s"
      break
    fi
    log "Cycle ${_n}: Pulse still shows ${_pulse_phantom_count} stream(s) for ${_CYCLE_ID} (attempt $(( _i + 1 ))/10)"
    sleep 3
    _i=$(( _i + 1 ))
  done

  assert_eq "${_pulse_phantom_count}" "0" \
    "${SCENARIO} cycle${_n} no phantom stream in Pulse for ${_CYCLE_ID}" || true
  assert_lte "${_pulse_conv}" 30 \
    "${SCENARIO} cycle${_n} Pulse phantom-clean convergence ≤30 s" || true

  log "Cycle ${_n} complete: ams_terminal=${_ams_terminal_state} pulse_phantom=${_pulse_phantom_count}"
  printf 'cycle%s: stop_ts=%s ams_terminal=%s ams_conv=%s pulse_conv=%s\n' \
    "${_n}" "${_stop_ts}" "${_ams_terminal_state}" "${_ams_conv}" "${_pulse_conv}" \
    >> "${EVIDENCE_DIR}/timeline.txt"
done

# ── Final: assert zero val-l04-<epoch>-* streams in Pulse ────────────────────────
log "Final: asserting zero val-l04-${EPOCH}-* streams remain in Pulse"
_prefix="val-l04-${EPOCH}-"
_final_count="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams" 2>/dev/null \
  | jq --arg pfx "${_prefix}" \
      '[(.items // [])[] | select(.stream_id | startswith($pfx))] | length' \
      2>/dev/null || echo 99)"
log "Final val-l04-${EPOCH}-* count in Pulse: ${_final_count}"
assert_eq "${_final_count}" "0" \
  "${SCENARIO} final: zero val-l04-${EPOCH}-* streams in Pulse" || true

capture_pulse "/live/streams" "final"
capture_pulse "/live/overview" "final"

# ── Verdict ───────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
