#!/usr/bin/env bash
# qa/realams/scenarios/TC-AN-03-anomaly-standalone-empty.sh
#
# TC-AN-03: /anomalies returns HTTP 200, no cpu/mem/disk node findings
#
# Assertion matrix row:
#   Steps:     1. GET /anomalies — no filters
#              2. Assert HTTP 200 (never 500)
#              3. Assert items[] contains NO cpu_pct/mem_pct/disk_pct anomalies
#   AMS truth: standalone AMS does not expose cpu/mem/disk via REST (no cluster);
#              Pulse fleet node cpu_pct=null (TC-H-01 confirmed, S17)
#   Pulse assert: /anomalies HTTP 200; no cpu_pct/mem_pct/disk_pct items
#   Exit:       0 PASS | 1 FAIL
#
# AV-12 (capability-map.md): "GET /api/v1/anomalies → {items:[],meta:{next_cursor:null}}.
#   Empty list. No cpu/mem/disk anomaly findings for standalone node
#   (no data source to baseline against)."
#
set -euo pipefail

SCENARIO="TC-AN-03"
echo "=== ${SCENARIO}: /anomalies HTTP 200, no cpu/mem/disk node findings ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

cleanup() { : ; }
trap cleanup EXIT

log "PULSE_URL=${PULSE_URL}"

# ── GET /anomalies ────────────────────────────────────────────────────────────
log "GET /anomalies (no filters)"
_anomaly_http="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -o "${EVIDENCE_DIR}/anomalies.json" \
  -w '%{http_code}' \
  "${PULSE_URL}/anomalies" 2>/dev/null || echo 000)"

_anomaly_resp="$(jq -c '.' "${EVIDENCE_DIR}/anomalies.json" 2>/dev/null || echo '{}')"
log "Anomaly HTTP=${_anomaly_http}"
capture_pulse "/anomalies" "anomalies"

# Total items
_total_items="$(printf '%s' "${_anomaly_resp}" | \
  jq '(.items // []) | length' 2>/dev/null || echo 0)"
log "Total anomaly items: ${_total_items}"

# Count cpu_pct / mem_pct / disk_pct findings
_cpu_mem_disk_count="$(printf '%s' "${_anomaly_resp}" | \
  jq '[(.items // [])[] | select(.metric == "cpu_pct" or .metric == "mem_pct" or .metric == "disk_pct")] | length' \
  2>/dev/null || echo 0)"
log "cpu_pct/mem_pct/disk_pct findings: ${_cpu_mem_disk_count}"

# Log all anomaly metrics for transparency
_all_metrics="$(printf '%s' "${_anomaly_resp}" | \
  jq -r '[(.items // [])[] | .metric] | unique | join(",")' 2>/dev/null || echo "")"
log "All anomaly metrics found: ${_all_metrics:-none}"

printf 'anomaly_http=%s\ntotal_items=%s\ncpu_mem_disk_count=%s\nall_metrics=%s\n' \
  "${_anomaly_http}" "${_total_items}" "${_cpu_mem_disk_count}" "${_all_metrics:-none}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# Dump full anomaly list for evidence
printf '%s' "${_anomaly_resp}" | jq . > "${EVIDENCE_DIR}/anomalies-full.json" 2>/dev/null || true

# ── Assertions ───────────────────────────────────────────────────────────────
# /anomalies must return HTTP 200 (never 500 — it may be 403 for non-Enterprise)
# For realams Enterprise stack, expect 200
assert_eq "${_anomaly_http}" "200" "${SCENARIO} /anomalies returns HTTP 200 (not 500)" || true

# No cpu_pct / mem_pct / disk_pct anomaly flags (standalone AMS has no such metrics)
assert_eq "${_cpu_mem_disk_count}" "0" "${SCENARIO} no cpu_pct/mem_pct/disk_pct anomaly items (standalone node)" || true

# ── Document standalone anomaly state ─────────────────────────────────────────
{
  printf '=== TC-AN-03: Anomaly endpoint — standalone AMS ===\n'
  printf 'HTTP status: %s\n' "${_anomaly_http}"
  printf 'Total anomaly items: %s\n' "${_total_items}"
  printf 'cpu/mem/disk_pct items: %s (expected 0)\n' "${_cpu_mem_disk_count}"
  printf 'All metrics seen: %s\n' "${_all_metrics:-none}"
  printf '\nEXPECTED BEHAVIOR (AV-12):\n'
  printf '  Standalone AMS does not expose cpu/mem/disk via REST API.\n'
  printf '  Pulse fleet node reports cpu_pct=null, mem_pct=null (TC-H-01, S17).\n'
  printf '  The anomaly detector has no baseline to compare against — no findings.\n'
  printf '  /anomalies returns HTTP 200 with items=[] (empty list, not 500).\n'
  printf '\nREFERENCE:\n'
  printf '  docs/assessment/capability-map.md §F9 (AV-12)\n'
  printf '  scenario-matrix.md TC-AN-03\n'
} > "${EVIDENCE_DIR}/anomaly-standalone-state.txt"
log "State documented in ${EVIDENCE_DIR}/anomaly-standalone-state.txt"

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
