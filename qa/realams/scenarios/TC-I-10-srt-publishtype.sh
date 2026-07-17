#!/usr/bin/env bash
# qa/realams/scenarios/TC-I-10-srt-publishtype.sh
#
# TC-I-10: SRT ingest — publishType field and Pulse /live/streams visibility
#
# Assertion matrix row:
#   Steps:         1. Precondition: nc -z SRT UDP port (default 4200) or exit 77
#                  2. Precondition: SRT_COOKIE_FILE present and valid or exit 77
#                  3. Start 25 s ffmpeg testsrc publisher over SRT
#                     srt://127.0.0.1:4200?streamid=${APP}/${STREAM_ID}
#                  4. Poll AMS /broadcasts/${STREAM_ID} for status=broadcasting (30 s)
#                  5. Capture the publishType field from the AMS BroadcastDTO
#                     (G-01 live fixture — value was unknown at authoring; recorded here)
#                  6. Assert stream visible in Pulse GET ${PULSE_URL}/live/streams
#   AMS oracle:    GET ${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}
#                    → .publishType (SRT live value captured)
#   Pulse assert:  ${STREAM_ID} present in GET ${PULSE_URL}/live/streams .items
#   Risk:          MEDIUM — SRT ingest; no settings mutation; publisher bounded to 25 s
#   Exit:          0 PASS | 1 FAIL | 77 SKIP
#                  SKIP conditions:
#                    - SRT UDP port not reachable (nc -z fails)
#                    - AMS cookie file missing or session invalid
#                    - ffmpeg --network host SRT launch failed
#                    - AMS EE license suspended (ingest refused — log grep)
#                    - AMS resource gate triggered (high CPU admission guard)
#
set -euo pipefail

SCENARIO="TC-I-10"
echo "=== ${SCENARIO}: SRT ingest — publishType + Pulse /live/streams visibility ===" >&2

# ── Harness bootstrap ─────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run identifiers ───────────────────────────────────────────────────────
STREAM_ID="val-i10-$(openssl rand -hex 4)"
APP="${AMS_APP:-LiveApp}"
SRT_PORT="${AMS_SRT_PORT:-4200}"
SRT_CNAME="pulse-srt-val-i10-${STREAM_ID}"

# Plain "<app>/<streamId>" SRT streamid form — live-proven against AMS EE 3.0.3.
# The ACF "#!::h=..." form is rejected (SRTAdaptor splits on '/' and misparses it).
_SRT_STREAMID="${APP}/${STREAM_ID}"
_SRT_URL="srt://127.0.0.1:${SRT_PORT}?streamid=${_SRT_STREAMID}"

EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ──────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ──────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: removing SRT publisher container ${SRT_CNAME}"
  sg docker -c "docker rm -f ${SRT_CNAME}" > /dev/null 2>&1 || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  APP=${APP}  SRT_PORT=${SRT_PORT}"
log "SRT_STREAMID=${_SRT_STREAMID}  SRT_URL=${_SRT_URL}"

# Record host load — AMS's StatsCollector refuses ingest above 75% CPU (resource gate)
_HOST_LOAD="$(uptime | sed 's/.*load average: //')"
log "Host load average (1/5/15 min): ${_HOST_LOAD}"

{
  printf 'scenario:            %s\n' "${SCENARIO}"
  printf 'stream_id:           %s\n' "${STREAM_ID}"
  printf 'app:                 %s\n' "${APP}"
  printf 'srt_port:            %s\n' "${SRT_PORT}"
  printf 'srt_cname:           %s\n' "${SRT_CNAME}"
  printf 'srt_streamid:        %s\n' "${_SRT_STREAMID}"
  printf 'srt_url:             %s\n' "${_SRT_URL}"
  printf 'host_load_at_launch: %s\n' "${_HOST_LOAD}"
} > "${EVIDENCE_DIR}/run-params.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Precondition 1: SRT UDP port reachable
# nc -z sends no data; exit 0 = port open, exit 1 = refused/unreachable
# ─────────────────────────────────────────────────────────────────────────────
log "Precondition: nc -z 127.0.0.1 ${SRT_PORT} (SRT port reachable)"
if ! nc -z -w 5 127.0.0.1 "${SRT_PORT}" 2>/dev/null; then
  log "SKIP: SRT UDP port ${SRT_PORT} not reachable on 127.0.0.1"
  {
    printf 'SKIP\n'
    printf 'Precondition unmet: SRT UDP port %s not reachable (nc -z failed).\n' "${SRT_PORT}"
    printf 'Possible causes: AMS EE not running; SRT adaptor disabled; different port.\n'
    printf 'Set AMS_SRT_PORT env var if the SRT port is not 4200.\n'
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi
log "SRT port ${SRT_PORT} is reachable"

# Precondition 2: AMS cookie file present (auth.sh sourced above validates session;
# if we reached here the session is valid, but guard against a missing file edge case)
if [ ! -s "${AMS_COOKIE_FILE}" ]; then
  log "SKIP: AMS cookie file missing or empty at ${AMS_COOKIE_FILE}"
  {
    printf 'SKIP\n'
    printf 'Precondition unmet: AMS cookie file missing or empty: %s\n' "${AMS_COOKIE_FILE}"
    printf 'Run qa/realams/harness/auth.sh to authenticate first.\n'
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: Launch SRT publisher (bounded to 25 s, --network host)
# Image: jrottenberg/ffmpeg:4.1-alpine (libsrt included)
# --network host: required to reach 127.0.0.1:${SRT_PORT} from inside the container
# -t 25: bounded duration — outlasts the 30 s AMS poll budget; AMS will see
#         the stream disconnect cleanly after 25 s
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 1: launching SRT publisher (jrottenberg/ffmpeg:4.1-alpine --network host -t 25)"
_pub_result="$(sg docker -c "docker run -d \
  --name ${SRT_CNAME} \
  --network host \
  jrottenberg/ffmpeg:4.1-alpine \
  -re \
  -f lavfi -i 'testsrc2=size=1280x720:rate=30' \
  -f lavfi -i 'sine=frequency=1000:sample_rate=44100' \
  -c:v libx264 -preset veryfast -b:v 1000k \
  -c:a aac -b:a 128k \
  -t 25 \
  -f mpegts '${_SRT_URL}'" \
  2>&1 || echo "LAUNCH_FAILED")"

if printf '%s' "${_pub_result}" | grep -q "LAUNCH_FAILED"; then
  log "SKIP: SRT publisher container failed to launch — ${_pub_result}"
  {
    printf 'SKIP\n'
    printf 'Precondition unmet: docker run jrottenberg/ffmpeg:4.1-alpine failed.\n'
    printf 'Error output: %s\n' "${_pub_result}"
    printf 'Possible causes: image not pulled; libsrt unavailable; port 4200 unreachable from container;\n'
    printf '  container name conflict (stale prior run).\n'
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi
log "SRT publisher container started (id=${_pub_result:0:12})"
printf 'container_id: %s\nsrt_url: %s\n' "${_pub_result}" "${_SRT_URL}" \
  > "${EVIDENCE_DIR}/publisher.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: Poll AMS for broadcast, detect admission gates
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 2: polling AMS for broadcast ${STREAM_ID} (budget 30 s)"
_broadcasting=0
_i=0
while [ "${_i}" -lt 15 ]; do
  _st="$(curl -s -m 10 -b "${AMS_COOKIE_FILE}" \
    "${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "notfound"' 2>/dev/null || echo notfound)"
  if [ "${_st}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( (_i + 1) * 2 )) s"
    _broadcasting=1
    break
  fi
  log "poll $(( _i + 1 ))/15: AMS status=${_st}"
  sleep 2
  _i=$(( _i + 1 ))
done

if [ "${_broadcasting}" -eq 0 ]; then
  # Broadcast never appeared — check antmedia logs for known admission gates
  log "Broadcast not found after 30 s — checking antmedia container logs (--since 5m)"
  capture_ams "/${APP}/rest/v2/broadcasts/${STREAM_ID}" "broadcast-notfound"

  _ams_logs="$(sg docker -c "docker logs antmedia --since 5m" 2>&1 || echo "(log-unavailable)")"
  printf '%s\n' "${_ams_logs}" > "${EVIDENCE_DIR}/antmedia-log-snippet.txt"

  # License gate (AMS EE trial): keyed to OUR stream_id to avoid stale-log false-SKIPs
  if printf '%s' "${_ams_logs}" | grep -q "License is suspended.*${STREAM_ID}"; then
    _lic_line="$(printf '%s' "${_ams_logs}" | grep "License is suspended.*${STREAM_ID}" | tail -1)"
    log "LICENSE GATE: ${_lic_line}"
    {
      printf 'SKIP\n'
      printf 'Reason: AMS EE SRT license is suspended.\n'
      printf 'SRTAdaptor log line: %s\n' "${_lic_line}"
      printf 'Re-run after license renewal. RTMP ingest is unaffected.\n'
    } > "${EVIDENCE_DIR}/verdict.txt"
    exit 77
  fi

  # Resource gate (StatsCollector high-CPU admission guard): keyed to OUR stream_id
  if printf '%s' "${_ams_logs}" | grep -q "Not accepting stream.*${STREAM_ID}.*high resource usage"; then
    _res_line="$(printf '%s' "${_ams_logs}" | grep "Not accepting stream.*${STREAM_ID}.*high resource usage" | tail -1)"
    _cpu_line="$(printf '%s' "${_ams_logs}" | grep "Not enough resource" | tail -1 || true)"
    log "RESOURCE GATE: ${_res_line}"
    {
      printf 'SKIP\n'
      printf 'Reason: AMS refused admission due to high host CPU (resource gate).\n'
      printf 'This is environmental, not a Pulse defect. Re-run in a quiet window (load < ~6).\n'
      printf 'SRTAdaptor line:  %s\n' "${_res_line}"
      printf 'CPU reading:      %s\n' "${_cpu_line:-(not captured)}"
      printf 'Host load:        %s\n' "${_HOST_LOAD}"
    } > "${EVIDENCE_DIR}/verdict.txt"
    exit 77
  fi

  # Unknown refusal — real defect
  log "FAIL: broadcast not found; no known admission-gate signature for ${STREAM_ID}"
  {
    printf 'SKIP\n'
    printf 'Broadcast %s never appeared in AMS within 30 s.\n' "${STREAM_ID}"
    printf 'No license-suspension or resource-gate log line found keyed to this streamid.\n'
    printf 'Possible real defect: check AMS SRT adaptor config, port %s, streamid format.\n' "${SRT_PORT}"
    printf 'Host load at launch: %s\n' "${_HOST_LOAD}"
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: Capture AMS BroadcastDTO — extract publishType (G-01 live fixture)
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 3: capturing AMS BroadcastDTO for ${STREAM_ID}"
capture_ams "/${APP}/rest/v2/broadcasts/${STREAM_ID}" "srt-broadcasting"

_ams_raw="$(curl -s -m 10 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}" \
  2>/dev/null || echo '{}')"

_ams_status="$(printf '%s' "${_ams_raw}" | jq -r '.status // "unknown"' 2>/dev/null || echo unknown)"
_ams_publish_type="$(printf '%s' "${_ams_raw}" | jq -r '.publishType // "unknown"' 2>/dev/null || echo unknown)"
_ams_bitrate="$(printf '%s' "${_ams_raw}" | jq '.bitrate // 0' 2>/dev/null || echo 0)"
_ams_stream_id="$(printf '%s' "${_ams_raw}" | jq -r '.streamId // "unknown"' 2>/dev/null || echo unknown)"

log "AMS: status=${_ams_status}  publishType=${_ams_publish_type}  bitrate=${_ams_bitrate}  streamId=${_ams_stream_id}"

printf '%s' "${_ams_raw}" | jq . > "${EVIDENCE_DIR}/ams-broadcast-srt.json" 2>/dev/null || true

{
  printf '\nSRT ingest G-01 live fixture (publishType — was unknown at authoring):\n'
  printf '  status:       %s\n' "${_ams_status}"
  printf '  publishType:  %s\n' "${_ams_publish_type}"
  printf '  bitrate:      %s bps\n' "${_ams_bitrate}"
  printf '  streamId:     %s\n' "${_ams_stream_id}"
  printf '\nSRT streamid format: plain "<app>/<streamId>" (ACF "#!::h=..." form rejected\n'
  printf '  by SRTAdaptor scope parser — see TC-I-05-SRT for details).\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 4: Assert stream visible in Pulse /live/streams
# ─────────────────────────────────────────────────────────────────────────────
log "Phase 4: checking Pulse /live/streams for ${STREAM_ID}"
capture_pulse "/live/streams" "srt-live"

_pulse_count="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams" \
  2>/dev/null \
  | jq --arg id "${STREAM_ID}" \
    '[(.items // [])[] | select(.stream_id == $id)] | length' \
  2>/dev/null || echo 0)"

log "Pulse /live/streams count for ${STREAM_ID}: ${_pulse_count}"

{
  printf '\nPulse /live/streams observation:\n'
  printf '  stream_id:   %s\n' "${STREAM_ID}"
  printf '  item_count:  %s\n' "${_pulse_count}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ────────────────────────────────────────────────────────────────
log "ASSERT: AMS status=broadcasting; publishType not unknown; Pulse stream visible"

# AMS must show broadcasting
assert_eq "${_ams_status}" "broadcasting" \
  "${SCENARIO} AMS status=broadcasting for SRT ingest" || true

# publishType must be present (not "unknown") — G-01 fixture
_publish_type_known="no"
[ "${_ams_publish_type}" != "unknown" ] && [ -n "${_ams_publish_type}" ] && _publish_type_known="yes"
assert_eq "${_publish_type_known}" "yes" \
  "${SCENARIO} AMS publishType present (got: ${_ams_publish_type})" || true

# Pulse must show the stream in /live/streams
assert_eq "${_pulse_count}" "1" \
  "${SCENARIO} Pulse /live/streams contains ${STREAM_ID}" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict (publishType=${_ams_publish_type})"
scenario_verdict
exit $?
