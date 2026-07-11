#!/usr/bin/env bash
# qa/realams/scenarios/TC-WH-01-webhook-audit.sh
#
# TC-WH-01: Webhook audit — no events arriving
#
# Assertion matrix row:
#   Steps:        1. Audit pulse-realams-pulse-1 logs for webhook deliveries
#                 2. Audit pulse-prod-pulse-1 logs (read-only, never stop/kill)
#   AMS truth:    AMS 3.0.3 cannot HMAC-sign webhook hooks (O3 decision, unsigned)
#   Pulse assert: Pulse webhook listener logs zero successful deliveries;
#                 all rejected with HMAC validation error OR simply not configured.
#   Exit:         0 PASS | 1 FAIL
#
# Evidence produced:
#   - realams-webhook-log-lines.txt  — all webhook-related log lines (realams)
#   - prod-webhook-log-lines.txt     — all webhook-related log lines (prod, read-only)
#   - verdict: zero accepted deliveries expected on both stacks
#
set -euo pipefail

SCENARIO="TC-WH-01"
echo "=== ${SCENARIO}: Webhook audit — no events arriving ===" >&2

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

log "auditing webhook log lines in both stacks"

# ── Audit pulse-realams-pulse-1 ──────────────────────────────────────────────
log "Reading pulse-realams-pulse-1 logs"
_realams_logs="$(sg docker -c "docker logs pulse-realams-pulse-1 2>&1" || true)"

# Extract all webhook-related lines
printf '%s' "${_realams_logs}" | grep -iE 'webhook|POST /webhook|hmac|listenerHookURL|vodReady|streamPublish' \
  > "${EVIDENCE_DIR}/realams-webhook-log-lines.txt" 2>/dev/null || true
_realams_wh_lines="$(wc -l < "${EVIDENCE_DIR}/realams-webhook-log-lines.txt" | tr -d ' ')"
log "realams webhook log lines: ${_realams_wh_lines}"

# Count accepted deliveries — look for lines indicating a successful webhook receipt
_realams_accepted="$(printf '%s' "${_realams_logs}" \
  | grep -ciE '(webhook.*accepted|webhook.*ok|webhook.*200|accepted.*webhook|delivery.*success)' \
  2>/dev/null || echo 0)"
log "realams accepted webhook deliveries: ${_realams_accepted}"
printf 'realams_webhook_log_lines=%s\nrealams_accepted_deliveries=%s\n' \
  "${_realams_wh_lines}" "${_realams_accepted}" >> "${EVIDENCE_DIR}/timeline.txt"

assert_eq "${_realams_accepted}" "0" "${SCENARIO} realams: zero accepted webhook deliveries (AMS 3.0.3 unsigned)" || true

# ── Audit pulse-prod-pulse-1 (READ-ONLY — never stop/kill) ──────────────────
log "Reading pulse-prod-pulse-1 logs (READ-ONLY)"
_prod_logs="$(sg docker -c "docker logs pulse-prod-pulse-1 2>&1" 2>/dev/null || true)"

if [ -z "${_prod_logs}" ]; then
  log "WARNING: no logs from pulse-prod-pulse-1 (container may not be named pulse-prod-pulse-1)"
  printf 'prod_log_access=unavailable\n' >> "${EVIDENCE_DIR}/timeline.txt"
else
  printf '%s' "${_prod_logs}" | grep -iE 'webhook|POST /webhook|hmac|listenerHookURL|vodReady|streamPublish' \
    > "${EVIDENCE_DIR}/prod-webhook-log-lines.txt" 2>/dev/null || true
  _prod_wh_lines="$(wc -l < "${EVIDENCE_DIR}/prod-webhook-log-lines.txt" | tr -d ' ')"
  log "prod webhook log lines: ${_prod_wh_lines}"

  _prod_accepted="$(printf '%s' "${_prod_logs}" \
    | grep -ciE '(webhook.*accepted|webhook.*ok|webhook.*200|accepted.*webhook|delivery.*success)' \
    2>/dev/null || echo 0)"
  log "prod accepted webhook deliveries: ${_prod_accepted}"

  printf 'prod_webhook_log_lines=%s\nprod_accepted_deliveries=%s\n' \
    "${_prod_wh_lines}" "${_prod_accepted}" >> "${EVIDENCE_DIR}/timeline.txt"

  assert_eq "${_prod_accepted}" "0" "${SCENARIO} prod: zero accepted webhook deliveries (AMS 3.0.3 unsigned)" || true
fi

# Document the root cause
{
  printf 'ROOT CAUSE:\n'
  printf '  AMS 3.0.3 cannot HMAC-sign listenerHookURL deliveries (no HMAC fields in AMS 3.0.3).\n'
  printf '  Per O3 decision: webhook events from AMS are rejected or not configured.\n'
  printf '  Poll path (REST poller) provides lifecycle event coverage instead.\n'
  printf '  See: agents/decisions.md O3, scenario-matrix.md TC-WH-01\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
