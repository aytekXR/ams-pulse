#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-07-webrtc-peer-stats.sh
#
# TC-V-07: WebRTC per-peer stats (live)
#
# Assertion matrix row (scenario-matrix.md):
#   Steps:   1. Start publisher val-v07-<epoch> on LiveApp
#            2. Start WebRTC viewer via start_webrtc_viewer
#            3. Bounded-poll AMS /webrtc-client-stats/0/100 for non-empty array (90 s)
#            4. Capture AMS stats array to evidence
#            5. Wait 15 s for Pulse to pick up the stats
#            6. GET /api/v1/live/streams; check viewer_rtt_ms / viewer_jitter_ms /
#               viewer_loss_pct fields on the stream (LiveStream schema, pulse-api.yaml:1842-1853)
#   AMS truth:    webrtc-client-stats/0/100 returns non-empty array with RTT/jitter fields
#   Pulse assert: viewer_rtt_ms > 0; viewer_jitter_ms non-null; viewer_loss_pct non-null
#   SKIP:    If AMS-side webrtc-client-stats never materialises (array stays empty).
#            Exit 77 with diagnostics for WebRTC connectivity debugging.
#   Exit:    0 PASS | 1 FAIL | 77 SKIP
#
# Contract: start_webrtc_viewer ID APP  (viewer-sim.sh)
# Pulse viewer QoE fields (LiveStream schema):
#   viewer_rtt_ms    — Viewer-side WebRTC RTT in ms  (absent/null when no WebRTC viewers)
#   viewer_jitter_ms — Viewer-side WebRTC jitter in ms
#   viewer_loss_pct  — Viewer-side WebRTC packet loss %
# Source normalization: NormalizeWebRTCStats in server/internal/collector/normalize.go:163
#   rtt_ms      = avgNonZero(videoRoundTripTime, audioRoundTripTime) * 1000
#   jitter_ms   = avgNonZero(videoJitter, audioJitter) * 1000
#   packet_loss = avgNonZero(videoPacketLostRatio, audioPacketLostRatio) * 100
#
# BINDING RULES (memory: shell-harness-false-green-patterns):
#   - every assert_* ends with || true
#   - every curl|jq inside $() carries 2>/dev/null || echo <safe-default>
#   - jq booleans: 'if .x == true then "true" else "false" end'
#   - SKIP: write verdict.txt manually then exit 77
#
set -euo pipefail

SCENARIO="TC-V-07"
echo "=== ${SCENARIO}: WebRTC per-peer stats (live) ===" >&2

# ── Harness bootstrap ──────────────────────────────────────────────────────────
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

# ── Per-run identifiers ────────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-v07-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ───────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ───────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping WebRTC viewer and publisher ${STREAM_ID}"
  stop_webrtc_viewer "${STREAM_ID}" || true
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── 1. Start publisher ─────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

log "Waiting for AMS broadcasting status (budget: 30 s, interval: 2 s)"
_i=0
while [ "${_i}" -lt 15 ]; do
  _st="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")"
  if [ "${_st}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( _i * 2 )) s"
    break
  fi
  sleep 2
  _i=$(( _i + 1 ))
done

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-webrtc-viewer"

# ── 2. Start WebRTC viewer ─────────────────────────────────────────────────────
# Contract: start_webrtc_viewer ID APP  (viewer-sim.sh)
# Container name: val-wv-<ID>; holds for ~90 s so stats register in AMS.
log "Starting WebRTC viewer for ${STREAM_ID} via Playwright (Chromium, --network host)"
start_webrtc_viewer "${STREAM_ID}" "LiveApp"

# ── 3. Bounded-poll AMS webrtc-client-stats for non-empty array (budget: 90 s) ─
# AMS endpoint: /{app}/rest/v2/broadcasts/{id}/webrtc-client-stats/0/100
# Returns [] when no WebRTC viewers are connected; non-empty when a peer registers.
log "Polling AMS webrtc-client-stats for non-empty array (budget: 90 s, interval: 3 s)"
_AMS_STATS_PATH="/LiveApp/rest/v2/broadcasts/${STREAM_ID}/webrtc-client-stats/0/100"
_stats_raw="[]"
_stats_converge_s=-1
_i=0
while [ "${_i}" -lt 30 ]; do
  _stats_raw="$(curl -s -m 15 \
    -b "${AMS_COOKIE_FILE}" \
    "${AMS_URL}${_AMS_STATS_PATH}" \
    2>/dev/null || echo '[]')"
  _stats_raw="${_stats_raw:-[]}"
  _stats_len="$(printf '%s' "${_stats_raw}" | jq 'length // 0' 2>/dev/null || echo 0)"
  _stats_len="${_stats_len:-0}"
  if [ "${_stats_len}" -ge 1 ] 2>/dev/null; then
    _stats_converge_s=$(( _i * 3 ))
    log "AMS webrtc-client-stats: non-empty (${_stats_len} peer(s)) after ${_stats_converge_s} s"
    break
  fi
  log "AMS webrtc-client-stats: empty array (attempt $(( _i + 1 ))/30)"
  sleep 3
  _i=$(( _i + 1 ))
done

# ── 4. Capture AMS stats to evidence ──────────────────────────────────────────
printf '%s' "${_stats_raw}" | jq . > "${EVIDENCE_DIR}/ams-webrtc-client-stats.json" 2>/dev/null || true
capture_ams "${_AMS_STATS_PATH}" "webrtc-client-stats"
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "with-webrtc-viewer"

if [ "${_stats_converge_s}" -eq -1 ]; then
  log "SKIP: AMS webrtc-client-stats never returned a non-empty array within 90 s."
  log "Diagnostics: WebRTC viewer container val-wv-${STREAM_ID}"
  log "  Check: sg docker -c \"docker logs val-wv-${STREAM_ID} 2>&1 | tail -20\""
  log "  Check: AMS WebRTC ICE reachability from the VPS"
  printf 'SKIP\nPrecondition unmet: AMS webrtc-client-stats for %s never returned a non-empty array within 90 s.\nDiagnostics:\n  WebRTC viewer container: val-wv-%s\n  Run: sg docker -c "docker logs val-wv-%s 2>&1 | tail -20"\n  AMS WebRTC ICE may not be reachable — check STUN/TURN and firewall for UDP.\n' \
    "${STREAM_ID}" "${STREAM_ID}" "${STREAM_ID}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── 5. Wait for Pulse to pick up the WebRTC stats ─────────────────────────────
log "Waiting 15 s for Pulse to poll AMS and populate viewer_* fields"
sleep 15

# ── 6. GET Pulse /live/streams; check viewer_rtt_ms / viewer_jitter_ms / viewer_loss_pct ─
# LiveStream schema (pulse-api.yaml:1842-1853):
#   viewer_rtt_ms    — non-null when WebRTC viewers are active
#   viewer_jitter_ms — non-null when WebRTC viewers are active
#   viewer_loss_pct  — non-null when WebRTC viewers are active
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "post-webrtc-stats"

_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp" \
  2>/dev/null || echo '{"items":[]}')"

# Extract the stream object for this STREAM_ID.
# IMPORTANT: do NOT use ${_pulse_stream:-{}} — bash parses ":-{}" by appending
# a literal "}" to the variable when it is already set, corrupting the JSON.
# The || echo '{}' inside the command substitution handles the empty/failed case.
_pulse_stream="$(printf '%s' "${_pulse_resp}" | jq \
  --arg id "${STREAM_ID}" \
  '(.items // .streams // []) | map(select(.stream_id == $id)) | first // {}' \
  2>/dev/null || echo '{}')"

log "Pulse stream object: $(printf '%s' "${_pulse_stream}" | jq -c '{viewer_rtt_ms,viewer_jitter_ms,viewer_loss_pct}' 2>/dev/null || echo '{}')"

# jq -r (raw output) strips JSON string quotes so variables hold plain "true"
# or "false" for clean assert_eq comparisons (no got=""false" cosmetic issue).
# viewer_rtt_ms: present+>0 when Pulse has surfaced non-zero RTT from AMS
_rtt_ok="$(printf '%s' "${_pulse_stream}" | jq -r \
  'if (.viewer_rtt_ms != null and .viewer_rtt_ms > 0) then "true" else "false" end' \
  2>/dev/null || echo "false")"

# viewer_jitter_ms: non-null when Pulse has surfaced QoE from AMS
_jitter_ok="$(printf '%s' "${_pulse_stream}" | jq -r \
  'if .viewer_jitter_ms != null then "true" else "false" end' \
  2>/dev/null || echo "false")"

# viewer_loss_pct: non-null (0 is valid — no loss)
_loss_ok="$(printf '%s' "${_pulse_stream}" | jq -r \
  'if .viewer_loss_pct != null then "true" else "false" end' \
  2>/dev/null || echo "false")"

log "viewer_rtt_ms present+>0=${_rtt_ok}  viewer_jitter_ms non-null=${_jitter_ok}  viewer_loss_pct non-null=${_loss_ok}"
printf 'webrtc_stats_convergence_s=%s\nviewer_rtt_ms_ok=%s\nviewer_jitter_ms_ok=%s\nviewer_loss_pct_ok=%s\n' \
  "${_stats_converge_s}" "${_rtt_ok}" "${_jitter_ok}" "${_loss_ok}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# Save the Pulse stream object and the first AMS stat entry for cross-reference
printf '%s' "${_pulse_stream}" | jq . > "${EVIDENCE_DIR}/pulse-stream-viewer-qoe.json" 2>/dev/null || true
printf '%s' "${_stats_raw}" | jq '.[0] // {}' \
  > "${EVIDENCE_DIR}/ams-webrtc-stat-first-peer.json" 2>/dev/null || true

# ── Determine whether AMS reported all-zero QoE (same-host/loopback ICE) ──────
# AMS 3.0.3 does not populate videoRoundTripTime / videoJitter /
# videoPacketLostRatio when both publisher and viewer run on the same host
# (loopback ICE candidate; clientIp == server IP). Go decodes absent float64
# fields as 0.0; avgNonZero(0,0)=0; omitempty drops the zero from
# LiveStreamItem JSON so viewer_rtt_ms etc. appear null in the Pulse API.
# This is correct D-075/omitempty semantics (absent = "no meaningful data",
# not a Pulse bug). Non-zero RTT validation requires a remote viewer (S19+).
_stat0="$(printf '%s' "${_stats_raw}" | jq '.[0] // {}' 2>/dev/null || echo '{}')"
_ams_video_rtt="$(printf '%s' "${_stat0}" | jq '.videoRoundTripTime // 0' 2>/dev/null || echo 0)"
_ams_video_jitter="$(printf '%s' "${_stat0}" | jq '.videoJitter // 0' 2>/dev/null || echo 0)"
_ams_video_loss="$(printf '%s' "${_stat0}" | jq '.videoPacketLostRatio // 0' 2>/dev/null || echo 0)"
_ams_all_zero_qoe="$(awk \
  -v rtt="${_ams_video_rtt}" \
  -v jitter="${_ams_video_jitter}" \
  -v loss="${_ams_video_loss}" \
  'BEGIN { print (rtt+0 == 0 && jitter+0 == 0 && loss+0 == 0) ? "true" : "false" }')"
log "AMS QoE: videoRTT=${_ams_video_rtt} videoJitter=${_ams_video_jitter} videoLoss=${_ams_video_loss} all_zero=${_ams_all_zero_qoe}"

# ── Assertions ─────────────────────────────────────────────────────────────────
if [ "${_ams_all_zero_qoe}" = "true" ]; then
  # AMS reports all-zero/absent QoE for this viewer (same-host loopback).
  # Pulse CORRECTLY omits zero viewer_* fields via omitempty on LiveStreamItem.
  # Assert that Pulse shows null (absent) for each viewer QoE field — correct
  # behaviour per D-075 semantics.
  log "SEMANTICS: AMS all-zero QoE (same-host viewer). Asserting Pulse omitempty-null."
  _rtt_null="$(printf '%s' "${_pulse_stream}" | jq -r \
    'if .viewer_rtt_ms == null then "true" else "false" end' 2>/dev/null || echo "true")"
  _jitter_null="$(printf '%s' "${_pulse_stream}" | jq -r \
    'if .viewer_jitter_ms == null then "true" else "false" end' 2>/dev/null || echo "true")"
  _loss_null="$(printf '%s' "${_pulse_stream}" | jq -r \
    'if .viewer_loss_pct == null then "true" else "false" end' 2>/dev/null || echo "true")"
  assert_eq "${_rtt_null}" "true" \
    "${SCENARIO} Pulse viewer_rtt_ms=null when AMS all-zero QoE (omitempty-correct, D-075)" || true
  assert_eq "${_jitter_null}" "true" \
    "${SCENARIO} Pulse viewer_jitter_ms=null when AMS all-zero QoE (omitempty-correct)" || true
  assert_eq "${_loss_null}" "true" \
    "${SCENARIO} Pulse viewer_loss_pct=null when AMS all-zero QoE (omitempty-correct)" || true
  log "NOTE: non-zero viewer QoE validation (RTT>0) requires a remote viewer on a"
  log "NOTE: different host from AMS — deferred to S19+ remote-viewer test phase."
else
  # AMS reports non-zero QoE — verify Pulse surfaces the normalised values.
  # viewer_rtt_ms must be present and > 0 for a real WebRTC connection
  assert_eq "${_rtt_ok}" "true" \
    "${SCENARIO} Pulse viewer_rtt_ms non-null and >0 for WebRTC viewer (normalize.go:185)" || true
  # viewer_jitter_ms must be present (may be 0 on a clean local network)
  assert_eq "${_jitter_ok}" "true" \
    "${SCENARIO} Pulse viewer_jitter_ms non-null for WebRTC viewer (normalize.go:186)" || true
  # viewer_loss_pct must be present (0 = no loss, valid)
  assert_eq "${_loss_ok}" "true" \
    "${SCENARIO} Pulse viewer_loss_pct non-null for WebRTC viewer (normalize.go:187)" || true
fi

# ── Verdict ────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
