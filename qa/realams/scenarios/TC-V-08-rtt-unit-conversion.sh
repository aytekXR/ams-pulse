#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-08-rtt-unit-conversion.sh
#
# TC-V-08: Unit conversion check — RTT seconds → ms; jitter and loss scaling
#
# Assertion matrix row (scenario-matrix.md):
#   Steps:   1. Start publisher val-v08-<epoch> on LiveApp (independent of TC-V-07)
#            2. Start WebRTC viewer via start_webrtc_viewer
#            3. Bounded-poll AMS webrtc-client-stats for non-empty entry (90 s)
#            4. Extract raw AMS videoRoundTripTime (seconds), audioRoundTripTime,
#               videoJitter, audioJitter, videoPacketLostRatio, audioPacketLostRatio
#            5. Compute Pulse-expected values using normalize.go:163-190 formulas:
#               rtt_ms   = avgNonZero(videoRTT, audioRTT) * 1000
#               jitter   = avgNonZero(videoJitter, audioJitter) * 1000
#               loss_pct = avgNonZero(videoLoss, audioLoss) * 100
#            6. Wait 15 s for Pulse to poll AMS and propagate
#            7. Read Pulse viewer_rtt_ms from GET /api/v1/live/streams
#            8. Assert pulse_rtt_ms ≈ expected_ms within ±20%
#               (jitter between samples; AMS RTT and Pulse poll cycle may differ)
#   AMS truth:    videoRoundTripTime in seconds (e.g. 0.025 s = 25 ms expected)
#   Pulse assert: viewer_rtt_ms ≈ AMS raw RTT * 1000  (± 20%)
#   SKIP:    If AMS webrtc-client-stats never materialises (empty array in 90 s).
#   Exit:    0 PASS | 1 FAIL | 77 SKIP
#
# Unit conversion reference (normalize.go:163-190):
#   AMS raw field             | Unit          | Pulse field      | Transform
#   videoRoundTripTime        | seconds (s)   | viewer_rtt_ms    | × 1000
#   videoJitter               | seconds (s)   | viewer_jitter_ms | × 1000
#   videoPacketLostRatio      | fraction 0..1 | viewer_loss_pct  | × 100
#   avgNonZero(a, b): returns (a+b)/2 if both > 0; a if only a > 0; b if only b > 0; 0 otherwise
#
# BINDING RULES (memory: shell-harness-false-green-patterns):
#   - every assert_* ends with || true
#   - every curl|jq inside $() carries 2>/dev/null || echo <safe-default>
#   - jq booleans: 'if .x == true then "true" else "false" end'
#   - SKIP: write verdict.txt manually then exit 77
#
set -euo pipefail

SCENARIO="TC-V-08"
echo "=== ${SCENARIO}: RTT unit conversion — AMS seconds → Pulse ms ===" >&2

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
STREAM_ID="val-v08-${EPOCH}"
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

# ── avgNonZero helper (mirrors normalize.go:319-333) ──────────────────────────
# Usage: _result=$(avg_non_zero "$a" "$b")
avg_non_zero() {
  awk -v a="$1" -v b="$2" 'BEGIN {
    if (a > 0 && b > 0) { printf "%.6f", (a + b) / 2 }
    else if (a > 0)     { printf "%.6f", a }
    else if (b > 0)     { printf "%.6f", b }
    else                { printf "%.6f", 0 }
  }'
}

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

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-viewer"

# ── 2. Start WebRTC viewer ─────────────────────────────────────────────────────
log "Starting WebRTC viewer for ${STREAM_ID} via Playwright"
start_webrtc_viewer "${STREAM_ID}" "LiveApp"

# ── 3. Bounded-poll AMS webrtc-client-stats for non-empty array (90 s) ────────
_AMS_STATS_PATH="/LiveApp/rest/v2/broadcasts/${STREAM_ID}/webrtc-client-stats/0/100"
log "Polling AMS webrtc-client-stats for non-empty array (budget: 90 s, interval: 3 s)"
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
    log "AMS webrtc-client-stats: ${_stats_len} peer(s) after ${_stats_converge_s} s"
    break
  fi
  log "AMS webrtc-client-stats: empty (attempt $(( _i + 1 ))/30)"
  sleep 3
  _i=$(( _i + 1 ))
done

# Save AMS raw stats to evidence
printf '%s' "${_stats_raw}" | jq . > "${EVIDENCE_DIR}/ams-webrtc-client-stats-raw.json" 2>/dev/null || true

if [ "${_stats_converge_s}" -eq -1 ]; then
  log "SKIP: AMS webrtc-client-stats never returned a non-empty array within 90 s."
  log "Cannot perform unit conversion check without AMS ground truth."
  printf 'SKIP\nPrecondition unmet: AMS webrtc-client-stats for %s never returned a non-empty array within 90 s.\nWebRTC viewer container: val-wv-%s\nCheck ICE connectivity: sg docker -c "docker logs val-wv-%s 2>&1 | tail -20"\n' \
    "${STREAM_ID}" "${STREAM_ID}" "${STREAM_ID}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── 4. Extract AMS raw unit values from first peer entry ─────────────────────
_stat0="$(printf '%s' "${_stats_raw}" | jq '.[0] // {}' 2>/dev/null || echo '{}')"
_stat0="${_stat0:-{}}"

_ams_video_rtt="$(printf '%s' "${_stat0}" | jq '.videoRoundTripTime // 0' 2>/dev/null || echo 0)"
_ams_audio_rtt="$(printf '%s' "${_stat0}" | jq '.audioRoundTripTime // 0' 2>/dev/null || echo 0)"
_ams_video_jitter="$(printf '%s' "${_stat0}" | jq '.videoJitter // 0' 2>/dev/null || echo 0)"
_ams_audio_jitter="$(printf '%s' "${_stat0}" | jq '.audioJitter // 0' 2>/dev/null || echo 0)"
_ams_video_loss="$(printf '%s' "${_stat0}" | jq '.videoPacketLostRatio // 0' 2>/dev/null || echo 0)"
_ams_audio_loss="$(printf '%s' "${_stat0}" | jq '.audioPacketLostRatio // 0' 2>/dev/null || echo 0)"

# Apply avgNonZero (mirrors normalize.go:171-173)
_avg_rtt_s="$(avg_non_zero "${_ams_video_rtt:-0}" "${_ams_audio_rtt:-0}")"
_avg_jitter_s="$(avg_non_zero "${_ams_video_jitter:-0}" "${_ams_audio_jitter:-0}")"
_avg_loss_ratio="$(avg_non_zero "${_ams_video_loss:-0}" "${_ams_audio_loss:-0}")"

# Compute expected Pulse values (normalize.go:185-187):
#   rtt_ms      = avg_rtt_s * 1000
#   jitter_ms   = avg_jitter_s * 1000
#   loss_pct    = avg_loss_ratio * 100
_expected_rtt_ms="$(awk -v v="${_avg_rtt_s:-0}" 'BEGIN { printf "%.3f", v * 1000 }')"
_expected_jitter_ms="$(awk -v v="${_avg_jitter_s:-0}" 'BEGIN { printf "%.3f", v * 1000 }')"
_expected_loss_pct="$(awk -v v="${_avg_loss_ratio:-0}" 'BEGIN { printf "%.3f", v * 100 }')"

log "AMS raw: videoRTT=${_ams_video_rtt}s  audioRTT=${_ams_audio_rtt}s"
log "AMS raw: videoJitter=${_ams_video_jitter}s  audioJitter=${_ams_audio_jitter}s"
log "AMS raw: videoLossRatio=${_ams_video_loss}  audioLossRatio=${_ams_audio_loss}"
log "Expected Pulse (after normalize.go conversion): rtt_ms=${_expected_rtt_ms}  jitter_ms=${_expected_jitter_ms}  loss_pct=${_expected_loss_pct}"

# Record conversion evidence to timeline
{
  printf 'ams_video_rtt_s=%s  ams_audio_rtt_s=%s  avg_rtt_s=%s  expected_rtt_ms=%s\n' \
    "${_ams_video_rtt}" "${_ams_audio_rtt}" "${_avg_rtt_s}" "${_expected_rtt_ms}"
  printf 'ams_video_jitter_s=%s  ams_audio_jitter_s=%s  expected_jitter_ms=%s\n' \
    "${_ams_video_jitter}" "${_ams_audio_jitter}" "${_expected_jitter_ms}"
  printf 'ams_video_loss=%s  ams_audio_loss=%s  expected_loss_pct=%s\n' \
    "${_ams_video_loss}" "${_ams_audio_loss}" "${_expected_loss_pct}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# Serialize conversion record to evidence JSON
jq -n \
  --arg stream_id "${STREAM_ID}" \
  --argjson video_rtt "${_ams_video_rtt:-0}" \
  --argjson audio_rtt "${_ams_audio_rtt:-0}" \
  --argjson avg_rtt "${_avg_rtt_s:-0}" \
  --argjson exp_rtt_ms "${_expected_rtt_ms:-0}" \
  --argjson video_jitter "${_ams_video_jitter:-0}" \
  --argjson audio_jitter "${_ams_audio_jitter:-0}" \
  --argjson exp_jitter_ms "${_expected_jitter_ms:-0}" \
  --argjson video_loss "${_ams_video_loss:-0}" \
  --argjson audio_loss "${_ams_audio_loss:-0}" \
  --argjson exp_loss_pct "${_expected_loss_pct:-0}" \
  '{
    stream_id: $stream_id,
    ams_raw: {
      videoRoundTripTime_s: $video_rtt, audioRoundTripTime_s: $audio_rtt,
      videoJitter_s: $video_jitter, audioJitter_s: $audio_jitter,
      videoPacketLostRatio: $video_loss, audioPacketLostRatio: $audio_loss
    },
    normalize_go_expected: {
      rtt_ms: $exp_rtt_ms,
      jitter_ms: $exp_jitter_ms,
      loss_pct: $exp_loss_pct
    },
    formula: "avgNonZero(video,audio)*1000 for RTT/jitter; avgNonZero(video,audio)*100 for loss"
  }' > "${EVIDENCE_DIR}/unit-conversion-record.json" 2>/dev/null || true

# ── 5. Wait for Pulse to poll AMS and propagate viewer stats ──────────────────
log "Waiting 15 s for Pulse poll cycle (poll interval ≤5 s; 15 s is 3 cycles)"
sleep 15

# ── 6. Read Pulse viewer_rtt_ms from /live/streams ────────────────────────────
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "post-stats"

_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp" \
  2>/dev/null || echo '{"items":[]}')"

_pulse_stream="$(printf '%s' "${_pulse_resp}" | jq \
  --arg id "${STREAM_ID}" \
  '(.items // .streams // []) | map(select(.stream_id == $id)) | first // {}' \
  2>/dev/null || echo '{}')"
_pulse_stream="${_pulse_stream:-{}}"

_pulse_rtt_ms="$(printf '%s' "${_pulse_stream}" | jq '.viewer_rtt_ms // 0' 2>/dev/null || echo 0)"
_pulse_rtt_ms="${_pulse_rtt_ms:-0}"

_pulse_jitter_ms="$(printf '%s' "${_pulse_stream}" | jq '.viewer_jitter_ms // 0' 2>/dev/null || echo 0)"
_pulse_jitter_ms="${_pulse_jitter_ms:-0}"

_pulse_loss_pct="$(printf '%s' "${_pulse_stream}" | jq '.viewer_loss_pct // 0' 2>/dev/null || echo 0)"
_pulse_loss_pct="${_pulse_loss_pct:-0}"

log "Pulse: viewer_rtt_ms=${_pulse_rtt_ms}  viewer_jitter_ms=${_pulse_jitter_ms}  viewer_loss_pct=${_pulse_loss_pct}"
log "Expected: rtt_ms=${_expected_rtt_ms}  jitter_ms=${_expected_jitter_ms}  loss_pct=${_expected_loss_pct}"

printf '%s' "${_pulse_stream}" | jq . > "${EVIDENCE_DIR}/pulse-stream-qoe.json" 2>/dev/null || true

printf 'pulse_viewer_rtt_ms=%s  expected=%s\n' "${_pulse_rtt_ms}" "${_expected_rtt_ms}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── 7. Assertions ──────────────────────────────────────────────────────────────
# Primary: RTT unit conversion — ±20% tolerance for inter-sample jitter
# (AMS and Pulse sample at different moments; the conversion formula is the invariant)
assert_approx "${_pulse_rtt_ms}" "${_expected_rtt_ms}" 20 \
  "${SCENARIO} Pulse viewer_rtt_ms ≈ AMS avgNonZero(videoRTT,audioRTT)×1000 (±20% normalize.go:185)" || true

# Record-only: jitter and loss conversion (logged but not strict-asserted
# since both may be 0 in a clean local network — cannot assert approx at b=0)
log "Conversion record: jitter AMS=${_avg_jitter_s}s → expected ${_expected_jitter_ms}ms, Pulse=${_pulse_jitter_ms}ms"
log "Conversion record: loss  AMS=${_avg_loss_ratio} → expected ${_expected_loss_pct}%, Pulse=${_pulse_loss_pct}%"

# Weak assertions on jitter and loss: only check non-negative (valid range)
assert_gte "${_pulse_jitter_ms}" 0 \
  "${SCENARIO} Pulse viewer_jitter_ms >=0 (normalize.go:186: ×1000)" || true
assert_gte "${_pulse_loss_pct}" 0 \
  "${SCENARIO} Pulse viewer_loss_pct >=0 (normalize.go:187: ×100)" || true

# ── Verdict ────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
