#!/usr/bin/env bash
# qa/realams/load/scenarios/TC-S-10-publisher-ramp.sh
#
# TC-S-10: Publisher ramp under load — Pulse convergence + oracle parity + API
#          latency + ghost-free teardown, at LOAD_N publishers on a DEDICATED AMS.
#
# Budgets exercised (docs/testing/full-e2e-validation-run.md §Rev2, L-1..L-9):
#   L-1  Convergence  : Pulse total_publishers delta >= +N within 120 s
#   L-2  Oracle parity: AMS active-live-stream-count delta and Pulse delta both +N
#   L-3  API latency  : GET /live/overview p95 < 300 ms (ASSERT at N<=50, RECORD above)
#   L-9  Teardown     : counts return to baseline <= 60 s after generators stop (no ghosts)
#
#   AMS oracle : GET ${AMS_URL}/${LOAD_APP}/rest/v2/broadcasts/active-live-stream-count
#   Pulse      : GET ${PULSE_URL}/live/overview → .total_publishers
#   Streams    : all IDs begin with val-load-<hex> (owned; delta assertions only)
#   Exit       : 0 PASS | 1 FAIL | 77 SKIP (load-env absent / instance too small / down)
#
set -euo pipefail
SCENARIO="TC-S-10"
echo "=== ${SCENARIO}: publisher ramp under load ===" >&2

# ── Harness bootstrap (load lane lives one dir deeper than scenarios/) ─────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_HARNESS="${_DIR}/../../harness"
if [ ! -f "${_HARNESS}/load-env.sh" ]; then
  echo "SKIP: ${_HARNESS}/load-env.sh not configured (copy load-env.sh.example) — load lane needs a DEDICATED instance" >&2
  exit 77
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
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

N="${LOAD_N}"
RUN="val-load-$(openssl rand -hex 3)"
cleanup() { log "CLEANUP: stopping ${N} publishers for ${RUN}"; load_stop_publishers "${RUN}" "${N}"; }
trap cleanup EXIT
log "RUN=${RUN}  N=${N}  generator=${LOAD_GENERATOR}  AMS=${AMS_URL}  PULSE=${PULSE_URL}"

_ams_active() {
  curl -s -m 10 -b "${AMS_COOKIE_FILE}" \
    "${AMS_URL}/${LOAD_APP}/rest/v2/broadcasts/active-live-stream-count" 2>/dev/null \
    | jq '.number // .totalActiveBroadcastCount // 0' 2>/dev/null || echo 0
}
_pulse_pub() {
  curl -s -m 10 -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/overview" 2>/dev/null | jq '.total_publishers // 0' 2>/dev/null || echo 0
}

# ── Preflight ─────────────────────────────────────────────────────────────────
if ! curl -sf -m 10 "${PULSE_HEALTH_URL}" >/dev/null; then
  log "SKIP: scratch Pulse health check failed (${PULSE_HEALTH_URL})"
  printf 'SKIP\nScratch Pulse not reachable.\n' > "${EVIDENCE_DIR}/verdict.txt"; exit 77
fi
_amsver="$(curl -s -m 10 -o /dev/null -w '%{http_code}' "${AMS_URL}/rest/v2/version" 2>/dev/null || echo 000)"
case "${_amsver}" in 200|401|403) : ;; *)
  log "SKIP: load AMS unreachable (http=${_amsver})"
  printf 'SKIP\nLoad AMS not reachable (http=%s).\n' "${_amsver}" > "${EVIDENCE_DIR}/verdict.txt"; exit 77 ;;
esac

BASE_A="$(_ams_active)"; BASE_P="$(_pulse_pub)"
log "Baseline: AMS active=${BASE_A}  Pulse total_publishers=${BASE_P}"

# ── Ramp ──────────────────────────────────────────────────────────────────────
log "Ramping ${N} publishers"
if ! load_start_publishers "${RUN}" "${N}"; then
  log "SKIP: generator could not start (official mode missing media/tool?)"
  printf 'SKIP\nLoad generator could not start.\n' > "${EVIDENCE_DIR}/verdict.txt"; exit 77
fi

# ── ENV-LIMIT capacity probe (mirror TC-S-01): avoid false-FAIL on a small box ─
sleep 12
_cap="$(load_count_ams_broadcasting "${RUN}")"
log "Capacity probe: AMS shows ${_cap}/${N} ${RUN} streams broadcasting"
printf 'capacity_probe=%s/%s\n' "${_cap}" "${N}" >> "${EVIDENCE_DIR}/timeline.txt"
if [ "${_cap}" -lt "${N}" ]; then
  log "ENV-LIMIT SKIP: AMS accepted only ${_cap}/${N} — dedicated instance too small for N=${N}"
  load_stop_publishers "${RUN}" "${N}"
  printf 'SKIP\nENV-LIMIT: load AMS accepted only %s/%s publishers — size the instance up or lower LOAD_N.\n' \
    "${_cap}" "${N}" > "${EVIDENCE_DIR}/verdict.txt"; exit 77
fi

# ── L-3 latency sampler (background, ~120 s of 3 s samples) ────────────────────
LAT="${EVIDENCE_DIR}/overview-latency.csv"; : > "${LAT}"
( _s=0; while [ "${_s}" -lt 40 ]; do
    curl -s -m 10 -o /dev/null -w '%{time_total}\n' \
      -H "Authorization: Bearer ${PULSE_TOKEN}" "${PULSE_URL}/live/overview" >> "${LAT}" 2>/dev/null || true
    _s=$(( _s + 1 )); sleep 3
  done ) & SAMPLER=$!

# ── L-1/L-2 convergence (poll every 5 s up to 120 s) ─────────────────────────
A="${BASE_A}"; P="${BASE_P}"; CONV=999; _i=0
while [ "${_i}" -lt 24 ]; do
  A="$(_ams_active)"; P="$(_pulse_pub)"
  log "converge attempt $(( _i + 1 ))/24: AMS=${A} (base ${BASE_A})  Pulse=${P} (base ${BASE_P})"
  if [ "${A}" -ge "$(( BASE_A + N ))" ] && [ "${P}" -ge "$(( BASE_P + N ))" ]; then
    CONV=$(( (_i + 1) * 5 )); log "Converged after ${CONV} s"; break
  fi
  sleep 5; _i=$(( _i + 1 ))
done
wait "${SAMPLER}" 2>/dev/null || true

D_A=$(( A - BASE_A )); D_P=$(( P - BASE_P ))
log "Deltas: AMS=${D_A}  Pulse=${D_P}  (target +${N})  convergence=${CONV}s"
printf 'delta_ams=%s delta_pulse=%s convergence_s=%s\n' "${D_A}" "${D_P}" "${CONV}" >> "${EVIDENCE_DIR}/timeline.txt"

assert_gte "${A}" "$(( BASE_A + N ))" "${SCENARIO} L-2 AMS active-live-stream-count +${N}" || true
assert_gte "${P}" "$(( BASE_P + N ))" "${SCENARIO} L-1 Pulse total_publishers +${N}" || true
assert_lte "${CONV}" 120 "${SCENARIO} L-1 convergence <=120 s" || true

# ── L-3 p95 latency ──────────────────────────────────────────────────────────
P95="$(sort -n "${LAT}" 2>/dev/null | awk 'NF{a[++n]=$1} END{ if(n){ i=int(n*0.95); if(i<1)i=1; print a[i] } else print 0 }')"
log "L-3 /live/overview p95=${P95}s over $(wc -l < "${LAT}" 2>/dev/null || echo 0) samples at N=${N}"
printf 'overview_p95_s=%s samples=%s\n' "${P95}" "$(wc -l < "${LAT}" 2>/dev/null || echo 0)" >> "${EVIDENCE_DIR}/timeline.txt"
if [ "${N}" -le 50 ]; then
  _p95_ok="$(awk -v p="${P95}" 'BEGIN{ print (p>0 && p<0.30) ? "1" : "0" }')"
  assert_eq "${_p95_ok}" "1" "${SCENARIO} L-3 /live/overview p95 < 300 ms at N=${N} (p95=${P95}s)" || true
else
  log "RECORD L-3 p95=${P95}s at N=${N} (hard assert tier is N<=50)"
fi

# ── L-9 teardown / ghost check ───────────────────────────────────────────────
log "Teardown: stopping ${N} publishers"
load_stop_publishers "${RUN}" "${N}"
_after="${P}"; _drain=999; _i=0
while [ "${_i}" -lt 12 ]; do
  sleep 5; _after="$(_pulse_pub)"
  if [ "${_after}" -le "${BASE_P}" ]; then _drain=$(( (_i + 1) * 5 )); log "Drained to ${_after} after ${_drain} s"; break; fi
  log "drain attempt $(( _i + 1 ))/12: Pulse=${_after} (base ${BASE_P})"
  _i=$(( _i + 1 ))
done
assert_lte "${_after}" "${BASE_P}" "${SCENARIO} L-9 Pulse total_publishers back to baseline (no ghosts)" || true
assert_lte "${_drain}" 60 "${SCENARIO} L-9 teardown convergence <= 60 s" || true

log "Writing verdict"
scenario_verdict
exit $?
