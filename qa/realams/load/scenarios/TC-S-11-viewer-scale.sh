#!/usr/bin/env bash
# qa/realams/load/scenarios/TC-S-11-viewer-scale.sh
#
# TC-S-11: Viewer scale — L-6 parity. WebRTC viewer count is ASSERTED (clean
#          ±10% parity); HLS is RECORDED only (the ~sliding-window inflation is a
#          known AMS semantic characterised by TC-V-10, never a parity target).
#          WebRTC phase runs FIRST so its clean counts are not polluted by HLS.
#
#   Source     : ONE native publisher val-load-v11-<hex>
#   WebRTC gen : official WebRTC Load Test Tool (WEBRTC_TEST_DIR) preferred for scale;
#                native Playwright viewers only for small M (<=8, browsers are heavy)
#   HLS gen    : native real-player HLS viewers (segment-once; honest, no refetch inflation)
#   AMS oracle : GET /${LOAD_APP}/rest/v2/broadcasts/${STREAM}/broadcast-statistics
#                  → .totalWebRTCWatchersCount / .totalHLSWatchersCount
#   Pulse      : GET /live/streams → items[stream_id==STREAM].viewers / .protocol_mix.*
#   Exit       : 0 PASS | 1 FAIL | 77 SKIP
#
set -euo pipefail
SCENARIO="TC-S-11"
echo "=== ${SCENARIO}: viewer scale (WebRTC parity + HLS characterisation) ===" >&2

_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_HARNESS="${_DIR}/../../harness"
if [ ! -f "${_HARNESS}/load-env.sh" ]; then
  echo "SKIP: ${_HARNESS}/load-env.sh not configured (copy load-env.sh.example)" >&2; exit 77
fi
# shellcheck source=/dev/null
source "${_HARNESS}/load-env.sh"
# shellcheck source=/dev/null
source "${_HARNESS}/assert.sh"
# shellcheck source=/dev/null
source "${_HARNESS}/publisher.sh"
# shellcheck source=/dev/null
source "${_HARNESS}/viewer-sim.sh"

EVIDENCE_DIR="${EVIDENCE_ROOT}/LOAD-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"; export EVIDENCE_DIR
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

M="${LOAD_VIEWERS}"
STREAM="val-load-v11-$(openssl rand -hex 3)"
WPGID=""
cleanup() {
  log "CLEANUP"
  stop_publisher "${STREAM}" 2>/dev/null || true
  stop_all_hls_viewers 2>/dev/null || true
  [ -n "${WPGID}" ] && kill -- -"${WPGID}" 2>/dev/null || true
  for _v in $(seq 1 8); do stop_webrtc_viewer "${STREAM}-w${_v}" 2>/dev/null || true; done
}
trap cleanup EXIT
log "STREAM=${STREAM}  M=${M}  AMS=${AMS_URL}  PULSE=${PULSE_URL}"

_stats() { curl -s -m 15 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/${LOAD_APP}/rest/v2/broadcasts/${STREAM}/broadcast-statistics" 2>/dev/null; }
_pulse_viewers() { curl -s -m 15 -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams" 2>/dev/null \
  | jq -r --arg s "${STREAM}" '[(.items // [])[] | select(.stream_id == $s)][0].viewers // 0' 2>/dev/null || echo 0; }

# ── Source publisher ──────────────────────────────────────────────────────────
log "Starting source publisher ${STREAM}"
start_publisher "${STREAM}" "${LOAD_APP}" "${LOAD_PUB_KBPS}"
sleep 10
if [ "$(_stats | jq -r '.totalWebRTCWatchersCount // "na"' 2>/dev/null || echo na)" = "na" ]; then
  log "SKIP: source stream ${STREAM} did not come up (no broadcast-statistics)"
  printf 'SKIP\nSource stream never produced broadcast-statistics.\n' > "${EVIDENCE_DIR}/verdict.txt"; exit 77
fi

# ── Phase 1: WebRTC viewers — parity ASSERTED ────────────────────────────────
if [ -n "${WEBRTC_TEST_DIR}" ] && [ -x "${WEBRTC_TEST_DIR}/run.sh" ]; then
  log "WebRTC gen: official WebRTC Load Test Tool, ${M} players"
  setsid bash -c "cd '${WEBRTC_TEST_DIR}' && ./run.sh -m player -i '${STREAM}' -n '${M}' -s '${LOAD_AMS_HOST}' -u false" \
    >"${EVIDENCE_DIR}/webrtc-tool.log" 2>&1 & WPGID=$!
  sleep 45
  W="$(_stats | jq -r '.totalWebRTCWatchersCount // 0' 2>/dev/null || echo 0)"
  PV="$(_pulse_viewers)"
  LO=$(( M * 90 / 100 )); HI=$(( M * 110 / 100 ))
  log "WebRTC: generated=${M} AMS=${W} Pulse=${PV} (accept ${LO}..${HI})"
  printf 'webrtc_generated=%s ams=%s pulse=%s\n' "${M}" "${W}" "${PV}" >> "${EVIDENCE_DIR}/timeline.txt"
  assert_gte "${W}" "${LO}" "${SCENARIO} L-6 AMS webRTC count >= 0.9·M (${W}>=${LO})" || true
  assert_lte "${W}" "${HI}" "${SCENARIO} L-6 AMS webRTC count <= 1.1·M (${W}<=${HI})" || true
  assert_gte "${PV}" "${LO}" "${SCENARIO} L-6 Pulse viewers >= 0.9·M (${PV}>=${LO})" || true
  kill -- -"${WPGID}" 2>/dev/null || true; WPGID=""; sleep 20
elif [ "${M}" -le 8 ]; then
  log "WebRTC gen: native Playwright viewers ×${M} (small-M path)"
  for _v in $(seq 1 "${M}"); do start_webrtc_viewer "${STREAM}-w${_v}" "${LOAD_APP}"; done
  sleep 45
  W="$(_stats | jq -r '.totalWebRTCWatchersCount // 0' 2>/dev/null || echo 0)"
  PV="$(_pulse_viewers)"
  LO=$(( M * 90 / 100 ))
  log "WebRTC(native): generated=${M} AMS=${W} Pulse=${PV}"
  printf 'webrtc_native_generated=%s ams=%s pulse=%s\n' "${M}" "${W}" "${PV}" >> "${EVIDENCE_DIR}/timeline.txt"
  assert_gte "${W}" "${LO}" "${SCENARIO} L-6 AMS webRTC count >= 0.9·M (${W}>=${LO})" || true
  assert_gte "${PV}" "${LO}" "${SCENARIO} L-6 Pulse viewers >= 0.9·M (${PV}>=${LO})" || true
  for _v in $(seq 1 "${M}"); do stop_webrtc_viewer "${STREAM}-w${_v}" 2>/dev/null || true; done
  sleep 20
else
  log "RECORD: WebRTC parity NOT asserted — set WEBRTC_TEST_DIR (official tool) for M=${M} viewers (native path caps at 8 browsers)"
  printf 'webrtc=SKIPPART (no WEBRTC_TEST_DIR; M=%s > native cap 8)\n' "${M}" >> "${EVIDENCE_DIR}/timeline.txt"
fi

# ── Phase 2: HLS viewers — RECORDED only (known inflation window) ─────────────
log "HLS gen: native real-player viewers ×${M} (recorded, not a parity target)"
for _v in $(seq 1 "${M}"); do start_hls_viewer "${STREAM}" "${LOAD_APP}" "s11-hls-${_v}"; done
sleep 45
H="$(_stats | jq -r '.totalHLSWatchersCount // 0' 2>/dev/null || echo 0)"
PV2="$(_pulse_viewers)"
log "HLS: generated=${M} AMS_hls=${H} Pulse=${PV2} (inflation expected — see TC-V-10)"
printf 'hls_generated=%s ams_hls=%s pulse=%s\n' "${M}" "${H}" "${PV2}" >> "${EVIDENCE_DIR}/timeline.txt"
assert_gte "${H}" 1 "${SCENARIO} L-6 HLS viewers registered on AMS (sanity, not parity)" || true
stop_all_hls_viewers 2>/dev/null || true

log "Writing verdict"
scenario_verdict
exit $?
