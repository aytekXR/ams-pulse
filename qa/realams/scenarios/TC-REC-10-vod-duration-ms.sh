#!/usr/bin/env bash
# qa/realams/scenarios/TC-REC-10-vod-duration-ms.sh
#
# TC-REC-10: VoD mp4 recording duration is in milliseconds — billing-unit guard (G-05)
#
# Assertion matrix row:
#   Goal:   Confirm that the 'duration' field in AMS VoD objects returned from
#           GET /rest/v2/vods/list/0/50 is reported in MILLISECONDS, not seconds.
#           A ~30 s recording at 1000 kbps should yield duration ≈ 30000 ms.
#
#   Steps:
#     1. Start RTMP publisher on ${APP}.
#     2. Wait for AMS status=broadcasting (SKIP if not reached in 30 s).
#     3. PUT recording/true?recordType=mp4  (exit 77 if non-200).
#     4. Sleep 30 s to accumulate ~30 s of mp4.
#     5. PUT recording/false to finalise the file.
#     6. stop_publisher — graceful disconnect.
#     7. Poll /vods/list/0/50 for the newest VoD matching STREAM_ID (budget 60 s).
#     8. Assert duration within ±6000 ms of 30000 ms (range 24000..36000).
#     9. Assert duration >= 1000 (not in seconds magnitude; guards G-05).
#
#   Risk:  MEDIUM (creates a permanent mp4 file on the AMS VoD store; no
#          ALLOW_SETTINGS_MUTATION guard needed because per-broadcast recording
#          toggle does NOT touch application settings endpoint).
#
# EXIT CODES
#   0   PASS  — both duration assertions pass
#   1   FAIL  — duration out of range, wrong unit, or VoD not found after 60 s
#   77  SKIP  — stream never reached broadcasting, or recording PUT returned non-200
#
set -euo pipefail

SCENARIO="TC-REC-10"
echo "=== ${SCENARIO}: VoD mp4 recording duration unit (ms not s) ===" >&2

# ── Harness bootstrap ─────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"
# shellcheck source=../harness/publisher.sh
source "${_DIR}/../harness/publisher.sh"

# ── Per-run identifiers ───────────────────────────────────────────────────────
APP="${AMS_APP:-LiveApp}"
STREAM_ID="val-r10-$(openssl rand -hex 4)"

EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ──────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap (set BEFORE start_publisher and any recording toggle) ────────
# Disables recording if left on, then stops the publisher.
_RECORDING_ON=0
cleanup() {
  log "CLEANUP: recording_on=${_RECORDING_ON}"
  if [ "${_RECORDING_ON}" = "1" ]; then
    log "CLEANUP: disabling recording for ${STREAM_ID}"
    curl -s -m 15 -X PUT \
      -b "${AMS_COOKIE_FILE}" \
      "${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}/recording/false" \
      > /dev/null 2>&1 || true
  fi
  log "CLEANUP: stopping publisher ${STREAM_ID}"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  APP=${APP}  AMS_URL=${AMS_URL}  PULSE_URL=${PULSE_URL}"

# ── Cookie precondition ───────────────────────────────────────────────────────
if [ ! -s "${AMS_COOKIE_FILE}" ]; then
  printf 'SKIP\nAMS_COOKIE_FILE missing or empty: %s\n' "${AMS_COOKIE_FILE}" \
    > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Step 1: Start publisher ───────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 1000 kbps on ${APP}"
start_publisher "${STREAM_ID}" "${APP}" 1000

# ── Step 2: Wait for AMS status=broadcasting (precondition; SKIP if not met) ──
log "Polling AMS for status=broadcasting (budget: 30 s, interval: 3 s)"
_ams_status="unknown"
_i=0
while [ "${_i}" -lt 10 ]; do
  _ams_status="$(curl -s -m 10 \
    "${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")"
  if [ "${_ams_status}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( (_i + 1) * 3 )) s"
    break
  fi
  log "AMS status=${_ams_status} (attempt $(( _i + 1 ))/10)"
  sleep 3
  _i=$(( _i + 1 ))
done

if [ "${_ams_status}" != "broadcasting" ]; then
  printf 'SKIP\nStream %s never reached broadcasting (final status=%s)\n' \
    "${STREAM_ID}" "${_ams_status}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi
capture_ams "/${APP}/rest/v2/broadcasts/${STREAM_ID}" "pre-recording"

# ── Step 3: Enable mp4 recording (SKIP if API returns non-200) ────────────────
log "Enabling mp4 recording: PUT ${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}/recording/true?recordType=mp4"
_rec_on_http="$(curl -s -m 20 \
  -o /dev/null \
  -w '%{http_code}' \
  -X PUT \
  -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}/recording/true?recordType=mp4" \
  2>/dev/null || echo "000")"
log "Recording enable HTTP=${_rec_on_http}"
printf 'recording_enable_http=%s\n' "${_rec_on_http}" >> "${EVIDENCE_DIR}/timeline.txt"

if [ "${_rec_on_http}" != "200" ]; then
  printf 'SKIP\nRecording PUT returned HTTP %s — mp4 recording API unavailable or unauthorised\n' \
    "${_rec_on_http}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi
_RECORDING_ON=1
_rec_start_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
log "Recording started at ${_rec_start_ts}"

# ── Step 4: Record for ~30 s ──────────────────────────────────────────────────
log "Recording for 30 s"
sleep 30

# ── Step 5: Disable recording (finalise mp4 file) ────────────────────────────
log "Disabling recording: PUT .../recording/false"
_rec_off_http="$(curl -s -m 20 \
  -o /dev/null \
  -w '%{http_code}' \
  -X PUT \
  -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}/recording/false" \
  2>/dev/null || echo "000")"
log "Recording disable HTTP=${_rec_off_http}"
_RECORDING_ON=0
printf 'recording_disable_http=%s  rec_start=%s\n' "${_rec_off_http}" "${_rec_start_ts}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Step 6: Stop publisher ────────────────────────────────────────────────────
log "Stopping publisher ${STREAM_ID}"
stop_publisher "${STREAM_ID}"

# ── Step 7: Poll AMS vods/list for the newest VoD with our streamId ───────────
# AMS finalises the mp4 file and indexes the VoD after the stream ends.
# Budget: 60 s (12 polls × 5 s interval).
log "Polling AMS /vods/list/0/50 for streamId=${STREAM_ID} (budget: 60 s)"
_vod_dur=-1
_i=0
while [ "${_i}" -lt 12 ]; do
  _vods_raw="$(curl -s -m 20 \
    -b "${AMS_COOKIE_FILE}" \
    "${AMS_URL}/${APP}/rest/v2/vods/list/0/50" 2>/dev/null || echo '[]')"
  printf '%s' "${_vods_raw}" | jq . > "${EVIDENCE_DIR}/ams-vods-list.json" 2>/dev/null || true

  # Handle direct array (typical AMS 3.x) and wrapped object gracefully.
  # Sort by creationDate ascending; take the last entry matching our streamId.
  _vod_dur="$(printf '%s' "${_vods_raw}" | jq --arg sid "${STREAM_ID}" '
    [( if type == "array" then . else [] end )[] | select(.streamId == $sid)]
    | sort_by(.creationDate)
    | if length > 0 then last.duration // -1 else -1 end
  ' 2>/dev/null || echo -1)"

  log "VoD poll attempt $(( _i + 1 ))/12: duration=${_vod_dur}"
  if [ "${_vod_dur}" != "-1" ] && [ "${_vod_dur}" != "null" ]; then
    log "VoD found — duration=${_vod_dur}"
    break
  fi
  sleep 5
  _i=$(( _i + 1 ))
done

printf 'vod_duration=%s\n' "${_vod_dur}" >> "${EVIDENCE_DIR}/timeline.txt"
capture_pulse "/live/streams" "post-stop"

if [ "${_vod_dur}" = "-1" ] || [ "${_vod_dur}" = "null" ]; then
  log "WARNING: no VoD found for ${STREAM_ID} after 60 s — assertions will FAIL"
  _vod_dur=-1
fi

# ── Step 8/9: Assertions ──────────────────────────────────────────────────────
log "ASSERT: VoD duration unit is milliseconds (G-05 billing-unit guard)"
printf 'assert_target=30000  assert_delta=6000  assert_min=1000\n' \
  >> "${EVIDENCE_DIR}/timeline.txt"

# A: duration within ±6000 ms of 30000 ms → passes for 24000..36000 ms
assert_within "${_vod_dur}" 30000 6000 \
  "${SCENARIO} VoD duration in ms range 24000..36000 (recorded ~30 s, 1000 kbps)" || true

# B: duration >= 1000 — confirms value is NOT in seconds magnitude (30 s ≈ 30)
assert_gte "${_vod_dur}" 1000 \
  "${SCENARIO} VoD duration >= 1000 (not seconds-magnitude — G-05 billing-unit guard)" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
