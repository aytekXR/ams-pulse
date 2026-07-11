#!/usr/bin/env bash
# qa/realams/harness/capture.sh
#
# Evidence capture helpers for scenario scripts.
# SOURCE this file after env.sh (requires PULSE_URL, AMS_URL, PULSE_TOKEN,
# AMS_COOKIE_FILE, and EVIDENCE_DIR from the calling scenario).
#
# Functions:
#   capture_ams   ENDPOINT LABEL   — timestamped AMS REST snapshot to EVIDENCE_DIR
#   capture_pulse ENDPOINT LABEL   — timestamped Pulse API snapshot to EVIDENCE_DIR
#   compare_viewer_count STREAM_ID APP
#                                  — per-stream AMS-vs-Pulse viewer count parity;
#                                    clamps negative rtmpViewerCount to 0;
#                                    uses assert_within with ±2 tolerance per
#                                    scenario-matrix.md "Viewer Count Tolerance".
#                                    Requires assert.sh to be sourced and EVIDENCE_DIR set.
#
set -euo pipefail

# ── capture_ams ENDPOINT LABEL ────────────────────────────────────────────────
# ENDPOINT: path relative to AMS_URL, e.g. /LiveApp/rest/v2/broadcasts/list/0/10
# Writes:
#   ${EVIDENCE_DIR}/ams-${LABEL}-${HH}${MM}${SS}.json
#   ${EVIDENCE_DIR}/ams-${LABEL}-${HH}${MM}${SS}.json.headers
capture_ams() {
  local endpoint="$1"
  local label="$2"
  local ts
  ts="$(date -u +%H%M%S)"
  local outfile="${EVIDENCE_DIR}/ams-${label}-${ts}.json"

  curl -s -m 20 \
    -b "$AMS_COOKIE_FILE" \
    -D "${outfile}.headers" \
    "${AMS_URL}${endpoint}" \
    | jq . > "$outfile" 2>/dev/null || {
      # Write raw response if jq fails (non-JSON body)
      curl -s -m 20 -b "$AMS_COOKIE_FILE" "${AMS_URL}${endpoint}" \
        > "$outfile" 2>/dev/null || true
    }

  echo "[capture] AMS  ${label} → ${outfile}" >&2
}

# ── capture_pulse ENDPOINT LABEL ──────────────────────────────────────────────
# ENDPOINT: path relative to PULSE_URL, e.g. /live/overview
# Writes:
#   ${EVIDENCE_DIR}/pulse-${LABEL}-${HH}${MM}${SS}.json
#   ${EVIDENCE_DIR}/pulse-${LABEL}-${HH}${MM}${SS}.json.headers
capture_pulse() {
  local endpoint="$1"
  local label="$2"
  local ts
  ts="$(date -u +%H%M%S)"
  local outfile="${EVIDENCE_DIR}/pulse-${label}-${ts}.json"

  curl -s -m 20 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    -D "${outfile}.headers" \
    "${PULSE_URL}${endpoint}" \
    | jq . > "$outfile" 2>/dev/null || {
      curl -s -m 20 \
        -H "Authorization: Bearer ${PULSE_TOKEN}" \
        "${PULSE_URL}${endpoint}" \
        > "$outfile" 2>/dev/null || true
    }

  echo "[capture] Pulse ${label} → ${outfile}" >&2
}

# ── compare_viewer_count STREAM_ID APP ────────────────────────────────────────
#
# AMS ground truth (inline broadcast fields — NOT the BroadcastStatistics
# endpoint, which is dead code Pulse never calls):
#   hlsViewerCount + webRTCViewerCount + max(0, rtmpViewerCount) + dashViewerCount
#
# RTMP pull viewers: AMS reports rtmpViewerCount = -1 on some streams
# (totalRTMPWatchersCount dead-code quirk). Clamp to 0 per scenario-matrix.md.
#
# Pulse ground truth from GET /api/v1/live/streams:
#   LiveStreamList.items[stream_id == STREAM_ID].viewers
#   (OpenAPI schema: LiveStream.viewers = integer, total viewer count)
#   Per-protocol breakdown available via LiveStream.protocol_mix.{webrtc,hls,rtmp,dash}
#
# Asserts within ±2 (absolute) per scenario-matrix.md tolerance window.
# Requires assert.sh sourced and EVIDENCE_DIR set.
#
compare_viewer_count() {
  local stream_id="$1"
  local app="${2:-LiveApp}"

  # ── AMS side ──────────────────────────────────────────────────────────────
  local ams_raw
  ams_raw="$(curl -s -m 15 -b "$AMS_COOKIE_FILE" \
    "${AMS_URL}/${app}/rest/v2/broadcasts/${stream_id}" 2>/dev/null || echo '{}')"

  local ams_vc
  ams_vc="$(printf '%s' "$ams_raw" | jq '
    ((.hlsViewerCount // 0) | if . < 0 then 0 else . end) +
    ((.webRTCViewerCount // 0) | if . < 0 then 0 else . end) +
    ((.rtmpViewerCount // 0) | if . < 0 then 0 else . end) +
    ((.dashViewerCount // 0) | if . < 0 then 0 else . end)
  ' 2>/dev/null || echo 0)"

  local ams_hls ams_webrtc ams_rtmp ams_dash
  ams_hls="$(printf '%s' "$ams_raw" | jq '(.hlsViewerCount // 0) | if . < 0 then 0 else . end' 2>/dev/null || echo 0)"
  ams_webrtc="$(printf '%s' "$ams_raw" | jq '(.webRTCViewerCount // 0) | if . < 0 then 0 else . end' 2>/dev/null || echo 0)"
  ams_rtmp="$(printf '%s' "$ams_raw" | jq '(.rtmpViewerCount // 0) | if . < 0 then 0 else . end' 2>/dev/null || echo 0)"
  ams_dash="$(printf '%s' "$ams_raw" | jq '(.dashViewerCount // 0) | if . < 0 then 0 else . end' 2>/dev/null || echo 0)"

  # ── Pulse side ────────────────────────────────────────────────────────────
  # GET /api/v1/live/streams returns LiveStreamList: {items: [LiveStream]}
  # LiveStream fields: stream_id, viewers (total), protocol_mix.{webrtc,hls,rtmp,dash}
  local pulse_raw
  pulse_raw="$(curl -s -m 15 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/live/streams" 2>/dev/null || echo '{"items":[]}')"

  local pulse_vc pulse_hls pulse_webrtc pulse_rtmp pulse_dash
  pulse_vc="$(printf '%s' "$pulse_raw" | jq \
    --arg id "$stream_id" \
    '(.items // []) | map(select(.stream_id == $id)) | first | .viewers // 0' \
    2>/dev/null || echo 0)"
  pulse_hls="$(printf '%s' "$pulse_raw" | jq \
    --arg id "$stream_id" \
    '(.items // []) | map(select(.stream_id == $id)) | first | .protocol_mix.hls // 0' \
    2>/dev/null || echo 0)"
  pulse_webrtc="$(printf '%s' "$pulse_raw" | jq \
    --arg id "$stream_id" \
    '(.items // []) | map(select(.stream_id == $id)) | first | .protocol_mix.webrtc // 0' \
    2>/dev/null || echo 0)"
  pulse_rtmp="$(printf '%s' "$pulse_raw" | jq \
    --arg id "$stream_id" \
    '(.items // []) | map(select(.stream_id == $id)) | first | .protocol_mix.rtmp // 0' \
    2>/dev/null || echo 0)"
  pulse_dash="$(printf '%s' "$pulse_raw" | jq \
    --arg id "$stream_id" \
    '(.items // []) | map(select(.stream_id == $id)) | first | .protocol_mix.dash // 0' \
    2>/dev/null || echo 0)"

  # ── Log breakdown ─────────────────────────────────────────────────────────
  echo "[compare:${stream_id}] AMS   total=${ams_vc}  hls=${ams_hls}  webrtc=${ams_webrtc}  rtmp=${ams_rtmp}  dash=${ams_dash}" >&2
  echo "[compare:${stream_id}] Pulse total=${pulse_vc}  hls=${pulse_hls}  webrtc=${pulse_webrtc}  rtmp=${pulse_rtmp}  dash=${pulse_dash}" >&2

  # ── Save per-compare snapshot ─────────────────────────────────────────────
  local ts
  ts="$(date -u +%H%M%S)"
  local snap="${EVIDENCE_DIR}/compare-vc-${stream_id}-${ts}.json"
  jq -n \
    --arg sid "$stream_id" \
    --argjson ams_total "$ams_vc" \
    --argjson ams_hls "$ams_hls" \
    --argjson ams_webrtc "$ams_webrtc" \
    --argjson ams_rtmp "$ams_rtmp" \
    --argjson ams_dash "$ams_dash" \
    --argjson pulse_total "$pulse_vc" \
    --argjson pulse_hls "$pulse_hls" \
    --argjson pulse_webrtc "$pulse_webrtc" \
    --argjson pulse_rtmp "$pulse_rtmp" \
    --argjson pulse_dash "$pulse_dash" \
    '{
      stream_id: $sid,
      ams:   {total: $ams_total,   hls: $ams_hls,   webrtc: $ams_webrtc,   rtmp: $ams_rtmp,   dash: $ams_dash},
      pulse: {total: $pulse_total, hls: $pulse_hls, webrtc: $pulse_webrtc, rtmp: $pulse_rtmp, dash: $pulse_dash},
      delta: ($pulse_total - $ams_total)
    }' > "$snap" 2>/dev/null || true
  echo "[compare:${stream_id}] snapshot → ${snap}" >&2

  # ── Assert total viewer count within ±2 (scenario-matrix tolerance) ───────
  # assert_within requires assert.sh to be sourced
  assert_within "$pulse_vc" "$ams_vc" 2 "viewer_count[${stream_id}] Pulse vs AMS (±2 tol)"
}
