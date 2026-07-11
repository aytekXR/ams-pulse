#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-05-viewer-ramp.sh
#
# TC-V-05: Viewer count ramp — 5 → 10 HLS viewers
#
# Assertion matrix row (scenario-matrix.md):
#   Steps:   1. Start publisher val-v05-<epoch> on LiveApp
#            2. Ramp 5 HLS viewers via ramp_hls_viewers (step=5, interval=10s)
#            3. Wait for stable AMS plateau 1 (two consecutive samples <10%
#               apart; budget 120 s); SKIP if AMS never reaches >=1
#            4. Capture AMS DTO + Pulse stream row back-to-back (same second)
#            5. Assert Pulse viewer_count ≈ AMS hlsViewerCount ±5% (HLS tol)
#            6. Add 5 more HLS viewers (total 10)
#            7. Wait for stable AMS plateau 2 (budget 120 s)
#            8. Capture AMS DTO + Pulse stream row back-to-back
#            9. Assert Pulse viewer_count ≈ AMS hlsViewerCount ±5% (plateau 2)
#
#   AMS truth:    AMS hlsViewerCount at stable plateau.
#                 NOTE: AMS uses a sliding request-window counter, NOT a
#                 session counter.  A corrected sim viewer (fetch-each-segment-
#                 once) maps ~1 viewer window per session, but window edge
#                 effects mean the AMS count may still not equal sim_count
#                 exactly.  This is an AMS-semantics finding documented in
#                 DG-01.  The assertion compares Pulse vs AMS only — not
#                 either vs sim_count.
#
#   Pulse assert: viewer_count tracks AMS hlsViewerCount within ±5% at
#                 each stable plateau (Pulse MUST agree with AMS).
#
#   SKIP:    If AMS hlsViewerCount never reaches >=1 within 120 s (premise
#            unmet — HLS not producing segments).
#   Exit:    0 PASS | 1 FAIL | 77 SKIP
#
# Ramp reduced from original 10→30 to 5→10 (S18 retest) so load stays sane
# while still exercising the ramp shape.
#
# BINDING RULES (memory: shell-harness-false-green-patterns):
#   - every assert_* ends with || true
#   - every curl|jq inside $() carries 2>/dev/null || echo <safe-default>
#   - SKIP: write verdict.txt manually then exit 77
#   - stop_all_hls_viewers called in cleanup trap
#
set -euo pipefail

SCENARIO="TC-V-05"
echo "=== ${SCENARIO}: Viewer count ramp (5 → 10 HLS viewers) ===" >&2

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
STREAM_ID="val-v05-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ───────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ───────────────────────────────────────────────────────────────
cleanup() {
  log "CLEANUP: stopping all HLS viewers and publisher ${STREAM_ID}"
  stop_all_hls_viewers || true
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

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "pre-ramp"

# ── 2. Ramp to 5 HLS viewers ──────────────────────────────────────────────────
# ramp_hls_viewers(ID, TARGET=5, STEP=5, INTERVAL=10s)
log "Ramp phase 1: starting 5 HLS viewers (step=5, interval=10 s)"
ramp_hls_viewers "${STREAM_ID}" 5 5 10

# ── 3. Stable-plateau poll for plateau 1 (budget: 120 s) ─────────────────────
# Wait for two consecutive AMS hlsViewerCount samples that differ by <10%.
# This avoids comparing while the sliding window is still filling.
log "Polling AMS hlsViewerCount for stable plateau 1 (budget: 120 s, interval: 3 s)"
_p1_prev_ams=-1
_p1_first_seen_s=-1
_p1_plateau_ams=0
_p1_converge_s=-1
_i=0
while [ "${_i}" -lt 40 ]; do
  _p1_curr="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
  _p1_curr="${_p1_curr:-0}"

  if [ "${_p1_curr}" -ge 1 ] 2>/dev/null; then
    # Record first time we see any viewers
    if [ "${_p1_first_seen_s}" -eq -1 ] 2>/dev/null; then
      _p1_first_seen_s=$(( _i * 3 ))
      log "Plateau 1: AMS hlsViewerCount first reached ${_p1_curr} at ${_p1_first_seen_s} s"
    fi

    # Check consecutive stability vs previous sample
    if [ "${_p1_prev_ams}" -ge 1 ] 2>/dev/null; then
      _p1_stable="$(awk -v c="${_p1_curr}" -v p="${_p1_prev_ams}" 'BEGIN {
        diff = (c - p < 0) ? (p - c) : (c - p)
        pct  = diff / p * 100
        print (pct < 10) ? "yes" : "no"
      }')"
      if [ "${_p1_stable}" = "yes" ]; then
        _p1_plateau_ams="${_p1_curr}"
        _p1_converge_s=$(( _i * 3 ))
        log "Plateau 1: stable at AMS hlsViewerCount=${_p1_curr} (prev=${_p1_prev_ams}, diff<10%) after ${_p1_converge_s} s"
        break
      fi
    fi
    _p1_prev_ams="${_p1_curr}"
  fi

  log "Plateau 1 poll: AMS hlsViewerCount=${_p1_curr} (attempt $(( _i + 1 ))/40)"
  sleep 3
  _i=$(( _i + 1 ))
done

# ── SKIP: HLS never produced any segment traffic ───────────────────────────────
if [ "${_p1_first_seen_s}" -eq -1 ] 2>/dev/null; then
  log "SKIP: AMS hlsViewerCount never reached >=1 within 120 s — HLS may not be serving segments"
  printf 'SKIP\nPrecondition unmet: AMS hlsViewerCount for %s never reached >=1 within 120 s.\nHLS is segment-request based; stream may not have produced segments yet.\n' \
    "${STREAM_ID}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# If we timed out before stable, log a warning and proceed with the last reading
if [ "${_p1_converge_s}" -eq -1 ] 2>/dev/null; then
  _p1_plateau_ams="${_p1_curr:-0}"
  log "WARNING: plateau 1 not fully stable within 120 s; using last AMS reading=${_p1_plateau_ams}"
fi

# ── 4. Back-to-back capture of AMS DTO + Pulse stream row ─────────────────────
# Capture both sources in rapid succession so the counts reflect the same moment.
# A short Pulse-lag window (≤Pulse poll interval) is covered by the ±5% tolerance.
log "Plateau 1: back-to-back AMS+Pulse capture (stable AMS=${_p1_plateau_ams})"
_p1_ams_snap="$(curl -s -m 10 \
  "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
  2>/dev/null || echo '{}')"
_p1_pulse_snap="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp" \
  2>/dev/null || echo '{"items":[]}')"

# Parse the same-moment snapshots
_p1_ams_hls="$(printf '%s' "${_p1_ams_snap}" \
  | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
_p1_ams_hls="${_p1_ams_hls:-0}"

_p1_pulse_vc="$(printf '%s' "${_p1_pulse_snap}" | jq \
  --arg id "${STREAM_ID}" \
  '(.items // .streams // []) | map(select(.stream_id == $id)) | first
   | (.viewers // .viewer_count // 0)' \
  2>/dev/null || echo 0)"
_p1_pulse_vc="${_p1_pulse_vc:-0}"

log "Plateau 1 snapshot: AMS hlsViewerCount=${_p1_ams_hls}  Pulse viewer_count=${_p1_pulse_vc}"
printf 'plateau1: ams_hls=%s  pulse_vc=%s  stable_at_s=%s  sim_viewers=5\n' \
  "${_p1_ams_hls}" "${_p1_pulse_vc}" "${_p1_converge_s}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# Save evidence copies
capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "plateau1-ams"
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "plateau1-pulse"

# AMS-semantics note: AMS hlsViewerCount uses a sliding request window, not a
# session counter, so the count may differ from sim_count (5). That is expected
# and documented in DG-01. The assertion compares Pulse vs AMS only.

# ── 5. Assert Pulse viewer_count ≈ AMS hlsViewerCount at plateau 1 ────────────
# HLS tolerance: ±5% (scenario-matrix.md §Viewer Count Tolerance)
assert_approx "${_p1_pulse_vc}" "${_p1_ams_hls}" 5 \
  "${SCENARIO} plateau1 Pulse viewer_count ≈ AMS hlsViewerCount (±5% HLS tolerance; Pulse vs AMS only)" || true

# ── 6. Add 5 more HLS viewers (total 10) ─────────────────────────────────────
log "Ramp phase 2: adding 5 more HLS viewers (total 10)"
_vn=1
while [ "${_vn}" -le 5 ]; do
  start_hls_viewer "${STREAM_ID}" "LiveApp" "v05-ext-${_vn}"
  _vn=$(( _vn + 1 ))
done
log "Extra batch (v05-ext-1..5) started (10 total HLS viewers active)"

# ── 7. Stable-plateau poll for plateau 2 (budget: 120 s) ─────────────────────
log "Polling AMS hlsViewerCount for stable plateau 2 (budget: 120 s, interval: 3 s)"
_p2_prev_ams=-1
_p2_plateau_ams="${_p1_plateau_ams}"
_p2_converge_s=-1
_i=0
while [ "${_i}" -lt 40 ]; do
  _p2_curr="$(curl -s -m 10 \
    "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
    | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
  _p2_curr="${_p2_curr:-0}"

  if [ "${_p2_curr}" -ge 1 ] 2>/dev/null; then
    if [ "${_p2_prev_ams}" -ge 1 ] 2>/dev/null; then
      _p2_stable="$(awk -v c="${_p2_curr}" -v p="${_p2_prev_ams}" 'BEGIN {
        diff = (c - p < 0) ? (p - c) : (c - p)
        pct  = diff / p * 100
        print (pct < 10) ? "yes" : "no"
      }')"
      if [ "${_p2_stable}" = "yes" ]; then
        _p2_plateau_ams="${_p2_curr}"
        _p2_converge_s=$(( _i * 3 ))
        log "Plateau 2: stable at AMS hlsViewerCount=${_p2_curr} (prev=${_p2_prev_ams}, diff<10%) after ${_p2_converge_s} s"
        break
      fi
    fi
    _p2_prev_ams="${_p2_curr}"
  fi

  log "Plateau 2 poll: AMS hlsViewerCount=${_p2_curr} (attempt $(( _i + 1 ))/40)"
  sleep 3
  _i=$(( _i + 1 ))
done

if [ "${_p2_converge_s}" -eq -1 ] 2>/dev/null; then
  _p2_plateau_ams="${_p2_curr:-0}"
  log "AMS-FINDING (DG-01): AMS hlsViewerCount did not reach a stable plateau 2 within 120 s."
  log "This may reflect HLS CDN caching or slow segment-expiry semantics."
  log "Using last AMS count=${_p2_plateau_ams} for plateau 2 comparison."
fi

# ── 8. Back-to-back capture of AMS DTO + Pulse stream row (plateau 2) ─────────
log "Plateau 2: back-to-back AMS+Pulse capture (last stable AMS=${_p2_plateau_ams})"
_p2_ams_snap="$(curl -s -m 10 \
  "${AMS_URL}/LiveApp/rest/v2/broadcasts/${STREAM_ID}" \
  2>/dev/null || echo '{}')"
_p2_pulse_snap="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/streams?stream=${STREAM_ID}&app=LiveApp" \
  2>/dev/null || echo '{"items":[]}')"

_p2_ams_hls="$(printf '%s' "${_p2_ams_snap}" \
  | jq '.hlsViewerCount // 0' 2>/dev/null || echo 0)"
_p2_ams_hls="${_p2_ams_hls:-0}"

_p2_pulse_vc="$(printf '%s' "${_p2_pulse_snap}" | jq \
  --arg id "${STREAM_ID}" \
  '(.items // .streams // []) | map(select(.stream_id == $id)) | first
   | (.viewers // .viewer_count // 0)' \
  2>/dev/null || echo 0)"
_p2_pulse_vc="${_p2_pulse_vc:-0}"

log "Plateau 2 snapshot: AMS hlsViewerCount=${_p2_ams_hls}  Pulse viewer_count=${_p2_pulse_vc}"
printf 'plateau2: ams_hls=%s  pulse_vc=%s  stable_at_s=%s  sim_viewers=10\n' \
  "${_p2_ams_hls}" "${_p2_pulse_vc}" "${_p2_converge_s}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

capture_ams "/LiveApp/rest/v2/broadcasts/${STREAM_ID}" "plateau2-ams"
capture_pulse "/live/streams?stream=${STREAM_ID}&app=LiveApp" "plateau2-pulse"

# ── 9. Assert Pulse viewer_count ≈ AMS hlsViewerCount at plateau 2 ────────────
assert_approx "${_p2_pulse_vc}" "${_p2_ams_hls}" 5 \
  "${SCENARIO} plateau2 Pulse viewer_count ≈ AMS hlsViewerCount (±5% HLS tolerance; Pulse vs AMS only)" || true

# ── Verdict ────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
