#!/usr/bin/env bash
# qa/realams/load/scenarios/TC-S-12-soak.sh
#
# TC-S-12: Soak — N publishers held for LOAD_SOAK_MIN minutes.
#   L-4  Poll health   : publisher count == base+N across >=95% of samples (ASSERT)
#   L-5  Alert @ load   : a guaranteed-fire rule created mid-soak reaches `firing`
#                         <=30 s (ASSERT when the rule is accepted; else RECORD)
#   L-8  Resource slope : scratch-Pulse RSS series (RECORD; flag slope >10%/h)
#
#   Alert rule (contracts/openapi AlertRuleWrite): name, metric, operator,
#     threshold, window_s, severity  (viewer_count gte 0 → always fires under load).
#     History match is by rule_id + state=="firing" (there is NO rule_name field).
#   Exit: 0 PASS | 1 FAIL | 77 SKIP
#
set -euo pipefail
SCENARIO="TC-S-12"
echo "=== ${SCENARIO}: soak (poll stability + alert-under-load + resource slope) ===" >&2

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

N="${LOAD_N}"
RUN="val-load-$(openssl rand -hex 3)"
RULE_ID=""
cleanup() {
  log "CLEANUP: stopping ${N} publishers + deleting L-5 rule"
  load_stop_publishers "${RUN}" "${N}" 2>/dev/null || true
  [ -n "${RULE_ID}" ] && curl -s -m 10 -H "Authorization: Bearer ${PULSE_TOKEN}" \
    -X DELETE "${PULSE_URL}/alerts/rules/${RULE_ID}" >/dev/null 2>&1 || true
}
trap cleanup EXIT
log "RUN=${RUN}  N=${N}  soak=${LOAD_SOAK_MIN}m  AMS=${AMS_URL}  PULSE=${PULSE_URL}"

_pulse_pub() { curl -s -m 10 -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/overview" 2>/dev/null | jq '.total_publishers // 0' 2>/dev/null || echo 0; }

if ! curl -sf -m 10 "${PULSE_HEALTH_URL}" >/dev/null; then
  printf 'SKIP\nScratch Pulse not reachable.\n' > "${EVIDENCE_DIR}/verdict.txt"; exit 77
fi
BASE="$(_pulse_pub)"
log "Baseline Pulse total_publishers=${BASE}"

log "Ramping ${N} publishers"
if ! load_start_publishers "${RUN}" "${N}"; then
  printf 'SKIP\nLoad generator could not start.\n' > "${EVIDENCE_DIR}/verdict.txt"; exit 77
fi
sleep 12
_cap="$(load_count_ams_broadcasting "${RUN}")"
if [ "${_cap}" -lt "${N}" ]; then
  log "ENV-LIMIT SKIP: AMS accepted only ${_cap}/${N}"
  load_stop_publishers "${RUN}" "${N}"
  printf 'SKIP\nENV-LIMIT: load AMS accepted only %s/%s publishers.\n' "${_cap}" "${N}" > "${EVIDENCE_DIR}/verdict.txt"; exit 77
fi
log "Convergence grace (90 s) before sampling"
sleep 90

# ── Soak loop ────────────────────────────────────────────────────────────────
CSV="${EVIDENCE_DIR}/soak.csv"; echo "t,publishers,latency_s,pulse_rss" > "${CSV}"
TOTAL=$(( LOAD_SOAK_MIN * 60 ))
STABLE=0; SAMPLES=0; FIRED=""; T0=0
SECONDS=0
while [ "${SECONDS}" -lt "${TOTAL}" ]; do
  P="$(_pulse_pub)"
  L="$(curl -s -m 10 -o /dev/null -w '%{time_total}' -H "Authorization: Bearer ${PULSE_TOKEN}" \
        "${PULSE_URL}/live/overview" 2>/dev/null || echo 0)"
  R="na"
  if [ -n "${LOAD_PULSE_CONTAINER:-}" ]; then
    R="$(sg docker -c "docker stats --no-stream --format '{{.MemUsage}}' ${LOAD_PULSE_CONTAINER}" 2>/dev/null \
        | awk -F/ '{print $1}' | tr -d ' ' || echo na)"
  fi
  echo "${SECONDS},${P},${L},${R:-na}" >> "${CSV}"
  SAMPLES=$(( SAMPLES + 1 ))
  [ "${P}" -ge "$(( BASE + N ))" ] && STABLE=$(( STABLE + 1 ))

  # L-5: at half-soak, create a guaranteed-fire rule (viewer_count >= 0)
  if [ -z "${RULE_ID}" ] && [ "${SECONDS}" -gt "$(( TOTAL / 2 ))" ]; then
    RULE_ID="$(curl -s -m 15 -H "Authorization: Bearer ${PULSE_TOKEN}" -X POST \
      "${PULSE_URL}/alerts/rules" -H 'Content-Type: application/json' \
      -d "{\"name\":\"${SCENARIO}-l5\",\"metric\":\"viewer_count\",\"operator\":\"gte\",\"threshold\":0,\"window_s\":60,\"severity\":\"warning\",\"scope\":{\"app\":\"${LOAD_APP}\"}}" \
      2>/dev/null | jq -r '.id // empty' 2>/dev/null || true)"
    if [ -n "${RULE_ID}" ]; then T0="${SECONDS}"; log "L-5 rule created id=${RULE_ID} at t=${SECONDS}s";
    else log "L-5 RECORD: alert rule not accepted (tier/shape) — L-5 not asserted"; fi
  fi
  # L-5: has it fired yet?
  if [ -n "${RULE_ID}" ] && [ -z "${FIRED}" ]; then
    if curl -s -m 10 -H "Authorization: Bearer ${PULSE_TOKEN}" "${PULSE_URL}/alerts/history" 2>/dev/null \
        | jq -e --arg r "${RULE_ID}" '[((.items // .) // [])[]? | select(.rule_id == $r and .state == "firing")] | length > 0' \
        >/dev/null 2>&1; then
      FIRED=$(( SECONDS - T0 )); log "L-5 rule fired after ${FIRED}s"
    fi
  fi
  sleep 30
done

# ── L-4 stability ─────────────────────────────────────────────────────────────
PCT=0; [ "${SAMPLES}" -gt 0 ] && PCT=$(( STABLE * 100 / SAMPLES ))
log "L-4 stability: ${STABLE}/${SAMPLES} samples at base+N = ${PCT}%"
printf 'stable=%s/%s pct=%s\n' "${STABLE}" "${SAMPLES}" "${PCT}" >> "${EVIDENCE_DIR}/timeline.txt"
assert_gte "${PCT}" 95 "${SCENARIO} L-4 publisher count stable in >=95% of ${SAMPLES} samples" || true

# ── L-5 verdict (only assert when the rule was actually created) ──────────────
if [ -n "${RULE_ID}" ]; then
  if [ -n "${FIRED}" ]; then
    assert_lte "${FIRED}" 30 "${SCENARIO} L-5 alert fired under load <=30 s (${FIRED}s)" || true
  else
    assert_eq "fired" "never" "${SCENARIO} L-5 alert rule never fired during soak" || true
  fi
else
  log "L-5 not asserted (rule not accepted) — recorded only"
fi

echo "RECORD L-8: scratch-Pulse RSS series in soak.csv — flag slope >10%/h" >&2
log "Writing verdict"
scenario_verdict
exit $?
