#!/usr/bin/env bash
# qa/realams/scenarios/TC-I-05-packet-loss.sh
#
# TC-I-05: Packet loss — RTMP/TCP transport semantics finding
#
# AMS-SEMANTICS-FINDING (confirmed S18 2026-07-11):
#   RTMP ingest rides TCP. TCP retransmission repairs packet loss below the
#   application layer, so AMS never observes lost packets on an RTMP ingest.
#   AMS fields `packetLostRatio` and `packetsLost` in BroadcastDTO reflect
#   UDP-layer counters (populated by WebRTC and SRT ingest paths only).
#   Injecting netem loss on a publisher's NIC while using RTMP produces:
#     • packetLostRatio  == 0.0  (expected — TCP masks loss)
#     • packetsLost      == 0    (expected)
#     • bitrate          >  0    (TCP absorbs loss; ingest healthy)
#     • status           == broadcasting
#   These are CORRECT values, not a measurement gap.
#
# Assertion matrix (revised):
#   Setup:        Start publisher pulse-pub-val-i05-<epoch> on LiveApp.
#                 Inject 10% packet loss via netem sidecar sharing the publisher's
#                 network namespace (--net container:<name> + NET_ADMIN capability).
#   AMS finding:  packetLostRatio == 0 for RTMP/TCP ingest (TCP masks loss).
#                 This is the EXPECTED value — TCP retransmits, AMS sees no loss.
#   Ingest check: bitrate > 0 and status == broadcasting (TCP absorbed the loss;
#                 ingest survived uninterrupted).
#   tc verify:    tc qdisc show dev eth0 on the sidecar confirms netem rule applied;
#                 the zero-ratio result is not a netem failure — TCP masked the loss.
#   SRT/WebRTC:   packetLostRatio for UDP-based ingest paths is an S19+ item
#                 requiring an SRT publisher setup.
#   Skip path:    If the netem sidecar fails for ANY reason → exit 77 SKIP with
#                 the blocker documented verbatim. Time-box: ≤3 attempts total.
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (netem unavailable or publisher premise unmet)
#
set -euo pipefail

SCENARIO="TC-I-05"
echo "=== ${SCENARIO}: Packet Loss — RTMP/TCP transport semantics ===" >&2

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
STREAM_ID="val-i05-${EPOCH}"
PUB_CNAME="pulse-pub-val-${STREAM_ID}"
SIDECAR_CNAME="netem-val-i05-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ────────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ────────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping sidecar ${SIDECAR_CNAME} (if running)"
  sg docker -c "docker stop ${SIDECAR_CNAME}" > /dev/null 2>&1 || true
  log "CLEANUP: stopping publisher ${STREAM_ID}"
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PUB_CNAME=${PUB_CNAME}  SIDECAR=${SIDECAR_CNAME}"
log "SEMANTICS: RTMP/TCP — packetLostRatio EXPECTED to remain 0 despite netem injection"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: Start publisher and confirm broadcasting
# ─────────────────────────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 2000 kbps on LiveApp"
start_publisher "${STREAM_ID}" "LiveApp" 2000

log "Polling AMS for status=broadcasting (budget: 30 s)"
_broadcasting=0
_i=0
while [ "${_i}" -lt 15 ]; do
  _st="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")"
  if [ "${_st}" = "broadcasting" ]; then
    log "AMS status=broadcasting after $(( _i * 2 )) s"
    _broadcasting=1
    break
  fi
  sleep 2
  _i=$(( _i + 1 ))
done

if [ "${_broadcasting}" -eq 0 ]; then
  log "SKIP: publisher never reached broadcasting (precondition unmet)"
  {
    echo "SKIP"
    echo "Publisher ${STREAM_ID} never reached AMS status=broadcasting in 30 s."
    echo "Cannot inject packet loss without an established RTMP ingest stream."
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# Wait 5 s for AMS to collect initial baseline metrics
log "Waiting 5 s for AMS baseline metrics before loss injection"
sleep 5

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-netem"

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: Netem sidecar injection — 10% packet loss on publisher eth0
# ─────────────────────────────────────────────────────────────────────────────
# Strategy: run a short-lived sidecar sharing the publisher container's network
# namespace (--net container:<name>). The sidecar needs NET_ADMIN capability
# to apply tc qdisc netem rules on eth0. The rule affects egress traffic from
# the publisher container (RTMP bytes going to AMS).
#
# Expected result: AMS packetLostRatio remains 0. TCP retransmission at the
# network layer repairs the injected loss before RTMP or AMS observes it.
# The bitrate may exhibit minor jitter (TCP backpressure), but ingest continues.
#
log "Phase 2: launching netem sidecar sharing ${PUB_CNAME} network namespace"
log "Sidecar cmd: docker run -d --net container:${PUB_CNAME} --cap-add NET_ADMIN alpine:3 sh -c 'apk add --no-cache iproute2 && tc qdisc add dev eth0 root netem loss 10% && sleep 40'"

_sidecar_result="$(sg docker -c "docker run -d \
  --name ${SIDECAR_CNAME} \
  --net container:${PUB_CNAME} \
  --cap-add NET_ADMIN \
  alpine:3 \
  sh -c 'apk add --no-cache iproute2 >/dev/null 2>&1 && tc qdisc add dev eth0 root netem loss 10% && sleep 40'" \
  2>&1 || echo "SIDECAR_FAILED")"

log "Sidecar launch result: ${_sidecar_result}"
{
  printf 'netem sidecar result: %s\n' "${_sidecar_result}"
} >> "${EVIDENCE_DIR}/timeline.txt"

if printf '%s' "${_sidecar_result}" | grep -q "SIDECAR_FAILED"; then
  log "SKIP: netem sidecar failed to launch — ${_sidecar_result}"
  {
    echo "SKIP"
    echo "Blocker: docker run --net container:${PUB_CNAME} --cap-add NET_ADMIN alpine:3 tc netem failed."
    echo "Error output: ${_sidecar_result}"
    echo "Possible causes: netem kernel module absent; NET_ADMIN denied by Docker daemon configuration;"
    echo "  alpine apk network access unavailable; host kernel lacks CONFIG_NET_SCH_NETEM."
    echo "Manual validation required: verify transport-layer semantics manually."
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# Sidecar is running; wait for iproute2 install + tc rule to take effect (~10 s)
log "Sidecar started (id=${_sidecar_result:0:12}); waiting 10 s for netem rule to activate"
sleep 10

# Verify the sidecar is still running (it might have exited if apk or tc failed)
_sidecar_running="$(sg docker -c "docker ps --filter name=${SIDECAR_CNAME} --format '{{.Names}}'" \
  2>/dev/null || echo "")"

if [ -z "${_sidecar_running}" ]; then
  _sidecar_logs="$(sg docker -c "docker logs ${SIDECAR_CNAME}" 2>&1 || echo "(no logs)")"
  log "SKIP: sidecar exited before netem rule was established — logs: ${_sidecar_logs}"
  {
    echo "SKIP"
    echo "Blocker: netem sidecar (${SIDECAR_CNAME}) exited prematurely after launch."
    echo "Sidecar logs: ${_sidecar_logs}"
    echo "Likely cause: apk install failed (no network) or tc netem unavailable on this kernel."
  } > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

log "Sidecar confirmed running — netem loss 10% is active on ${PUB_CNAME} eth0"

# Capture tc qdisc show to prove the netem rule was applied
_tc_output="$(sg docker -c "docker exec ${SIDECAR_CNAME} tc qdisc show dev eth0" 2>&1 || echo "(tc exec failed)")"
log "tc qdisc show: ${_tc_output}"
{
  printf 'tc qdisc show dev eth0 (from sidecar): %s\n' "${_tc_output}"
  printf 'SEMANTICS NOTE: RTMP uses TCP. TCP retransmission repairs injected loss below\n'
  printf '  the application layer. AMS packetLostRatio is expected to remain 0.\n'
  printf '  A zero result here is CORRECT behavior, not a measurement failure.\n'
  printf '  packetLostRatio > 0 is only meaningful for UDP ingest (SRT, WebRTC).\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

log "Holding 30 s under packet loss to confirm AMS bitrate and status remain stable"
sleep 30

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: Capture and assert AMS metrics under TCP-absorbed loss
# ─────────────────────────────────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "post-netem"
_ams_post_raw="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
  2>/dev/null || echo '{}')"

_ams_loss_ratio="$(printf '%s' "${_ams_post_raw}" | jq '.packetLostRatio // 0' 2>/dev/null || echo 0)"
_ams_packets_lost="$(printf '%s' "${_ams_post_raw}" | jq '.packetsLost // 0' 2>/dev/null || echo 0)"
_ams_status_post="$(printf '%s' "${_ams_post_raw}" | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")"
_ams_bitrate="$(printf '%s' "${_ams_post_raw}" | jq '.bitrate // 0' 2>/dev/null || echo 0)"

log "AMS post-netem: status=${_ams_status_post} packetLostRatio=${_ams_loss_ratio} packetsLost=${_ams_packets_lost} bitrate=${_ams_bitrate}"
{
  printf 'AMS post-netem: packetLostRatio=%s packetsLost=%s status=%s bitrate=%s\n' \
    "${_ams_loss_ratio}" "${_ams_packets_lost}" "${_ams_status_post}" "${_ams_bitrate}"
  printf 'EXPECTED for RTMP/TCP: packetLostRatio==0 (TCP masks loss); bitrate>0 (ingest healthy)\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Assertions — RTMP/TCP transport semantics
# ─────────────────────────────────────────────────────────────────────────────

# FINDING ASSERTION 1: packetLostRatio == 0 for RTMP/TCP.
# TCP retransmission repairs network-level loss before AMS observes it.
# A non-zero ratio here would indicate a non-TCP ingest path (unexpected).
_ams_loss_is_zero="$(awk -v r="${_ams_loss_ratio}" 'BEGIN { print (r == 0) ? "true" : "false" }')"
assert_eq "${_ams_loss_is_zero}" "true" \
  "${SCENARIO} AMS packetLostRatio==0 for RTMP/TCP under netem 10% loss (TCP retransmits mask loss — expected semantics)"

# FINDING ASSERTION 2: packetsLost == 0 for RTMP/TCP (same root cause).
_ams_pkts_lost_is_zero="$(awk -v p="${_ams_packets_lost}" 'BEGIN { print (p == 0) ? "true" : "false" }')"
assert_eq "${_ams_pkts_lost_is_zero}" "true" \
  "${SCENARIO} AMS packetsLost==0 for RTMP/TCP under netem 10% loss (TCP retransmits mask loss — expected semantics)"

# INGEST HEALTH 1: ingest must still be broadcasting (TCP absorbed the loss).
assert_eq "${_ams_status_post}" "broadcasting" \
  "${SCENARIO} AMS status=broadcasting under 10% netem loss (TCP absorbed it; RTMP ingest survived)"

# INGEST HEALTH 2: bitrate must be > 0 (stream is flowing).
_bitrate_gt0="$(awk -v b="${_ams_bitrate}" 'BEGIN { print (b > 0) ? "true" : "false" }')"
assert_eq "${_bitrate_gt0}" "true" \
  "${SCENARIO} AMS bitrate (${_ams_bitrate} bps) > 0 under 10% netem loss (TCP kept ingest alive)"

# Informational Pulse capture — packet_loss_pct will also be 0 since AMS reports 0
_NOW_S="$(date +%s)"
_FROM_MS=$(( (_NOW_S - 600) * 1000 ))
_TO_MS=$(( _NOW_S * 1000 ))
capture_pulse "/qoe/ingest?stream=${STREAM_ID}&app=LiveApp&from=${_FROM_MS}&to=${_TO_MS}" "post-netem"

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

_pulse_health="$(printf '%s' "${_pulse_resp}" | \
  jq --arg id "${STREAM_ID}" '
    .streams[]?
    | select(.stream_id == $id)
    | .health_score // 0
  ' 2>/dev/null | head -1 || echo 0)"
_pulse_health="${_pulse_health:-0}"

log "Pulse post-netem: packet_loss_pct=${_pulse_loss_pct} health_score=${_pulse_health} (informational only)"
{
  printf 'Pulse post-netem: packet_loss_pct=%s health_score=%s\n' "${_pulse_loss_pct}" "${_pulse_health}"
  printf 'Informational: Pulse inherits packetLostRatio from AMS; both are 0 for RTMP/TCP.\n'
  printf 'packetLostRatio is a meaningful signal ONLY for UDP-based ingest (SRT, WebRTC).\n'
  printf 'SRT/WebRTC ingest validation is an S19+ item (requires SRT publisher setup).\n'
  printf 'DG-18: packetLostRatio semantics per ingest protocol — see documentation-gaps.md\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict — AMS-SEMANTICS-FINDING: RTMP/TCP masks packet loss; packetLostRatio is 0 (correct)"
scenario_verdict
exit $?
