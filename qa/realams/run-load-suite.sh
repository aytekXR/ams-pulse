#!/usr/bin/env bash
# qa/realams/run-load-suite.sh
#
# OPT-IN load lane runner (docs/testing/full-e2e-validation-run.md §Rev2, §9).
# Runs TC-S-10..13 against a DEDICATED throw-away AMS + a SCRATCH Pulse, reading
# ONLY qa/realams/harness/load-env.sh — never env.sh, so it can NEVER touch the
# shared validation VPS or prod. Absent/placeholder load-env.sh → whole lane
# SKIPs (77); a forbidden host in it → hard abort (exit 1) via load-env.sh guard.
#
# Exit 0 = all PASS/SKIP; 1 = any FAIL; 77 = lane not configured.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOAD_ENV="${SCRIPT_DIR}/harness/load-env.sh"
LOAD_SCENARIOS="${SCRIPT_DIR}/load/scenarios"

if [ ! -f "${LOAD_ENV}" ]; then
  echo "[load-suite] SKIP: ${LOAD_ENV} not configured." >&2
  echo "[load-suite]   cp qa/realams/harness/load-env.sh.example qa/realams/harness/load-env.sh && edit it." >&2
  exit 77
fi
# load-env.sh runs its own guards (placeholder → 77, forbidden host → 1).
# shellcheck source=/dev/null
source "${LOAD_ENV}"

# ── Preflight the DEDICATED endpoints ─────────────────────────────────────────
if ! curl -sf --max-time 10 "${PULSE_HEALTH_URL}" >/dev/null; then
  echo "[load-suite] scratch Pulse down (${PULSE_HEALTH_URL})" >&2; exit 1
fi
_amscode="$(curl -s --max-time 10 -o /dev/null -w '%{http_code}' "${AMS_URL}/rest/v2/version" 2>/dev/null || echo 000)"
case "${_amscode}" in 200|401|403) : ;; *) echo "[load-suite] load AMS down (http=${_amscode})" >&2; exit 1 ;; esac

RUN_ID="LOAD-$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="${EVIDENCE_ROOT}/${RUN_ID}"
mkdir -p "${RUN_DIR}"
declare -A RESULT

shopt -s nullglob
scripts=( "${LOAD_SCENARIOS}"/TC-S-1[0-9]-*.sh )
if [ "${#scripts[@]}" -eq 0 ]; then
  echo "[load-suite] no TC-S-1x load scenarios installed under ${LOAD_SCENARIOS}" >&2; exit 77
fi

for sc in "${scripts[@]}"; do
  name="$(basename "${sc}" .sh)"
  echo "── ${name}" | tee -a "${RUN_DIR}/run.log"
  bash "${sc}" >"${RUN_DIR}/${name}.log" 2>&1
  rc=$?
  case "${rc}" in 0) RESULT[${name}]=PASS ;; 77) RESULT[${name}]=SKIP ;; *) RESULT[${name}]=FAIL ;; esac
  echo "${name} → ${RESULT[${name}]} (rc=${rc})" | tee -a "${RUN_DIR}/run.log"
done

{
  echo "# Pulse Load Suite — ${RUN_ID}"
  echo
  echo "SUT: ${LOAD_AMS_HOST} (dedicated) · N=${LOAD_N} publishers · M=${LOAD_VIEWERS} viewers · soak=${LOAD_SOAK_MIN}m · generator=${LOAD_GENERATOR}"
  echo
  echo "| Scenario | Result |"
  echo "|---|---|"
  for k in $(printf '%s\n' "${!RESULT[@]}" | sort); do echo "| ${k} | ${RESULT[${k}]} |"; done
  echo
  echo "Budgets L-1..L-9: docs/testing/full-e2e-validation-run.md §Rev2. Per-scenario evidence under ${RUN_DIR}/"
} | tee "${RUN_DIR}/LOAD-REPORT.md"

for k in "${!RESULT[@]}"; do [ "${RESULT[${k}]}" = FAIL ] && exit 1; done
exit 0
