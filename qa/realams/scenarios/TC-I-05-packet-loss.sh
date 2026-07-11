#!/usr/bin/env bash
# qa/realams/scenarios/TC-I-05-packet-loss.sh
#
# TC-I-05: Packet loss — simulated via Docker netem sidecar
#
# Assertion matrix row:
#   Setup:        Start publisher pulse-pub-val-val-i05-<epoch> on LiveApp.
#                 Inject 10% packet loss via netem sidecar sharing the publisher's
#                 network namespace (--net container:<name> + NET_ADMIN capability).
#                 No sudo required — network manipulation happens inside Docker.
#   AMS truth:    packetLostRatio > 0 in broadcast DTO after 30 s injection
#   Pulse assert: packet_loss_pct ≈ ratio*100 within ±50 pct band (loss measurement is
#                 inherently noisy; AMS may smooth or sample infrequently)
#   Skip path:    If the netem sidecar fails for ANY reason (image pull, kernel
#                 module absent, permission denied, etc.) → exit 77 SKIP with the
#                 blocker documented verbatim. Time-box: ≤3 sidecar attempts total.
#   Exit:         0 PASS | 1 FAIL | 77 SKIP (netem unavailable or publisher premise unmet)
#
set -euo pipefail

SCENARIO="TC-I-05"
echo "=== ${SCENARIO}: Packet Loss Injection — netem sidecar ===" >&2

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
# Attempt order:
#   1. Try alpine:3 with inline apk install of iproute2 (needs internet access)
#   2. On failure: SKIP 77 with blocker documented

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
    echo "Manual validation required: verify packetLostRatio in AMS BroadcastDTO manually."
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
log "Holding 30 s under packet loss for AMS to accumulate loss metrics"
sleep 30

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: Capture and assert AMS + Pulse packet loss metrics
# ─────────────────────────────────────────────────────────────────────────────
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "post-netem"
_ams_post_raw="$(curl -s -m 10 "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
  2>/dev/null || echo '{}')"

_ams_loss_ratio="$(printf '%s' "${_ams_post_raw}" | jq '.packetLostRatio // 0' 2>/dev/null || echo 0)"
_ams_packets_lost="$(printf '%s' "${_ams_post_raw}" | jq '.packetsLost // 0' 2>/dev/null || echo 0)"
_ams_status_post="$(printf '%s' "${_ams_post_raw}" | jq -r '.status // "unknown"' 2>/dev/null || echo "unknown")"

log "AMS post-netem: status=${_ams_status_post} packetLostRatio=${_ams_loss_ratio} packetsLost=${_ams_packets_lost}"
{
  printf 'AMS post-netem: packetLostRatio=%s packetsLost=%s status=%s\n' \
    "${_ams_loss_ratio}" "${_ams_packets_lost}" "${_ams_status_post}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# Allow 5 s more for Pulse ClickHouse flush
log "Waiting 5 s for Pulse ClickHouse insert + MV propagation"
sleep 5

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

log "Pulse post-netem: packet_loss_pct=${_pulse_loss_pct} health_score=${_pulse_health}"
{
  printf 'Pulse post-netem: packet_loss_pct=%s health_score=%s\n' "${_pulse_loss_pct}" "${_pulse_health}"
  printf 'Expected: AMS ratio > 0; Pulse loss_pct ≈ ratio*100 (±50 pct band)\n'
} >> "${EVIDENCE_DIR}/timeline.txt"

# ─────────────────────────────────────────────────────────────────────────────
# Assertions
# ─────────────────────────────────────────────────────────────────────────────

# AMS must show packetLostRatio > 0 (loss injection was active)
_ams_loss_gt0="$(awk -v r="${_ams_loss_ratio}" 'BEGIN { print (r > 0) ? "true" : "false" }')"
assert_eq "${_ams_loss_gt0}" "true" \
  "${SCENARIO} AMS packetLostRatio (${_ams_loss_ratio}) > 0 after netem 10% loss injection" || true

# Pulse packet_loss_pct should approximate AMS ratio*100 within ±50 pct band.
# AMS ratio 0..1; Pulse stores ratio*100. E.g. AMS=0.05 → Pulse≈5.
# ±50 pct band accounts for AMS smoothing, sampling intervals, and netem stochasticity.
# Only assert if AMS actually reported loss (precondition).
if [ "${_ams_loss_gt0}" = "true" ]; then
  _expected_loss_pct="$(awk -v r="${_ams_loss_ratio}" 'BEGIN { printf "%.2f", r * 100 }')"
  assert_approx "${_pulse_loss_pct}" "${_expected_loss_pct}" 50 \
    "${SCENARIO} Pulse packet_loss_pct (${_pulse_loss_pct}) ≈ AMS ratio*100 (${_expected_loss_pct}) within ±50 pct" || true
else
  log "NOTE: AMS reported zero packet loss — Pulse packet_loss_pct assertion skipped (no ground truth)"
  {
    printf 'NOTE: AMS packetLostRatio=0 after injection. Netem may not have affected AMS-side'\''s measurement window.\n'
    printf 'Pulse packet_loss_pct assertion skipped (no AMS ground truth > 0).\n'
  } >> "${EVIDENCE_DIR}/timeline.txt"
fi

# ── Verdict ─────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
