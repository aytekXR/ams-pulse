#!/usr/bin/env bash
# qa/realams/scenarios/TC-H-04-bitrate-floor-alert.sh
#
# TC-H-04: Alert rule — ingest_bitrate_floor lt 99999 fires for live stream
#
# Assertion matrix row:
#   Steps:     1. Start publisher val-h04-<epoch> on LiveApp at 2000 kbps
#              2. Create alert rule metric=ingest_bitrate_floor op=lt threshold=99999
#                 scoped to val-h04 stream (scope.app + scope.stream_id)
#              3. Poll /alerts/history?rule_id=<id>&state=firing ≤30 s
#              4. Assert firing row appears; record observed convergence time
#   AMS truth: Publisher ingest_bitrate_floor ≈ 2000 kbps < 99999 kbps → rule fires
#   Pulse assert: /alerts/history row with state=firing for this rule_id
#   Tolerance:  30 s (evaluator tick ≤5 s per e2e.yml A1; ~4 s observed in CI)
#   Exit:       0 PASS | 1 FAIL | 77 SKIP (publisher never reached broadcasting)
#   Channels:   none (history-only assert per TC-H-04 spec)
#
set -euo pipefail

SCENARIO="TC-H-04"
echo "=== ${SCENARIO}: Alert — ingest_bitrate_floor lt 99999 fires ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"
# shellcheck source=../harness/publisher.sh
source "${_DIR}/../harness/publisher.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-h04-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

RULE_ID=""

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping publisher ${STREAM_ID}"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
  if [ -n "${RULE_ID}" ]; then
    log "CLEANUP: deleting alert rule ${RULE_ID}"
    curl -s -m 10 -X DELETE \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/alerts/rules/${RULE_ID}" > /dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Step 1: Start publisher at 2000 kbps ─────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

# Wait for AMS to show broadcasting (≤30 s, 3 s interval)
log "Polling AMS for broadcasting status (budget: 30 s)"
_ams_status="unknown"
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_status="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "curl_error")"
  if [ "${_ams_status}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( (_i + 1) * 3 )) s"
    break
  fi
  log "AMS status=${_ams_status} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-rule-ams"

if [ "${_ams_status}" != "broadcasting" ]; then
  log "SKIP: AMS stream never reached broadcasting — cannot test bitrate-floor alert"
  printf 'SKIP\nPrecondition unmet: AMS stream %s never reached broadcasting.\nFinal AMS status: %s\n' \
    "${STREAM_ID}" "${_ams_status}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Step 2: Create alert rule (no channel — history-only) ────────────────────
log "Creating alert rule: metric=ingest_bitrate_floor op=lt threshold=99999 scope=${STREAM_ID}"
_rule_body="{\"name\":\"val-h04-rule-${EPOCH}\",\"metric\":\"ingest_bitrate_floor\",\"operator\":\"lt\",\"threshold\":99999,\"window_s\":0,\"cooldown_s\":1,\"severity\":\"warning\",\"scope\":{\"app\":\"LiveApp\",\"stream_id\":\"${STREAM_ID}\"}}"
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
  log "FAIL: rule creation returned HTTP ${_rule_http} or empty id"
  assert_eq "${_rule_http}" "201" "${SCENARIO} alert rule created (HTTP 201)" || true
  scenario_verdict
  exit 1
fi

# ── Step 3: Poll /alerts/history for firing row (≤30 s, 2 s interval) ────────
log "Polling /alerts/history?rule_id=${RULE_ID}&state=firing (budget: 30 s, 2 s interval)"
_fire_conv_s=999
_history_item=""
_i=0
while [ "${_i}" -lt 15 ]; do
  sleep 2
  _hist_resp="$(curl -s -m 15 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/alerts/history?rule_id=${RULE_ID}&state=firing" \
    2>/dev/null || echo '{}')"
  _history_item="$(printf '%s' "${_hist_resp}" | \
    jq '.items[0] // empty' 2>/dev/null || true)"
  if [ -n "${_history_item}" ]; then
    _fire_conv_s=$(( (_i + 1) * 2 ))
    log "Firing history row found after ${_fire_conv_s} s"
    break
  fi
  log "No firing row yet (attempt $(( _i + 1 ))/15, elapsed $(( (_i + 1) * 2 )) s)"
  _i=$(( _i + 1 ))
done

capture_pulse "/alerts/history?rule_id=${RULE_ID}&state=firing" "alert-history"

# Save firing row detail
if [ -n "${_history_item}" ]; then
  printf '%s' "${_history_item}" | jq . > "${EVIDENCE_DIR}/firing-row.json" 2>/dev/null || true
  _fire_state="$(printf '%s' "${_history_item}" | jq -r '.state // "absent"' 2>/dev/null || echo "absent")"
  _fire_metric="$(printf '%s' "${_history_item}" | jq -r '.metric // "absent"' 2>/dev/null || echo "absent")"
  _fire_value="$(printf '%s' "${_history_item}" | jq -r '.value // "absent"' 2>/dev/null || echo "absent")"
  log "Firing row: state=${_fire_state}  metric=${_fire_metric}  value=${_fire_value}"
else
  _fire_state="absent"
  _fire_metric="absent"
  _fire_value="absent"
  log "FAIL: no firing row in /alerts/history within 30 s"
fi

# Record convergence in timeline
printf 'rule_id=%s\nfiring_convergence_s=%s\nfiring_metric=%s\nfiring_value=%s\n' \
  "${RULE_ID}" "${_fire_conv_s}" "${_fire_metric}" "${_fire_value}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ───────────────────────────────────────────────────────────────
log "ASSERT: firing row state=firing within ≤30 s budget"
assert_eq "${_fire_state}" "firing" "${SCENARIO} alert history row state=firing" || true
assert_eq "${_fire_metric}" "ingest_bitrate_floor" "${SCENARIO} alert history metric=ingest_bitrate_floor" || true
assert_lte "${_fire_conv_s}" 30 "${SCENARIO} firing convergence ≤30 s (observed: ${_fire_conv_s} s)" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
