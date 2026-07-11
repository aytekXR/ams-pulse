#!/usr/bin/env bash
# qa/realams/scenarios/TC-H-06-cpu-alert-standalone.sh
#
# TC-H-06: Standalone AMS provides no cpu_pct — cpu_pct alert must NOT fire
#
# Assertion matrix row:
#   Steps:     1. Create alert rule metric=cpu_pct op=gt threshold=0
#              2. Wait 60 s (≥ 2 evaluator ticks; evaluator tick ≤5 s in realams)
#              3. Assert /alerts/history for this rule_id has 0 firing rows
#              4. Assert /anomalies has no cpu_pct / mem_pct / disk_pct node findings
#   AMS truth: standalone AMS (non-cluster) does NOT expose cpu_pct via REST;
#              Pulse fleet node shows cpu_pct=null (TC-H-01 confirmed, S17)
#   Pulse assert: no cpu_pct firing rows after 60 s — evaluator has no metric data
#   Exit:       0 PASS | 1 FAIL
#
# Capability map cross-link:
#   docs/assessment/capability-map.md §F9 / TC-AN-03 row:
#   "cpu/mem/disk anomaly: standalone AMS does not provide these from REST;
#    anomaly detector has no data to baseline against."
#
set -euo pipefail

SCENARIO="TC-H-06"
echo "=== ${SCENARIO}: cpu_pct alert — standalone AMS, no firing expected ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EPOCH="$(date +%s)"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

RULE_ID=""

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────
cleanup() {
  if [ -n "${RULE_ID}" ]; then
    log "CLEANUP: deleting alert rule ${RULE_ID}"
    curl -s -m 10 -X DELETE \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/alerts/rules/${RULE_ID}" > /dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

log "PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Step 1: Create cpu_pct alert rule ─────────────────────────────────────────
# No stream scope — this is a node-level metric (evaluated globally)
log "Creating alert rule: metric=cpu_pct op=gt threshold=0 (no scope)"
_rule_body="{\"name\":\"val-h06-cpu-rule-${EPOCH}\",\"metric\":\"cpu_pct\",\"operator\":\"gt\",\"threshold\":0,\"window_s\":0,\"cooldown_s\":1,\"severity\":\"warning\"}"
_rule_http="$(curl -s -m 15 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${_rule_body}" \
  -o "${EVIDENCE_DIR}/rule-create.json" \
  -w '%{http_code}' \
  "${PULSE_URL}/alerts/rules" 2>/dev/null || echo 000)"

_rule_resp="$(jq -c '.' "${EVIDENCE_DIR}/rule-create.json" 2>/dev/null || echo '{}')"
RULE_ID="$(printf '%s' "${_rule_resp}" | jq -r '.id // empty' 2>/dev/null || true)"
log "Rule create HTTP=${_rule_http}  id=${RULE_ID:-EMPTY}"

if [ "${_rule_http}" != "201" ] || [ -z "${RULE_ID}" ]; then
  log "FAIL: rule creation returned HTTP ${_rule_http} or empty id — cannot evaluate"
  assert_eq "${_rule_http}" "201" "${SCENARIO} cpu_pct alert rule created (HTTP 201)" || true
  scenario_verdict
  exit 1
fi

printf 'rule_id=%s\n' "${RULE_ID}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Step 2: Wait 60 s for evaluator to tick ───────────────────────────────────
# Evaluator tick ≤5 s in realams stack; 60 s = ≥12 ticks with no cpu_pct data.
log "Waiting 60 s for evaluator to tick (≥12 ticks at ≤5 s interval)"
sleep 60
log "60 s elapsed — checking firing history"

# ── Step 3: Assert no firing rows for this rule_id ────────────────────────────
capture_pulse "/alerts/history?rule_id=${RULE_ID}&state=firing" "alert-history"
_hist_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/alerts/history?rule_id=${RULE_ID}&state=firing" \
  2>/dev/null || echo '{}')"
printf '%s' "${_hist_resp}" | jq . > "${EVIDENCE_DIR}/alert-history.json" 2>/dev/null || true

_firing_count="$(printf '%s' "${_hist_resp}" | \
  jq '(.items // []) | length' 2>/dev/null || echo 0)"
log "Firing row count for cpu_pct rule after 60 s: ${_firing_count}"
printf 'firing_row_count=%s\n' "${_firing_count}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Step 4: Check /anomalies for cpu_pct / mem_pct / disk_pct ────────────────
capture_pulse "/anomalies" "anomalies"
_anomaly_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/anomalies" \
  2>/dev/null || echo '{}')"
printf '%s' "${_anomaly_resp}" | jq . > "${EVIDENCE_DIR}/anomalies.json" 2>/dev/null || true

_anomaly_http="$(curl -s -m 15 -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/anomalies" 2>/dev/null || echo 000)"

_cpu_anomalies="$(printf '%s' "${_anomaly_resp}" | \
  jq '[(.items // [])[] | select(.metric == "cpu_pct" or .metric == "mem_pct" or .metric == "disk_pct")] | length' \
  2>/dev/null || echo 0)"
log "Anomaly HTTP=${_anomaly_http}  cpu/mem/disk_pct anomalies: ${_cpu_anomalies}"
printf 'anomaly_http=%s\ncpu_mem_disk_anomaly_count=%s\n' \
  "${_anomaly_http}" "${_cpu_anomalies}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ───────────────────────────────────────────────────────────────
# PASS = no firing rows (standalone AMS provides no cpu_pct data to evaluate)
assert_eq "${_firing_count}" "0" "${SCENARIO} cpu_pct rule: 0 firing rows after 60 s (no cpu data from standalone AMS)" || true

# /anomalies must return HTTP 200 (never 500)
assert_eq "${_anomaly_http}" "200" "${SCENARIO} /anomalies returns HTTP 200 (not 500)" || true

# No cpu/mem/disk anomaly flags from standalone node
assert_eq "${_cpu_anomalies}" "0" "${SCENARIO} no cpu_pct/mem_pct/disk_pct node anomalies (standalone AMS)" || true

# ── Document standalone gap ───────────────────────────────────────────────────
{
  printf '=== TC-H-06: cpu_pct alert standalone gap ===\n'
  printf 'AMS topology: standalone (non-cluster); no cpu_pct metric via REST\n'
  printf 'Rule: metric=cpu_pct op=gt threshold=0 id=%s\n' "${RULE_ID}"
  printf 'Firing rows after 60 s: %s (expected 0)\n' "${_firing_count}"
  printf 'Anomaly cpu/mem/disk flags: %s (expected 0)\n' "${_cpu_anomalies}"
  printf '\nROOT CAUSE:\n'
  printf '  AMS system-status (/rest/v2/system-status) does NOT return cpu/mem/disk\n'
  printf '  values from the standalone REST API. Pulse fleet node reports cpu_pct=null.\n'
  printf '  The alert evaluator has no cpu_pct samples to evaluate against, so the\n'
  printf '  rule never fires. See: docs/assessment/capability-map.md §F9\n'
} > "${EVIDENCE_DIR}/cpu-standalone-gap.txt"
log "Gap documented in ${EVIDENCE_DIR}/cpu-standalone-gap.txt"

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
