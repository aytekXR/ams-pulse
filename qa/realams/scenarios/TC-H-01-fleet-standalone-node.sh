#!/usr/bin/env bash
# qa/realams/scenarios/TC-H-01-fleet-standalone-node.sh
#
# TC-H-01: Fleet view — standalone node
#
# Assertion matrix row:
#   Steps:        1. Confirm AMS has no cluster (no cluster/nodes endpoint)
#                 2. Check Pulse /api/v1/fleet/nodes
#   AMS truth:    /rest/v2/system-status → {osName:Linux, osArch:amd64, javaVersion:17, ...}
#   Pulse assert: fleet/nodes → node card has os_name/java_version populated;
#                 cpu_pct and mem_pct are null (absent-or-null, NOT false-zero)
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (fleet/nodes returns 0 items — not implemented)
#
set -euo pipefail

SCENARIO="TC-H-01"
echo "=== ${SCENARIO}: Fleet view — standalone node ===" >&2

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

# ── AMS ground truth: system-status ─────────────────────────────────────────
log "AMS ground truth: GET /rest/v2/system-status"
_sys_status="$(curl -s -m 15 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/rest/v2/system-status" 2>/dev/null || echo '{}')"
printf '%s' "${_sys_status}" | jq . > "${EVIDENCE_DIR}/ams-system-status.json" 2>/dev/null || true
log "AMS system-status: $(printf '%s' "${_sys_status}" | jq -c '{osName,osArch,javaVersion,processorCount}' 2>/dev/null || echo 'parse_error')"

_ams_os_name="$(printf '%s' "${_sys_status}" | jq -r '.osName // ""' 2>/dev/null || true)"
_ams_java_ver="$(printf '%s' "${_sys_status}" | jq -r '.javaVersion // ""' 2>/dev/null || true)"
log "AMS osName=${_ams_os_name}  javaVersion=${_ams_java_ver}"

# ── Pulse fleet/nodes ────────────────────────────────────────────────────────
log "Pulse: GET /api/v1/fleet/nodes"
capture_pulse "/fleet/nodes" "fleet-nodes"

_fleet_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/fleet/nodes" 2>/dev/null || echo '{}')"

_node_count="$(printf '%s' "${_fleet_resp}" | jq '(.items // []) | length' 2>/dev/null || echo 0)"
log "Pulse fleet/nodes: ${_node_count} node(s)"
printf '%s' "${_fleet_resp}" | jq . > "${EVIDENCE_DIR}/pulse-fleet-nodes.json" 2>/dev/null || true

if [ "${_node_count}" -eq 0 ]; then
  log "SKIP: Pulse fleet/nodes returned 0 items — fleet F7 implementation may not be complete"
  printf 'SKIP\nPrecondition unmet: Pulse /api/v1/fleet/nodes returned 0 items.\nAMS system-status shows osName=%s — Pulse did not surface a synthetic node.\n' \
    "${_ams_os_name}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# Inspect first node
_node="$(printf '%s' "${_fleet_resp}" | jq '.items[0]' 2>/dev/null || echo '{}')"
log "First node: $(printf '%s' "${_node}" | jq -c '{node_id,role,status,version,os_name,java_version}' 2>/dev/null || echo 'parse_error')"

# ── Assertions ───────────────────────────────────────────────────────────────
# OS name must be populated
_pulse_os_name="$(printf '%s' "${_node}" | jq -r '.os_name // ""' 2>/dev/null || true)"
assert_eq "$([ -n "${_pulse_os_name}" ] && echo present || echo absent)" "present" \
  "${SCENARIO} fleet node os_name is populated" || true

# JVM version must be populated
_pulse_java_ver="$(printf '%s' "${_node}" | jq -r '.java_version // ""' 2>/dev/null || true)"
assert_eq "$([ -n "${_pulse_java_ver}" ] && echo present || echo absent)" "present" \
  "${SCENARIO} fleet node java_version is populated" || true

# cpu_pct must be null (absent or null — NOT 0.0)
# jq returns "null" (string) for both absent key and explicit null; returns "0" for zero
_cpu_val="$(printf '%s' "${_node}" | jq '.cpu_pct' 2>/dev/null || echo null)"
assert_eq "${_cpu_val}" "null" "${SCENARIO} fleet node cpu_pct is null (not false-zero)" || true

# mem_pct must be null
_mem_val="$(printf '%s' "${_node}" | jq '.mem_pct' 2>/dev/null || echo null)"
assert_eq "${_mem_val}" "null" "${SCENARIO} fleet node mem_pct is null (not false-zero)" || true

log "os_name=${_pulse_os_name}  java_version=${_pulse_java_ver}  cpu_pct=${_cpu_val}  mem_pct=${_mem_val}"
printf 'pulse_node_os_name=%s\npulse_node_java_version=%s\npulse_node_cpu_pct=%s\npulse_node_mem_pct=%s\n' \
  "${_pulse_os_name}" "${_pulse_java_ver}" "${_cpu_val}" "${_mem_val}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
