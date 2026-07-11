#!/usr/bin/env bash
# qa/realams/scenarios/TC-F-06-invalid-stream-key.sh
#
# TC-F-06: Invalid stream key publish — token control rejection
#
# Assertion matrix row:
#   Setup:        Enable publishTokenControlEnabled on pulse-test ONLY (NOT LiveApp).
#                 Save original settings to /tmp/claude-1000/tc-f06-orig-<epoch>.json.
#                 Restore unconditionally in EXIT trap.
#   Steps:        1. GET /rest/v2/applications/settings/pulse-test → save original
#                 2. Flip publishTokenControlEnabled → true and PUT back
#                 3. Publish RTMP to pulse-test/<stream-id> WITHOUT a token
#                 4. Wait 15 s; assert stream never reaches broadcasting on AMS
#                 5. Assert no phantom stream in Pulse /api/v1/live/streams
#   AMS truth:    GET /pulse-test/rest/v2/broadcasts/<id> → 404 (rejected publish)
#   Pulse assert: GET /api/v1/live/streams → stream absent (no phantom)
#   Restore:      PUT original settings back in EXIT trap (unconditional)
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (settings GET/PUT failed; never touch LiveApp)
#
set -euo pipefail

SCENARIO="TC-F-06"
echo "=== ${SCENARIO}: Invalid Stream Key — Token Control Rejection ===" >&2

# ── Harness bootstrap ───────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"
# shellcheck source=../harness/publisher.sh
source "${_DIR}/../harness/publisher.sh"

# ── Per-run identifiers ─────────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-f06-${EPOCH}"
ORIG_SETTINGS_FILE="/tmp/claude-1000/tc-f06-orig-${EPOCH}.json"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Settings restore on exit (CRITICAL: must run unconditionally) ────────────────
_SETTINGS_MODIFIED=0
restore_settings() {
  log "CLEANUP: stopping publisher ${STREAM_ID} (if running)"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
  if [ "${_SETTINGS_MODIFIED}" -eq 1 ] && [ -f "${ORIG_SETTINGS_FILE}" ]; then
    log "CLEANUP: restoring original pulse-test settings from ${ORIG_SETTINGS_FILE}"
    _restore_resp="$(curl -s -m 20 \
      -b "$AMS_COOKIE_FILE" \
      -X PUT \
      -H "Content-Type: application/json" \
      -d "@${ORIG_SETTINGS_FILE}" \
      "${AMS_URL}/rest/v2/applications/settings/pulse-test" 2>/dev/null || echo '{}')"
    _restore_ok="$(printf '%s' "${_restore_resp}" | jq -r '.success // "false"' 2>/dev/null || echo "false")"
    log "CLEANUP: settings restore response: success=${_restore_ok}"
    if [ "${_restore_ok}" != "true" ]; then
      log "CLEANUP WARNING: settings restore may have failed — response: ${_restore_resp}"
    fi
  fi
}
trap restore_settings EXIT

log "STREAM_ID=${STREAM_ID}  orig_settings=${ORIG_SETTINGS_FILE}"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: GET current pulse-test settings and save original
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 1: fetching current pulse-test settings"
_orig_settings="$(curl -s -m 20 \
  -b "$AMS_COOKIE_FILE" \
  "${AMS_URL}/rest/v2/applications/settings/pulse-test" 2>/dev/null || echo '')"

if [ -z "${_orig_settings}" ]; then
  log "SKIP: could not fetch pulse-test settings (empty response)"
  {
    echo "SKIP"
    echo "Precondition unmet: GET ${AMS_URL}/rest/v2/applications/settings/pulse-test returned empty."
    echo "Cannot enable token control without existing settings baseline."
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# Verify it looks like a settings object (must have appName field)
_settings_appname="$(printf '%s' "${_orig_settings}" | jq -r '.appName // ""' 2>/dev/null || echo "")"
if [ -z "${_settings_appname}" ]; then
  log "SKIP: settings response does not look like AppSettings JSON: ${_orig_settings:0:200}"
  {
    echo "SKIP"
    echo "Precondition unmet: GET settings for pulse-test returned non-settings JSON."
    echo "Response preview: ${_orig_settings:0:200}"
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# Save original to /tmp/claude-1000 (not gitignored evidence dir, for cross-trap access)
printf '%s' "${_orig_settings}" > "${ORIG_SETTINGS_FILE}"
# Also save to evidence
printf '%s' "${_orig_settings}" | jq . > "${EVIDENCE_DIR}/pulse-test-settings-original.json" \
  2>/dev/null || printf '%s' "${_orig_settings}" > "${EVIDENCE_DIR}/pulse-test-settings-original.json"

# Confirm current token control state (read the actual field name from the JSON)
_orig_token_ctrl="$(printf '%s' "${_orig_settings}" | \
  jq 'if .publishTokenControlEnabled == true then "true" else "false" end' \
  2>/dev/null || echo "false")"
log "Original pulse-test publishTokenControlEnabled: ${_orig_token_ctrl}"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: Enable publishTokenControlEnabled and PUT back
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 2: enabling publishTokenControlEnabled on pulse-test"

# Flip the flag using jq; keep ALL other settings intact
_modified_settings="$(printf '%s' "${_orig_settings}" | \
  jq '.publishTokenControlEnabled = true' 2>/dev/null || echo '')"

if [ -z "${_modified_settings}" ]; then
  log "SKIP: jq failed to modify settings JSON"
  {
    echo "SKIP"
    echo "Precondition unmet: jq could not set publishTokenControlEnabled=true on settings."
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# Save modified settings to evidence
printf '%s' "${_modified_settings}" | jq . > "${EVIDENCE_DIR}/pulse-test-settings-modified.json" \
  2>/dev/null || true

_put_resp="$(curl -s -m 20 \
  -b "$AMS_COOKIE_FILE" \
  -X PUT \
  -H "Content-Type: application/json" \
  -d "${_modified_settings}" \
  "${AMS_URL}/rest/v2/applications/settings/pulse-test" 2>/dev/null || echo '{}')"

_put_ok="$(printf '%s' "${_put_resp}" | jq -r '.success // "false"' 2>/dev/null || echo "false")"
log "Settings PUT response: success=${_put_ok}"
printf '%s' "${_put_resp}" | jq . > "${EVIDENCE_DIR}/settings-put-response.json" 2>/dev/null || true

if [ "${_put_ok}" != "true" ]; then
  log "SKIP: settings PUT did not return success=true — response: ${_put_resp}"
  {
    echo "SKIP"
    echo "Precondition unmet: PUT settings/pulse-test to enable publishTokenControlEnabled failed."
    echo "PUT response: ${_put_resp}"
    echo "Cannot test token rejection without enabling token control."
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# Mark settings as modified so EXIT trap restores them
_SETTINGS_MODIFIED=1
log "publishTokenControlEnabled=true on pulse-test — AMS may need up to 5 s to apply"
sleep 5

# Verify the flag was actually applied
_verify_resp="$(curl -s -m 20 \
  -b "$AMS_COOKIE_FILE" \
  "${AMS_URL}/rest/v2/applications/settings/pulse-test" 2>/dev/null || echo '{}')"
_applied_flag="$(printf '%s' "${_verify_resp}" | \
  jq 'if .publishTokenControlEnabled == true then "true" else "false" end' \
  2>/dev/null || echo "false")"
log "Verified applied publishTokenControlEnabled=${_applied_flag} on pulse-test"

if [ "${_applied_flag}" != "true" ]; then
  log "SKIP: token control flag not reflected in settings after PUT"
  {
    echo "SKIP"
    echo "Precondition unmet: publishTokenControlEnabled=true not visible in GET after PUT."
    echo "AMS may have rejected or silently ignored the settings update."
    echo "Verify response: ${_verify_resp:0:300}"
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: Publish WITHOUT a token — should be rejected by AMS
# ─────────────────────────────────────────────────────────────────────────────
# When publishTokenControlEnabled=true, AMS requires a valid publish token in
# the RTMP URL (as a query param ?token=<valid_token>). Publishing without one
# should be rejected at the RTMP handshake/connect level and the stream
# should never appear in the broadcasts list.
#
# We publish to pulse-test (not LiveApp) so rejection is scoped to the test app.
log "Phase 3: publishing to pulse-test WITHOUT a token (expect rejection)"
start_publisher "${STREAM_ID}" "pulse-test" 500

# Wait 15 s — this is the full tolerance window; a rejected stream should never
# appear even after the timeout.
log "Waiting 15 s — rejected publish should never reach broadcasting"
sleep 15

# ─────────────────────────────────────────────────────────────────────────────
# Phase 4: Assert stream absent from AMS and Pulse
# ─────────────────────────────────────────────────────────────────────────────
capture_ams "/pulse-test/rest/v2/broadcasts/${STREAM_ID}" "post-rejected-ams"

_ams_poll_body="/tmp/claude-1000/ams-f06-poll-$$.json"
_ams_http_code="$(curl -s -m 10 \
  -o "${_ams_poll_body}" \
  -w '%{http_code}' \
  -b "$AMS_COOKIE_FILE" \
  "${AMS_URL}/pulse-test/rest/v2/broadcasts/${STREAM_ID}" 2>/dev/null || echo 000)"

if [ "${_ams_http_code}" = "404" ]; then
  _ams_stream_status="absent-404"
else
  _ams_stream_status="$(jq -r '.status // "present"' "${_ams_poll_body}" 2>/dev/null || echo "present")"
fi
rm -f "${_ams_poll_body}"

log "AMS pulse-test/${STREAM_ID} http=${_ams_http_code} status=${_ams_stream_status}"

# Check Pulse /live/streams — should not contain the rejected stream
capture_pulse "/live/streams" "post-rejected-pulse"
_pulse_stream_count="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams" \
  | jq --arg id "${STREAM_ID}" '[(.items // [])[] | select(.stream_id == $id)] | length' \
  2>/dev/null || echo 99)"
_pulse_stream_count="${_pulse_stream_count:-99}"

log "Pulse /live/streams count for ${STREAM_ID}: ${_pulse_stream_count}"
{
  printf 'AMS broadcast status after rejected publish: http=%s status=%s\n' \
    "${_ams_http_code}" "${_ams_stream_status}"
  printf 'Pulse /live/streams count for %s: %s\n' "${STREAM_ID}" "${_pulse_stream_count}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ───────────────────────────────────────────────────────────────────

# AMS must NOT show the stream as broadcasting (absent or non-broadcasting)
_ams_is_broadcasting="$([ "${_ams_stream_status}" = "broadcasting" ] && echo "true" || echo "false")"
assert_eq "${_ams_is_broadcasting}" "false" \
  "${SCENARIO} AMS: stream ${STREAM_ID} NOT broadcasting after tokenless publish (status=${_ams_stream_status})" || true

# Pulse must not have a phantom entry
assert_eq "${_pulse_stream_count}" "0" \
  "${SCENARIO} Pulse: no phantom stream for ${STREAM_ID} in /live/streams" || true

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict (EXIT trap will restore pulse-test settings)"
scenario_verdict
exit $?
