#!/usr/bin/env bash
# qa/realams/scenarios/TC-H-05-viewer-count-alert.sh
#
# TC-H-05: Alert rule — viewer_count gt 0 fires when HLS viewer joins
#
# Assertion matrix row:
#   Steps:     1. Start publisher val-h05-<epoch> on LiveApp
#              2. Create alert rule metric=viewer_count op=gt threshold=0
#                 scoped to val-h05 stream
#              3. Start 1 HLS curl-loop viewer
#              4. Poll /alerts/history?rule_id=<id>&state=firing ≤60 s
#              5. Assert firing row appears; record convergence
#   AMS truth: hlsViewerCount >= 1 once HLS segments fetched
#   Pulse assert: /alerts/history row with state=firing for this rule_id
#   Tolerance:  60 s (HLS segment-based count lag per scenario-matrix §Viewer Count Tolerance)
#   Exit:       0 PASS | 1 FAIL | 77 SKIP (publisher never reached broadcasting)
#
set -euo pipefail

SCENARIO="TC-H-05"
echo "=== ${SCENARIO}: Alert — viewer_count gt 0 with HLS viewer ===" >&2

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
# shellcheck source=../harness/viewer-sim.sh
source "${_DIR}/../harness/viewer-sim.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-h05-${EPOCH}"
VIEWER_ID="hls-h05-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

RULE_ID=""

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping HLS viewer ${VIEWER_ID}"
  stop_hls_viewer "${VIEWER_ID}" 2>/dev/null || true
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

log "STREAM_ID=${STREAM_ID}  VIEWER_ID=${VIEWER_ID}  PULSE_URL=${PULSE_URL}"

# ── Step 1: Start publisher ───────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 1000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 1000

# Wait for AMS broadcasting (≤30 s, 3 s interval)
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

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-viewer-ams"

if [ "${_ams_status}" != "broadcasting" ]; then
  log "SKIP: AMS stream never reached broadcasting — cannot test viewer_count alert"
  printf 'SKIP\nPrecondition unmet: AMS stream %s never reached broadcasting.\nFinal AMS status: %s\n' \
    "${STREAM_ID}" "${_ams_status}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Step 2: Create alert rule ─────────────────────────────────────────────────
log "Creating alert rule: metric=viewer_count op=gt threshold=0 scope=${STREAM_ID}"
_rule_body="{\"name\":\"val-h05-rule-${EPOCH}\",\"metric\":\"viewer_count\",\"operator\":\"gt\",\"threshold\":0,\"window_s\":0,\"cooldown_s\":1,\"severity\":\"info\",\"scope\":{\"app\":\"LiveApp\",\"stream_id\":\"${STREAM_ID}\"}}"
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

# ── Step 3: Start 1 HLS viewer ────────────────────────────────────────────────
# HLS is served at /<app>/streams/<id>.m3u8 (S17 correction: flat URL, no /playlist.m3u8)
log "Starting HLS viewer ${VIEWER_ID} for stream ${STREAM_ID}"
start_hls_viewer "${STREAM_ID}" "LiveApp" "${VIEWER_ID}"

# ── Step 4: Poll /alerts/history for firing row (≤60 s, 3 s interval) ────────
log "Polling /alerts/history?rule_id=${RULE_ID}&state=firing (budget: 60 s, 3 s interval)"
_fire_conv_s=999
_history_item=""
_i=0
while [ "${_i}" -lt 20 ]; do
  sleep 3
  _hist_resp="$(curl -s -m 15 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/alerts/history?rule_id=${RULE_ID}&state=firing" \
    2>/dev/null || echo '{}')"
  _history_item="$(printf '%s' "${_hist_resp}" | \
    jq '.items[0] // empty' 2>/dev/null || true)"
  if [ -n "${_history_item}" ]; then
    _fire_conv_s=$(( (_i + 1) * 3 ))
    log "Firing history row found after ${_fire_conv_s} s"
    break
  fi
  log "No firing row yet (attempt $(( _i + 1 ))/20, elapsed $(( (_i + 1) * 3 )) s)"
  _i=$(( _i + 1 ))
done

capture_pulse "/alerts/history?rule_id=${RULE_ID}&state=firing" "alert-history"

# Save firing row
if [ -n "${_history_item}" ]; then
  printf '%s' "${_history_item}" | jq . > "${EVIDENCE_DIR}/firing-row.json" 2>/dev/null || true
  _fire_state="$(printf '%s' "${_history_item}" | jq -r '.state // "absent"' 2>/dev/null || echo "absent")"
  _fire_metric="$(printf '%s' "${_history_item}" | jq -r '.metric // "absent"' 2>/dev/null || echo "absent")"
  _fire_value="$(printf '%s' "${_history_item}" | jq '.value // -1' 2>/dev/null || echo -1)"
  log "Firing row: state=${_fire_state}  metric=${_fire_metric}  value=${_fire_value}"
else
  _fire_state="absent"
  _fire_metric="absent"
  _fire_value="-1"
  log "FAIL: no firing row in /alerts/history within 60 s"
fi

printf 'rule_id=%s\nfiring_convergence_s=%s\nfiring_metric=%s\nfiring_value=%s\n' \
  "${RULE_ID}" "${_fire_conv_s}" "${_fire_metric}" "${_fire_value}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── AMS ground truth capture ──────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "post-viewer-ams"
_ams_hls="$(curl -s -m 10 \
  "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
  | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
log "AMS hlsViewerCount=${_ams_hls}"
printf 'ams_hls_viewer_count=%s\n' "${_ams_hls}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ───────────────────────────────────────────────────────────────
assert_eq "${_fire_state}" "firing" "${SCENARIO} alert history row state=firing" || true
assert_eq "${_fire_metric}" "viewer_count" "${SCENARIO} alert history metric=viewer_count" || true
assert_lte "${_fire_conv_s}" 60 "${SCENARIO} firing convergence ≤60 s (HLS lag budget; observed: ${_fire_conv_s} s)" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
