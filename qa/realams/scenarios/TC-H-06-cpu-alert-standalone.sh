#!/usr/bin/env bash
# qa/realams/scenarios/TC-H-06-cpu-alert-standalone.sh
#
# TC-H-06: node_cpu threshold rules evaluate against the real CPU of a standalone AMS
#
# History: until D-179 a standalone AMS exposed no CPU over REST and this scenario asserted
# that a `cpu_pct` rule never fired. D-179 made Pulse read /rest/v2/system-resources, so the
# CPU is now real; and since v0.4.1 (D-166) the API refuses `cpu_pct` for THRESHOLD rules —
# it is the anomaly-rule name; threshold rules use `node_cpu`. The old script therefore
# failed with a 422, which in S125 exposed a real web-UI defect (the rule form offered
# `cpu_pct`). This version tests what is true now:
#
# Assertion matrix row:
#   Steps:     1. POST a threshold rule with metric=cpu_pct → must be REFUSED (422)
#              2. Create node_cpu gt 1000 (can never fire) and node_cpu gte 0 (must fire
#                 while the node reports CPU)
#              3. Poll /alerts/history for up to 60 s (evaluator tick ≤5 s)
#   AMS truth: /rest/v2/system-resources reports cpuUsage.systemCPULoad (TC-FL-01)
#   Pulse assert: the gte-0 rule fires (with the latency recorded); the gt-1000 rule never
#              fires; /anomalies answers 200
#   Exit:       0 PASS | 1 FAIL
set -euo pipefail

SCENARIO="TC-H-06"
echo "=== ${SCENARIO}: node_cpu threshold rules vs real standalone CPU ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

EPOCH="$(date +%s)"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

NEVER_ID=""
ALWAYS_ID=""
REFUSED_ID=""

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

cleanup() {
  for _id in "${NEVER_ID}" "${ALWAYS_ID}" "${REFUSED_ID}"; do
    [ -n "${_id}" ] || continue
    log "CLEANUP: deleting alert rule ${_id}"
    curl -s -m 10 -X DELETE -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/alerts/rules/${_id}" > /dev/null 2>&1 || true
  done
}
trap cleanup EXIT

# create_rule NAME METRIC OPERATOR THRESHOLD OUTFILE → prints "HTTP_CODE ID"
create_rule() {
  local body http id
  body="{\"name\":\"$1\",\"metric\":\"$2\",\"operator\":\"$3\",\"threshold\":$4,\"window_s\":0,\"cooldown_s\":1,\"severity\":\"warning\"}"
  http="$(curl -s -m 15 -X POST -H "Authorization: Bearer ${PULSE_TOKEN}" \
    -H "Content-Type: application/json" -d "${body}" -o "$5" -w '%{http_code}' \
    "${PULSE_URL}/alerts/rules" 2>/dev/null || echo 000)"
  id="$(jq -r '.id // empty' "$5" 2>/dev/null || true)"
  printf '%s %s\n' "${http}" "${id}"
}

log "PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Step 1: cpu_pct is the anomaly-rule name — a threshold rule on it must be refused ──
read -r _refused_http REFUSED_ID < <(create_rule "val-h06-cpu-pct-${EPOCH}" cpu_pct gt 0 "${EVIDENCE_DIR}/rule-cpu-pct.json")
log "threshold rule on cpu_pct: HTTP=${_refused_http} id=${REFUSED_ID:-none}"
assert_eq "${_refused_http}" "422" "${SCENARIO} threshold rule on cpu_pct refused (422 — anomaly-only name)" || true

# ── Step 2: two node_cpu rules, one impossible and one certain ──
read -r _never_http NEVER_ID < <(create_rule "val-h06-never-${EPOCH}" node_cpu gt 1000 "${EVIDENCE_DIR}/rule-never.json")
read -r _always_http ALWAYS_ID < <(create_rule "val-h06-always-${EPOCH}" node_cpu gte 0 "${EVIDENCE_DIR}/rule-always.json")
log "node_cpu gt 1000: HTTP=${_never_http} id=${NEVER_ID:-none};  node_cpu gte 0: HTTP=${_always_http} id=${ALWAYS_ID:-none}"
assert_eq "${_never_http}" "201" "${SCENARIO} node_cpu gt 1000 rule created" || true
assert_eq "${_always_http}" "201" "${SCENARIO} node_cpu gte 0 rule created" || true
if [ -z "${NEVER_ID}" ] || [ -z "${ALWAYS_ID}" ]; then
  scenario_verdict
  exit 1
fi

# firing_rows RULE_ID → number of firing history rows
firing_rows() {
  curl -s -m 15 -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/alerts/history?rule_id=$1&state=firing" 2>/dev/null \
    | jq '(.items // []) | length' 2>/dev/null || echo 0
}

# ── Step 3: poll up to 60 s ──
_t0="$(date +%s)"
_fired_after=""
while [ $(( $(date +%s) - _t0 )) -lt 60 ]; do
  if [ "$(firing_rows "${ALWAYS_ID}")" -gt 0 ]; then
    _fired_after=$(( $(date +%s) - _t0 ))
    break
  fi
  sleep 5
done
[ -n "${_fired_after}" ] || sleep $(( 60 - ($(date +%s) - _t0) > 0 ? 60 - ($(date +%s) - _t0) : 0 ))
_never_rows="$(firing_rows "${NEVER_ID}")"
log "gte-0 rule fired after: ${_fired_after:-never (60 s)} s;  gt-1000 rule firing rows: ${_never_rows}"
printf 'always_fired_after_s=%s\nnever_firing_rows=%s\n' "${_fired_after:-none}" "${_never_rows}" >> "${EVIDENCE_DIR}/timeline.txt"
capture_pulse "/alerts/history?rule_id=${ALWAYS_ID}" "alert-history-always"

assert_eq "$([ -n "${_fired_after}" ] && echo fired || echo silent)" "fired" \
  "${SCENARIO} node_cpu gte 0 fires within 60 s on real CPU data" || true
assert_eq "${_never_rows}" "0" "${SCENARIO} node_cpu gt 1000 never fires" || true

# /anomalies must answer 200 (cpu/mem/disk anomaly flags are legitimate now that CPU is real)
_anomaly_http="$(curl -s -m 15 -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer ${PULSE_TOKEN}" "${PULSE_URL}/anomalies" 2>/dev/null || echo 000)"
assert_eq "${_anomaly_http}" "200" "${SCENARIO} /anomalies returns HTTP 200 (not 500)" || true

scenario_verdict
exit $?
