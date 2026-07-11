#!/usr/bin/env bash
# qa/realams/scenarios/TC-H-02-healthz.sh
#
# TC-H-02: Healthz endpoint
#
# Assertion matrix row:
#   Steps:        1. GET /healthz on the realams Pulse target (unauthenticated)
#   AMS truth:    N/A
#   Pulse assert: HTTP 200 with {status:ok};
#                 components.clickhouse.status=ok
#                 components.meta_store.status=ok
#                 components.collector.status=ok
#   Exit:         0 PASS | 1 FAIL
#
# Note: /healthz is at the root path (not under /api/v1) per harness env.sh
#       PULSE_HEALTH_URL exports the correct URL.
#
set -euo pipefail

SCENARIO="TC-H-02"
echo "=== ${SCENARIO}: Healthz endpoint ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
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

log "healthz_url=${PULSE_HEALTH_URL}"

# ── GET /healthz ─────────────────────────────────────────────────────────────
log "GET ${PULSE_HEALTH_URL}"
_outfile="${EVIDENCE_DIR}/pulse-healthz.json"
_http_code="$(curl -s -m 15 \
  -o "${_outfile}" \
  -w '%{http_code}' \
  "${PULSE_HEALTH_URL}" 2>/dev/null || echo 0)"
log "HTTP status: ${_http_code}"

# Parse JSON body
_body="$(jq . "${_outfile}" 2>/dev/null || cat "${_outfile}" 2>/dev/null || echo '{}')"
printf '%s' "${_body}" | jq . > "${_outfile}" 2>/dev/null || true

log "Body: $(printf '%s' "${_body}" | jq -c '.' 2>/dev/null || echo 'parse_error')"

# ── Assertions ───────────────────────────────────────────────────────────────
assert_eq "${_http_code}" "200" "${SCENARIO} /healthz returns HTTP 200" || true

_status="$(printf '%s' "${_body}" | jq -r '.status // ""' 2>/dev/null || true)"
assert_eq "${_status}" "ok" "${SCENARIO} /healthz status=ok" || true

_ch_status="$(printf '%s' "${_body}" | jq -r '.components.clickhouse.status // ""' 2>/dev/null || true)"
assert_eq "${_ch_status}" "ok" "${SCENARIO} components.clickhouse.status=ok" || true

_ms_status="$(printf '%s' "${_body}" | jq -r '.components.meta_store.status // ""' 2>/dev/null || true)"
assert_eq "${_ms_status}" "ok" "${SCENARIO} components.meta_store.status=ok" || true

_col_status="$(printf '%s' "${_body}" | jq -r '.components.collector.status // ""' 2>/dev/null || true)"
assert_eq "${_col_status}" "ok" "${SCENARIO} components.collector.status=ok" || true

log "status=${_status}  clickhouse=${_ch_status}  meta_store=${_ms_status}  collector=${_col_status}"
printf 'http_code=%s\nstatus=%s\nclickhouse=%s\nmeta_store=%s\ncollector=%s\n' \
  "${_http_code}" "${_status}" "${_ch_status}" "${_ms_status}" "${_col_status}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
