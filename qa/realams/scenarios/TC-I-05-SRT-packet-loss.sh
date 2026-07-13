#!/usr/bin/env bash
# qa/realams/scenarios/TC-I-05-SRT-packet-loss.sh
#
# TC-I-05-SRT: SRT ingest packet loss — protocol semantics + license gate
#
# PURPOSE:
#   Validate SRT ingest metrics via AMS BroadcastDTO when AMS EE SRT ingest
#   is available (license valid). Documents packetLostRatio semantics for SRT
#   as the DG-18 variant complement to TC-I-05 (RTMP/TCP semantics, S18).
#
# S29/D-091 FIRST RUN STATUS (2026-07-13):
#   AMS EE license is suspended (lapsed 2026-07-12T12:09Z; first enforcement
#   delta observed 2026-07-13 20:57:47Z). SRTAdaptor rejects ingest with:
#     "License is suspended. Not accepting the stream"
#   This scenario detects that condition and SKIP-exits 77 with evidence.
#   RTMP ingest is unaffected by the lapse (confirmed separately).
#
# OBSERVATION FRAMING (when license is valid — future run):
#   SRT's ARQ (Automatic Repeat reQuest) at srtReceiveLatencyInMs=150 ms can
#   fully repair netem-injected packet loss before AMS sees the stream.
#   A packetLostRatio=0 post-ARQ is a VALID observation — it is NOT a metric
#   gap or broken instrumentation. Assertions are structural only:
#     1. broadcast status=broadcasting
#     2. bitrate > 0 (ingest is flowing)
#   The following are recorded as observations (any value is valid):
#     - packetLostRatio  (post-ARQ loss; may be 0 even under transport loss)
#     - packetsLost      (cumulative post-ARQ lost packet count)
#     - publishType      (SRT live value unknown at authoring; recorded here)
#     - Pulse packet_loss_pct (derived from AMS packetLostRatio × 100)
#
# NETEM / LOSS INJECTION NOTE:
#   This publisher runs with --network host, sharing the VPS host network.
#   Applying tc netem to the host eth0 NIC is FORBIDDEN: it shapes ALL traffic
#   on the VPS including prod Caddy, Pulse polling, and other tenants.
#
#   Loss injection for SRT requires a BRIDGE-NETWORK publisher instead:
#     1. Run publisher without --network host (gets 172.17.x.x Docker bridge IP)
#     2. Point SRT at the Docker bridge gateway: srt://172.17.0.1:4200?streamid=...
#        (172.17.0.1 is the default bridge gateway; AMS on --network host listens
#        on all interfaces including the bridge-side address on port 4200)
#     3. Apply netem on the publisher container's eth0 via a NET_ADMIN sidecar:
#           sg docker -c "docker run -d \
#             --name <sidecar> \
#             --net container:<pub-container> \
#             --cap-add NET_ADMIN \
#             alpine:3 sh -c \
#             'apk add --no-cache iproute2 && tc qdisc add dev eth0 root netem loss 10% && sleep 40'"
#   This bridge-path variant is NOT implemented here to keep the primary run
#   simple. It is a follow-on exercise using the TC-I-05 netem sidecar pattern.
#
# LICENSE GATE (implemented below):
#   After starting the publisher, if the broadcast does NOT appear in AMS REST
#   within 30 s, check antmedia container logs (last 5 min) for "License is suspended":
#     → FOUND:     SKIP exit 77 — expected first-run outcome (S29/D-091)
#     → NOT FOUND: FAIL exit 1  — real defect; SRT should be accepted without licence block
#
# Exit codes:
#   0   PASS — broadcast established; structural assertions pass (license valid)
#   1   FAIL — SRT ingest rejected for unknown reason (not a licence error)
#   77  SKIP — AMS EE licence suspended gates SRT ingest
#
set -euo pipefail

SCENARIO="TC-I-05-SRT"
echo "=== ${SCENARIO}: SRT ingest — packet-loss semantics + license gate ===" >&2

# ── Harness bootstrap ─────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run identifiers ───────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-i05-srt-${EPOCH}"
SRT_CNAME="pulse-srt-val-i05-${EPOCH}"

# ACF streamid format confirmed vs AMS logs.
# Variable set before use; '#' inside double-quotes is literal, not a comment.
_SRT_STREAMID="#!::h=LiveApp/${STREAM_ID},m=publish"
_SRT_URL="srt://127.0.0.1:4200?streamid=${_SRT_STREAMID}"

EVIDENCE_DIR="${EVIDENCE_ROOT}/S29-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ──────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap — always remove the SRT publisher container on EXIT ──────────
# Runs on every exit path (PASS / FAIL / SKIP / signal).
# docker rm -f is idempotent: safe even if the container never started.
cleanup() {
  log "CLEANUP: removing SRT publisher ${SRT_CNAME} (if present)"
  sg docker -c "docker rm -f ${SRT_CNAME}" > /dev/null 2>&1 || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  SRT_CNAME=${SRT_CNAME}"
log "SRT_URL=${_SRT_URL}"
log "ACF streamid: ${_SRT_STREAMID}"
printf 'scenario: %s\nstream_id: %s\nsrt_cname: %s\nsrt_url: %s\n' \
  "${SCENARIO}" "${STREAM_ID}" "${SRT_CNAME}" "${_SRT_URL}" \
  > "${EVIDENCE_DIR}/run-params.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: Launch SRT publisher
# ─────────────────────────────────────────────────────────────────────────────
# Image: jrottenberg/ffmpeg:4.1-alpine (confirmed locally present; libsrt built in).
# --network host: publisher shares the VPS host network namespace so it can
#   reach 127.0.0.1:4200 (AMS SRT listen address) without bridge-NAT translation.
#
# The ACF streamid '#!::h=...' is double-quoted inside the inner sh -c command
# (via escaped quotes \"...\") so /bin/sh does not interpret '#' as a comment.
#
# -t 50: bounded publisher duration (slightly longer than the 30 s poll window).
# -f mpegts: standard MPEG-TS container for SRT transport.
#
log "Phase 1: launching SRT publisher (jrottenberg/ffmpeg:4.1-alpine --network host)"
_pub_result="$(sg docker -c "docker run -d \
  --name ${SRT_CNAME} \
  --network host \
  jrottenberg/ffmpeg:4.1-alpine \
  -re \
  -f lavfi -i 'testsrc2=size=1280x720:rate=30' \
  -f lavfi -i 'sine=frequency=1000:sample_rate=44100' \
  -c:v libx264 -preset veryfast -b:v 1000k \
  -c:a aac -b:a 128k \
  -t 50 \
  -f mpegts \"${_SRT_URL}\"" \
  2>&1 || echo "LAUNCH_FAILED")"

if printf '%s' "${_pub_result}" | grep -q "LAUNCH_FAILED"; then
  log "SKIP: SRT publisher container failed to launch — ${_pub_result}"
  {
    echo "SKIP"
    printf 'Blocker: docker run jrottenberg/ffmpeg:4.1-alpine -f mpegts %s failed.\n' "${_SRT_URL}"
    printf 'Error output: %s\n' "${_pub_result}"
    printf 'Possible causes: libsrt not found in image; port 4200 unreachable; container name conflict.\n'
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

log "SRT publisher container started (id=${_pub_result:0:12})"
printf 'publisher_container_id: %s\nsrt_url: %s\n' "${_pub_result}" "${_SRT_URL}" \
  > "${EVIDENCE_DIR}/publisher.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: License gate — poll AMS for broadcast; check for license rejection
# ─────────────────────────────────────────────────────────────────────────────
#
# If the AMS EE license is suspended, SRTAdaptor rejects the SRT connect at
# the protocol level and logs "License is suspended. Not accepting the stream".
# The broadcast never appears in the AMS REST API.
#
# Poll budget: 30 s (15 × 2 s). A legitimate SRT ingest should appear within
# this window once the publisher connects and AMS processes the stream.
#
log "Phase 2: polling AMS for broadcast ${STREAM_ID} (budget: 30 s)"
_broadcasting=0
_i=0
while [ "${_i}" -lt 15 ]; do
  _st="$(curl -s -m 10 -b "${AMS_COOKIE_FILE}" \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "notfound"' 2>/dev/null || echo "notfound")"
  if [ "${_st}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( _i * 2 )) s"
    _broadcasting=1
    break
  fi
  log "poll ${_i}: AMS status=${_st}"
  sleep 2
  _i=$(( _i + 1 ))
done

if [ "${_broadcasting}" -eq 0 ]; then
  # Broadcast never appeared — check for license suspension in antmedia logs.
  # "License is suspended" without the streamid: AMS logs the rejection before
  # completing stream registration, so the log line may not contain the stream ID.
  log "Broadcast not found after 30 s — checking antmedia container logs (--since 5m)"

  capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "broadcast-notfound"

  _ams_logs="$(sg docker -c "docker logs antmedia --since 5m" 2>&1 || echo "(log-unavailable)")"
  printf '%s\n' "${_ams_logs}" > "${EVIDENCE_DIR}/antmedia-log-snippet.txt"
  log "antmedia logs → ${EVIDENCE_DIR}/antmedia-log-snippet.txt"

  # V3 must-fix (S29): filter by OUR streamid — a stale rejection line from a
  # prior SRT attempt inside the log window must NOT SKIP-mask an unrelated
  # failure (that is a FAIL, not a license SKIP). AMS logs the rejected
  # streamid verbatim in the rejection line.
  if printf '%s' "${_ams_logs}" | grep -q "License is suspended.*${STREAM_ID}"; then
    _license_line="$(printf '%s' "${_ams_logs}" | grep "License is suspended.*${STREAM_ID}" | tail -1)"
    log "LICENSE GATE TRIGGERED: ${_license_line}"
    {
      echo "SKIP"
      printf 'Reason: AMS EE SRT ingest is license-gated; license is suspended.\n'
      printf 'First post-lapse SRT enforcement observed: 2026-07-13 20:57:47Z (S29/D-091).\n'
      printf 'AMS trial license lapsed: 2026-07-12T12:09Z; RTMP ingest unaffected.\n'
      printf 'SRTAdaptor log line: %s\n' "${_license_line}"
      printf 'Evidence log:  %s/antmedia-log-snippet.txt\n' "${EVIDENCE_DIR}"
      printf 'Evidence REST: %s/ams-broadcast-notfound-*.json\n' "${EVIDENCE_DIR}"
      printf 'Scenario committed and ready; re-run after license renewal.\n'
      printf 'Closure criterion: live SRT observation run post-license (DG-18, S29/D-091).\n'
    } > "${EVIDENCE_DIR}/verdict.txt"
    exit 77
  else
    log "FAIL: broadcast not found and no licence-suspension log found — real defect"
    {
      echo "FAIL"
      printf 'Broadcast %s never appeared in AMS within 30 s.\n' "${STREAM_ID}"
      printf 'No "License is suspended" line found in antmedia logs (last 5 min).\n'
      printf 'This is a real defect — SRT ingest should be accepted or explicitly rejected.\n'
      printf 'Investigate: AMS SRT adaptor configuration, port 4200 availability, ACF streamid format.\n'
      printf 'Evidence: %s/\n' "${EVIDENCE_DIR}"
    } > "${EVIDENCE_DIR}/verdict.txt"
    exit 1
  fi
fi

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: Observation — SRT ingest metrics (runs when license is valid)
# ─────────────────────────────────────────────────────────────────────────────
#
# OBSERVATION FRAMING (DG-18 — SRT post-ARQ semantics):
#   AMS BroadcastDTO packetLostRatio for SRT ingest reflects post-ARQ loss:
#   what remains after SRT's error-correction at srtReceiveLatencyInMs.
#   Pre-ARQ transport loss that SRT's retransmission repaired before delivering
#   to AMS is invisible to both AMS and Pulse. A packetLostRatio=0 on SRT
#   ingest may coexist with significant transport-layer loss if ARQ succeeded.
#
#   This differs from RTMP (TCP retransmission masks all loss → always 0) and
#   WebRTC (UDP-native, no ARQ masking → raw network loss visible).
#
#   The publishType field for SRT in BroadcastDTO was unknown at S29 authoring;
#   its live value is recorded here for the first time.
#
#   See docs/known-limitations.md LIM-17; docs/assessment/final-assessment.md §4.2.
#
log "Phase 3: collecting SRT ingest metrics (observation mode)"

# Allow AMS to collect initial baseline metrics after the broadcaster appears
sleep 5

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "srt-post-start"
_ams_raw="$(curl -s -m 10 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
  2>/dev/null || echo '{}')"

_ams_status="$(printf '%s' "${_ams_raw}" \
  | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")"
_ams_bitrate="$(printf '%s' "${_ams_raw}" \
  | jq '.bitrate // 0' 2>/dev/null || echo 0)"
_ams_loss_ratio="$(printf '%s' "${_ams_raw}" \
  | jq '.packetLostRatio // 0' 2>/dev/null || echo 0)"
_ams_packets_lost="$(printf '%s' "${_ams_raw}" \
  | jq '.packetsLost // 0' 2>/dev/null || echo 0)"
_ams_publish_type="$(printf '%s' "${_ams_raw}" \
  | jq -r '.publishType // "unknown"' 2>/dev/null || echo "unknown")"

log "AMS: status=${_ams_status} bitrate=${_ams_bitrate} packetLostRatio=${_ams_loss_ratio} packetsLost=${_ams_packets_lost} publishType=${_ams_publish_type}"

{
  printf 'SRT ingest observations (S29/D-091 — first post-licence live run when available):\n'
  printf '  status:           %s\n' "${_ams_status}"
  printf '  bitrate:          %s bps\n' "${_ams_bitrate}"
  printf '  packetLostRatio:  %s  (post-ARQ; 0 = ARQ repaired all transport loss)\n' "${_ams_loss_ratio}"
  printf '  packetsLost:      %s  (cumulative post-ARQ lost packet count)\n' "${_ams_packets_lost}"
  printf '  publishType:      %s  (live value recorded; was unknown at S29 authoring)\n' "${_ams_publish_type}"
  printf '\n'
  printf 'PROTOCOL NOTE (DG-18):\n'
  printf '  packetLostRatio for SRT = post-ARQ loss at srtReceiveLatencyInMs.\n'
  printf '  Pre-ARQ transport loss repaired by SRT is invisible to AMS and Pulse.\n'
  printf '  For RTMP: always 0 (TCP masks loss). For WebRTC: raw UDP loss visible.\n'
  printf '  Cross-reference: docs/known-limitations.md LIM-17\n'
  printf '                   docs/assessment/final-assessment.md §4.2\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Structural assertions ─────────────────────────────────────────────────────
# These confirm the SRT ingest is established and healthy.
# They do NOT assert any particular value for packetLostRatio — any value
# (including 0) is valid depending on whether ARQ repaired the transport loss.
assert_eq "${_ams_status}" "broadcasting" \
  "${SCENARIO} AMS status=broadcasting (SRT ingest established)"

_bitrate_gt0="$(awk -v b="${_ams_bitrate}" 'BEGIN { print (b > 0) ? "true" : "false" }')"
assert_eq "${_bitrate_gt0}" "true" \
  "${SCENARIO} AMS bitrate (${_ams_bitrate} bps) > 0 (SRT ingest is flowing)"

# ── Pulse-side observation ────────────────────────────────────────────────────
# Pulse inherits packetLostRatio from AMS (× 100 → packet_loss_pct).
# This is informational; any value is valid for the same ARQ reasons above.
_NOW_S="$(date +%s)"
_FROM_MS=$(( (_NOW_S - 600) * 1000 ))
_TO_MS=$(( _NOW_S * 1000 ))
capture_pulse "/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" "srt-post-start"

_pulse_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" \
  2>/dev/null || echo '{}')"

_pulse_loss_pct="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .timeseries
    | if length > 0 then .[-1].packet_loss_pct // 0 else 0 end
  ' 2>/dev/null | head -1 || echo 0)"
_pulse_loss_pct="${_pulse_loss_pct:-0}"

log "Pulse packet_loss_pct=${_pulse_loss_pct} (informational; = AMS packetLostRatio × 100)"
{
  printf 'Pulse observation:\n'
  printf '  packet_loss_pct: %s  (= AMS packetLostRatio × 100; post-ARQ)\n' "${_pulse_loss_pct}"
  printf '\n'
  printf 'Cross-reference: docs/known-limitations.md LIM-17\n'
  printf '                 docs/AMS-INTEGRATION.md §1.1 DG-18 variant note\n'
  printf '                 docs/assessment/documentation-gaps.md DG-18\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict — structural assertions (status=broadcasting, bitrate>0)"
scenario_verdict
exit $?
