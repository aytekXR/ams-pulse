#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-10-hls-inflation-characterization.sh
#
# TC-V-10: HLS viewer-count inflation characterization (LOW risk, observe-only)
#
# PURPOSE
#   Characterize the known AMS HLS viewer-count inflation bug: AMS reports
#   totalHLSWatchersCount ≈ 9× the actual number of HLS segment-downloading
#   clients (each segment pull is counted as a new "viewer" within the
#   sliding window).  This scenario deliberately does NOT assert parity between
#   AMS and Pulse — that would be a false-failure given the inflation.
#
#   The ONLY assertion is a sanity check: peak AMS totalHLSWatchersCount >= 3,
#   confirming that 3 inline HLS pseudo-viewers were actually registered by AMS.
#
# STEPS
#   1. Start RTMP publisher (SKIP if never reaches broadcasting in 30 s).
#   2. Wait for HLS manifest to be available at ${AMS_URL}/${APP}/streams/${STREAM_ID}.m3u8
#      (SKIP if not available after 60 s — HLS may be disabled on this AMS build).
#   3. Start 3 inline HLS pseudo-viewers as background bash subshells, each
#      curling the newest .ts segment from the manifest every 2 s.
#   4. Viewing phase (60 s): sample AMS broadcast-statistics.totalHLSWatchersCount
#      and Pulse /live/streams[stream_id].viewers every 5 s → CSV.
#   5. Decay phase (90 s): stop the 3 viewers, continue sampling every 5 s → CSV.
#   6. Assert peak AMS totalHLSWatchersCount >= 3 (sanity only).
#   7. Write hls-inflation.csv to EVIDENCE_DIR for offline analysis.
#
# EXIT CODES
#   0   PASS  — peak AMS HLS count >= 3
#   1   FAIL  — peak count < 3 (viewers were not registered by AMS)
#   77  SKIP  — stream never reached broadcasting, or HLS manifest unavailable
#
set -euo pipefail

SCENARIO="TC-V-10"
echo "=== ${SCENARIO}: HLS viewer-count inflation characterization ===" >&2

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
STREAM_ID="val-v10-$(openssl rand -hex 4)"

EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ──────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap — set BEFORE publisher and background viewer PIDs ─────────────
# PID variables initialised to empty; kill is skipped when empty (avoids kill 0).
_HLS_PID1=""
_HLS_PID2=""
_HLS_PID3=""
cleanup() {
  log "CLEANUP: stopping HLS pseudo-viewers and publisher"
  [ -n "${_HLS_PID1}" ] && kill "${_HLS_PID1}" 2>/dev/null || true
  [ -n "${_HLS_PID2}" ] && kill "${_HLS_PID2}" 2>/dev/null || true
  [ -n "${_HLS_PID3}" ] && kill "${_HLS_PID3}" 2>/dev/null || true
  stop_publisher "${STREAM_ID}" 2>/dev/null || true
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  APP=${APP}  AMS_URL=${AMS_URL}  PULSE_URL=${PULSE_URL}"

# ── Step 1: Start publisher ───────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} at 1000 kbps on ${APP}"
start_publisher "${STREAM_ID}" "${APP}" 1000

# ── Step 2a: Wait for AMS status=broadcasting (precondition) ──────────────────
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

# ── Step 2b: Wait for HLS manifest (precondition — HLS may be disabled) ───────
_HLS_MANIFEST="${AMS_URL}/${APP}/streams/${STREAM_ID}.m3u8"
log "Waiting for HLS manifest at ${_HLS_MANIFEST} (budget: 60 s, interval: 5 s)"
_hls_ready=0
_i=0
while [ "${_i}" -lt 12 ]; do
  _hls_http="$(curl -s -m 8 -o /dev/null -w '%{http_code}' "${_HLS_MANIFEST}" 2>/dev/null || echo 000)"
  if [ "${_hls_http}" = "200" ]; then
    _hls_ready=1
    log "HLS manifest available after $(( _i * 5 )) s (HTTP=200)"
    break
  fi
  log "HLS manifest HTTP=${_hls_http} (attempt $(( _i + 1 ))/12)"
  sleep 5
  _i=$(( _i + 1 ))
done

if [ "${_hls_ready}" != "1" ]; then
  printf 'SKIP\nHLS manifest not available after 60 s: %s (HLS may be disabled on this AMS build)\n' \
    "${_HLS_MANIFEST}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi
capture_ams "/${APP}/rest/v2/broadcasts/${STREAM_ID}" "pre-viewers"

# ── Step 3: Define and start 3 inline HLS pseudo-viewers ──────────────────────
# Each loop fetches the live m3u8 playlist, extracts the newest .ts segment,
# and downloads it — simulating what a real HLS player does every ~2 s.
# Running as background subshells (fork): inherit parent shell variables and
# functions; || true on every command to survive set -euo pipefail in subshell.
_hls_viewer_loop() {
  local manifest_url="$1"
  local base_url="${manifest_url%/*}"
  local m3u8 seg
  while true; do
    m3u8="$(curl -s -m 8 "${manifest_url}" 2>/dev/null)" || true
    seg="$(printf '%s' "${m3u8}" | grep -v '^#' | grep -E '\.(ts|m4s)$' | tail -1 2>/dev/null)" || true
    if [ -n "${seg}" ]; then
      case "${seg}" in
        http*) curl -s -m 12 "${seg}" -o /dev/null 2>/dev/null || true ;;
        *)     curl -s -m 12 "${base_url}/${seg}" -o /dev/null 2>/dev/null || true ;;
      esac
    fi
    sleep 2
  done
}

log "Starting 3 inline HLS pseudo-viewers (background bash subshells)"
_hls_viewer_loop "${_HLS_MANIFEST}" &
_HLS_PID1=$!
_hls_viewer_loop "${_HLS_MANIFEST}" &
_HLS_PID2=$!
_hls_viewer_loop "${_HLS_MANIFEST}" &
_HLS_PID3=$!
log "HLS viewer PIDs: ${_HLS_PID1} ${_HLS_PID2} ${_HLS_PID3}"

# ── Step 4 & 5: Sample loop — viewing phase (60 s) then decay phase (90 s) ────
_CSV="${EVIDENCE_DIR}/hls-inflation.csv"
printf 'timestamp,phase,sample_s,ams_hls_count,pulse_viewers\n' > "${_CSV}"
_peak_ams=0
_sample_s=0

for _phase in viewing decay; do
  if [ "${_phase}" = "decay" ]; then
    log "Decay phase: stopping HLS viewers (PIDs: ${_HLS_PID1} ${_HLS_PID2} ${_HLS_PID3})"
    kill "${_HLS_PID1}" 2>/dev/null || true
    kill "${_HLS_PID2}" 2>/dev/null || true
    kill "${_HLS_PID3}" 2>/dev/null || true
    _HLS_PID1=""
    _HLS_PID2=""
    _HLS_PID3=""
  fi

  _phase_budget=60
  [ "${_phase}" = "decay" ] && _phase_budget=90
  _elapsed=0
  log "Phase=${_phase}  budget=${_phase_budget} s  interval=5 s"

  while [ "${_elapsed}" -lt "${_phase_budget}" ]; do
    # AMS broadcast-statistics: totalHLSWatchersCount reflects AMS's inflated count
    _ams_hls="$(curl -s -m 10 \
      -b "${AMS_COOKIE_FILE}" \
      "${AMS_URL}/${APP}/rest/v2/broadcasts/${STREAM_ID}/broadcast-statistics" \
      | jq '.totalHLSWatchersCount // 0' 2>/dev/null || echo 0)"

    # Pulse per-stream viewers from /live/streams items list (harness contract §8)
    _pulse_viewers="$(curl -s -m 10 \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/live/streams" \
      | jq --arg id "${STREAM_ID}" \
        '[(.items // [])[] | select(.stream_id == $id)] | if length > 0 then first | .viewers // 0 else 0 end' \
      2>/dev/null || echo 0)"

    printf '%s,%s,%s,%s,%s\n' \
      "$(date -u +%H:%M:%SZ)" "${_phase}" "${_sample_s}" "${_ams_hls}" "${_pulse_viewers}" \
      >> "${_CSV}"
    log "phase=${_phase} t=${_sample_s}s ams_hls=${_ams_hls} pulse_viewers=${_pulse_viewers}"

    # Track peak AMS HLS count across both phases
    if awk -v a="${_ams_hls:-0}" -v p="${_peak_ams:-0}" \
        'BEGIN{exit (a+0 > p+0) ? 0 : 1}' 2>/dev/null; then
      _peak_ams="${_ams_hls}"
    fi

    sleep 5
    _elapsed=$(( _elapsed + 5 ))
    _sample_s=$(( _sample_s + 5 ))
  done
done

log "Sampling complete — peak AMS HLS count: ${_peak_ams}"
printf 'peak_ams_hls=%s\n' "${_peak_ams}" >> "${EVIDENCE_DIR}/timeline.txt"
printf 'characterization_note: AMS totalHLSWatchersCount inflates approx 9x vs actual HLS clients (known behavior; NOT a parity assertion)\n' \
  >> "${EVIDENCE_DIR}/timeline.txt"
capture_ams "/${APP}/rest/v2/broadcasts/${STREAM_ID}" "post-decay"
capture_pulse "/live/streams" "post-decay"

# ── Step 6: Sanity assertion ──────────────────────────────────────────────────
# Deliberately NOT a parity assertion — inflation makes parity a false target.
# Only assert that AMS saw >= 3 HLS clients, confirming the pseudo-viewers
# were registered. (With ~9x inflation 3 viewers typically show as ~27+.)
log "ASSERT: sanity — peak AMS totalHLSWatchersCount >= 3"
assert_gte "${_peak_ams}" 3 \
  "${SCENARIO} peak AMS totalHLSWatchersCount >= 3 (sanity: 3 HLS pseudo-viewers were active; NOT a Pulse-parity check)" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict  CSV=${_CSV}"
scenario_verdict
exit $?
