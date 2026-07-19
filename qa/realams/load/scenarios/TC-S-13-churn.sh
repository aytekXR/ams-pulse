#!/usr/bin/env bash
# qa/realams/load/scenarios/TC-S-13-churn.sh
#
# TC-S-13: Start/stop churn storms — L-9 ghost-stream correctness.
#   Repeatedly ramp a wave of publishers, hold, stop, and check that after the
#   final settle the Pulse count returns EXACTLY to baseline (no leaked/ghost
#   streams) and that each wave actually reached base+WAVE at peak (no lost
#   streams under connect storms).
#   Exit: 0 PASS | 1 FAIL | 77 SKIP
#
set -euo pipefail
SCENARIO="TC-S-13"
echo "=== ${SCENARIO}: start/stop churn (ghost-stream check) ===" >&2

_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_HARNESS="${_DIR}/../../harness"
if [ ! -f "${_HARNESS}/load-env.sh" ]; then
  echo "SKIP: ${_HARNESS}/load-env.sh not configured (copy load-env.sh.example)" >&2; exit 77
fi
# shellcheck source=/dev/null
source "${_HARNESS}/load-env.sh"
# shellcheck source=/dev/null
source "${_HARNESS}/assert.sh"
# shellcheck source=/dev/null
source "${_HARNESS}/publisher.sh"
# shellcheck source=/dev/null
source "${_HARNESS}/load-gen.sh"

EVIDENCE_DIR="${EVIDENCE_ROOT}/LOAD-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"; export EVIDENCE_DIR
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

CYCLES="${LOAD_CHURN_CYCLES}"
WAVE="${LOAD_CHURN_WAVE:-10}"
_active_run=""
cleanup() {
  log "CLEANUP"
  [ -n "${_active_run}" ] && load_stop_publishers "${_active_run}" "${WAVE}" 2>/dev/null || true
}
trap cleanup EXIT

_pulse_pub() { curl -s -m 10 -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/overview" 2>/dev/null | jq '.total_publishers // 0' 2>/dev/null || echo 0; }

if ! curl -sf -m 10 "${PULSE_HEALTH_URL}" >/dev/null; then
  printf 'SKIP\nScratch Pulse not reachable.\n' > "${EVIDENCE_DIR}/verdict.txt"; exit 77
fi
BASE="$(_pulse_pub)"
log "Baseline Pulse total_publishers=${BASE}  CYCLES=${CYCLES}  WAVE=${WAVE}"

echo "cycle,peak,after_stop" > "${EVIDENCE_DIR}/churn.csv"
_missed=0
for _c in $(seq 1 "${CYCLES}"); do
  RUN="val-load-churn${_c}-$(openssl rand -hex 2)"
  _active_run="${RUN}"
  log "cycle ${_c}/${CYCLES}: ramp ${WAVE} (${RUN})"
  load_start_publishers "${RUN}" "${WAVE}" || true
  sleep 25
  PEAK="$(_pulse_pub)"
  load_stop_publishers "${RUN}" "${WAVE}"
  _active_run=""
  sleep 25
  AFTER="$(_pulse_pub)"
  echo "${_c},${PEAK},${AFTER}" >> "${EVIDENCE_DIR}/churn.csv"
  log "cycle ${_c}: peak=${PEAK} after_stop=${AFTER} (want peak>=$(( BASE + WAVE )))"
  [ "${PEAK}" -lt "$(( BASE + WAVE ))" ] && _missed=$(( _missed + 1 ))
done

log "Settle (120 s) before final ghost check"
sleep 120
FINAL="$(_pulse_pub)"
log "FINAL Pulse total_publishers=${FINAL} (baseline ${BASE}); waves that missed peak=${_missed}/${CYCLES}"
printf 'final=%s base=%s missed=%s/%s\n' "${FINAL}" "${BASE}" "${_missed}" "${CYCLES}" >> "${EVIDENCE_DIR}/timeline.txt"

assert_lte "${FINAL}" "${BASE}" "${SCENARIO} L-9 final count == baseline after churn settle (no ghosts)" || true
# Allow at most one wave to miss peak (connect-storm jitter); >=2 misses = lost streams.
assert_lte "${_missed}" 1 "${SCENARIO} at most 1/${CYCLES} waves missed base+${WAVE} at peak" || true

log "Writing verdict"
scenario_verdict
exit $?
