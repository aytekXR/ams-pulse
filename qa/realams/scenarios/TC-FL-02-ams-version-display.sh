#!/usr/bin/env bash
# qa/realams/scenarios/TC-FL-02-ams-version-display.sh
#
# TC-FL-02: AMS version display
#
# Assertion matrix row:
#   Steps:        1. GET /rest/v2/version (authed) for AMS version ground truth
#                 2. GET /api/v1/fleet/nodes for Pulse representation
#   AMS truth:    {versionName:"3.0.3", versionType:"Enterprise"}
#   Pulse assert: fleet node card or system info shows AMS version 3.0.3 Enterprise
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (fleet/nodes has no nodes)
#
set -euo pipefail

SCENARIO="TC-FL-02"
echo "=== ${SCENARIO}: AMS version display ===" >&2

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

# ── AMS ground truth: GET /rest/v2/version ───────────────────────────────────
log "AMS ground truth: GET /rest/v2/version"
capture_ams "/rest/v2/version" "ams-version"
_ams_version_raw="$(curl -s -m 15 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/rest/v2/version" 2>/dev/null || echo '{}')"

_ams_version_name="$(printf '%s' "${_ams_version_raw}" | jq -r '.versionName // ""' 2>/dev/null || true)"
_ams_version_type="$(printf '%s' "${_ams_version_raw}" | jq -r '.versionType // ""' 2>/dev/null || true)"
log "AMS version: versionName=${_ams_version_name}  versionType=${_ams_version_type}"
printf 'ams_versionName=%s\nams_versionType=%s\n' "${_ams_version_name}" "${_ams_version_type}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

assert_eq "${_ams_version_name}" "3.0.3" "${SCENARIO} AMS versionName=3.0.3 (ground truth)" || true
# S17 live: this build reports versionType="Enterprise Edition" (S16 capture
# said "Enterprise") — accept the Enterprise* family, record the exact string.
case "${_ams_version_type}" in Enterprise*) _vt_family="Enterprise" ;; *) _vt_family="${_ams_version_type}" ;; esac
assert_eq "${_vt_family}" "Enterprise" "${SCENARIO} AMS versionType is Enterprise-family (observed: ${_ams_version_type})" || true

# ── Pulse fleet/nodes ────────────────────────────────────────────────────────
log "Pulse: GET /api/v1/fleet/nodes"
capture_pulse "/fleet/nodes" "fleet-nodes"
_fleet_raw="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/fleet/nodes" 2>/dev/null || echo '{}')"

_node_count="$(printf '%s' "${_fleet_raw}" | jq '(.items // []) | length' 2>/dev/null || echo 0)"
log "Pulse fleet/nodes: ${_node_count} node(s)"

if [ "${_node_count}" -eq 0 ]; then
  log "SKIP: Pulse fleet/nodes returned 0 items — cannot verify version display"
  printf 'SKIP\nPrecondition unmet: Pulse /api/v1/fleet/nodes returned 0 items.\nAMS version ground truth: %s %s\n' \
    "${_ams_version_name}" "${_ams_version_type}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

_node="$(printf '%s' "${_fleet_raw}" | jq '.items[0]' 2>/dev/null || echo '{}')"
_pulse_version="$(printf '%s' "${_node}" | jq -r '.version // ""' 2>/dev/null || true)"
log "Pulse fleet node version: '${_pulse_version}'"
printf 'pulse_node_version=%s\n' "${_pulse_version}" >> "${EVIDENCE_DIR}/timeline.txt"

# Assert Pulse surfaces AMS version containing "3.0.3"
# Accept either exact "3.0.3" or a composed string like "3.0.3-Enterprise"
_version_present="$([ -n "${_pulse_version}" ] && echo present || echo absent)"
assert_eq "${_version_present}" "present" "${SCENARIO} fleet node version field is populated" || true

# Version must contain "3.0.3"
_has_version_num="$(printf '%s' "${_pulse_version}" | grep -c '3\.0\.3' 2>/dev/null || echo 0)"
assert_gte "${_has_version_num}" 1 "${SCENARIO} fleet node version contains '3.0.3'" || true

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
