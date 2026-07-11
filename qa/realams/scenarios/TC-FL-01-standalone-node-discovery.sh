#!/usr/bin/env bash
# qa/realams/scenarios/TC-FL-01-standalone-node-discovery.sh
#
# TC-FL-01: Standalone node discovery
#
# Assertion matrix row:
#   Steps:        1. GET /rest/v2/cluster/nodes → must return 404 (standalone AMS)
#                 2. GET /rest/v2/system-status → OS/JVM ground truth
#                 3. GET /api/v1/fleet/nodes → Pulse representation
#   AMS truth:    cluster/nodes=404; system-status has osName/javaVersion
#   Pulse assert: 0 or 1 node; if present: os_name/java_version populated;
#                 cpu_pct/mem_pct absent-or-null (NOT false-zero)
#   Exit:         0 PASS | 1 FAIL
#
set -euo pipefail

SCENARIO="TC-FL-01"
echo "=== ${SCENARIO}: Standalone node discovery ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

cleanup() { : ; }
trap cleanup EXIT

log "PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Step 1: AMS cluster/nodes must 404 (standalone — no cluster) ─────────────
log "AMS ground truth: GET /rest/v2/cluster/nodes (expect 404)"
_cluster_code="$(curl -s -m 15 -b "${AMS_COOKIE_FILE}" \
  -o /dev/null -w '%{http_code}' \
  "${AMS_URL}/rest/v2/cluster/nodes" 2>/dev/null || echo 0)"
log "AMS /rest/v2/cluster/nodes: HTTP ${_cluster_code}"
printf 'ams_cluster_nodes_http=%s\n' "${_cluster_code}" >> "${EVIDENCE_DIR}/timeline.txt"

assert_eq "${_cluster_code}" "404" "${SCENARIO} AMS /rest/v2/cluster/nodes returns 404 (standalone)" || true

# ── Step 2: AMS system-status for OS/JVM ground truth ───────────────────────
log "AMS ground truth: GET /rest/v2/system-status"
capture_ams "/rest/v2/system-status" "system-status"
_sys_raw="$(curl -s -m 15 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/rest/v2/system-status" 2>/dev/null || echo '{}')"

_ams_os_name="$(printf '%s' "${_sys_raw}" | jq -r '.osName // ""' 2>/dev/null || true)"
_ams_os_arch="$(printf '%s' "${_sys_raw}" | jq -r '.osArch // ""' 2>/dev/null || true)"
_ams_java_ver="$(printf '%s' "${_sys_raw}" | jq -r '.javaVersion // ""' 2>/dev/null || true)"
_ams_proc_count="$(printf '%s' "${_sys_raw}" | jq '.processorCount // 0' 2>/dev/null || echo 0)"

log "AMS system-status: osName=${_ams_os_name} osArch=${_ams_os_arch} javaVersion=${_ams_java_ver} processorCount=${_ams_proc_count}"
printf 'ams_os_name=%s\nams_os_arch=%s\nams_java_version=%s\nams_processor_count=%s\n' \
  "${_ams_os_name}" "${_ams_os_arch}" "${_ams_java_ver}" "${_ams_proc_count}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# Assert AMS has the expected standalone OS info
assert_eq "$([ -n "${_ams_os_name}" ] && echo present || echo absent)" "present" \
  "${SCENARIO} AMS system-status.osName is present (ground truth)" || true
assert_eq "$([ -n "${_ams_java_ver}" ] && echo present || echo absent)" "present" \
  "${SCENARIO} AMS system-status.javaVersion is present (ground truth)" || true

# ── Step 3: Pulse fleet/nodes ────────────────────────────────────────────────
log "Pulse: GET /api/v1/fleet/nodes"
capture_pulse "/fleet/nodes" "fleet-nodes"
_fleet_raw="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/fleet/nodes" 2>/dev/null || echo '{}')"

_node_count="$(printf '%s' "${_fleet_raw}" | jq '(.items // []) | length' 2>/dev/null || echo 0)"
log "Pulse fleet/nodes: ${_node_count} node(s)"
printf 'pulse_node_count=%s\n' "${_node_count}" >> "${EVIDENCE_DIR}/timeline.txt"

if [ "${_node_count}" -gt 0 ]; then
  _node="$(printf '%s' "${_fleet_raw}" | jq '.items[0]' 2>/dev/null || echo '{}')"
  log "First node: $(printf '%s' "${_node}" | jq -c '{node_id,role,status,os_name,java_version,cpu_pct,mem_pct}' 2>/dev/null || echo 'parse_error')"

  # OS name and JVM version must be populated if node is present
  _pulse_os="$(printf '%s' "${_node}" | jq -r '.os_name // ""' 2>/dev/null || true)"
  assert_eq "$([ -n "${_pulse_os}" ] && echo present || echo absent)" "present" \
    "${SCENARIO} fleet node os_name populated from AMS system-status" || true

  _pulse_java="$(printf '%s' "${_node}" | jq -r '.java_version // ""' 2>/dev/null || true)"
  assert_eq "$([ -n "${_pulse_java}" ] && echo present || echo absent)" "present" \
    "${SCENARIO} fleet node java_version populated from AMS system-status" || true

  # cpu_pct and mem_pct must be null — standalone AMS does not expose these
  _cpu_val="$(printf '%s' "${_node}" | jq '.cpu_pct' 2>/dev/null || echo null)"
  assert_eq "${_cpu_val}" "null" "${SCENARIO} fleet node cpu_pct null (not false-zero)" || true

  _mem_val="$(printf '%s' "${_node}" | jq '.mem_pct' 2>/dev/null || echo null)"
  assert_eq "${_mem_val}" "null" "${SCENARIO} fleet node mem_pct null (not false-zero)" || true

  printf 'pulse_node_os_name=%s\npulse_node_java_version=%s\npulse_cpu_pct=%s\npulse_mem_pct=%s\n' \
    "${_pulse_os}" "${_pulse_java}" "${_cpu_val}" "${_mem_val}" \
    >> "${EVIDENCE_DIR}/timeline.txt"
else
  # 0 nodes is acceptable for standalone — Pulse may not synthesize a node without cluster API
  log "INFO: Pulse returned 0 fleet nodes for standalone AMS — synthetic node not implemented (acceptable)"
  printf 'note=0_fleet_nodes_returned_for_standalone_AMS\n' >> "${EVIDENCE_DIR}/timeline.txt"
fi

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
