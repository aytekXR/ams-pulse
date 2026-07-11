#!/usr/bin/env bash
# qa/realams/scenarios/TC-WH-03-vod-recording-gap.sh
#
# TC-WH-03: VoD recording gap — BUG-002 evidence
#
# Assertion matrix row:
#   Steps:        1. Authed AMS GET /rest/v2/applications/info → vodCount/storage ground truth
#                 2. Pulse GET /api/v1/reports/usage → recording_gb
#   AMS truth:    vodCount > 0 (WebRTCAppEE has ~24 GB VoDs)
#   Pulse assert: recording_gb == 0 (no vodReady webhook delivery — AMS 3.0.3 unsigned)
#                 Documents BUG-002: recording_gb gap caused by webhook unavailability
#   Exit:         0 PASS | 1 FAIL
#
# Evidence establishes BUG-002: AMS has VoDs but Pulse cannot count them because
# vodReady webhooks are not delivered (AMS 3.0.3 cannot sign hooks, O3 decision).
#
set -euo pipefail

SCENARIO="TC-WH-03"
echo "=== ${SCENARIO}: VoD recording gap (BUG-002 evidence) ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

cleanup() { : ; }
trap cleanup EXIT

log "PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Step 1: AMS ground truth — applications/info (vodCount / storage) ─────────
log "AMS: GET /rest/v2/applications/info"
capture_ams "/rest/v2/applications/info" "ams-applications-info"
_apps_info_raw="$(curl -s -m 20 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/rest/v2/applications/info" 2>/dev/null || echo '[]')"
printf '%s' "${_apps_info_raw}" | jq . > "${EVIDENCE_DIR}/ams-applications-info.json" 2>/dev/null || true

# Sum vodCount and vodSize across all apps
_total_vod_count="$(printf '%s' "${_apps_info_raw}" | \
  jq '[.[] | (.vodCount // 0)] | add // 0' 2>/dev/null || echo 0)"
_total_vod_size_bytes="$(printf '%s' "${_apps_info_raw}" | \
  jq '[.[] | (.vodSize // .storageSize // 0)] | add // 0' 2>/dev/null || echo 0)"

# FALLBACK (S17 triage finding): AMS build 20260504_1443 returns HTTP 405 on
# GET /rest/v2/applications/info (API drift vs the S16 capture). Ground truth
# then comes from per-app app-scope GET /{app}/rest/v2/vods/count ({"number":N});
# size is unavailable via this path (recorded as unknown, not 0-claimed).
_vod_size_known=1
if [ "${_total_vod_count}" = "0" ] || [ -z "${_total_vod_count}" ]; then
  log "applications/info unusable (405 API drift?) — falling back to per-app vods/count"
  _vod_size_known=0
  _total_vod_size_bytes=0
  _apps_json="$(curl -s -m 15 -b "${AMS_COOKIE_FILE}" "${AMS_URL}/rest/v2/applications" 2>/dev/null || echo '{}')"
  _total_vod_count=0
  for _app in $(printf '%s' "${_apps_json}" | jq -r '.applications[]? // empty' 2>/dev/null); do
    _n="$(curl -s -m 15 "${AMS_URL}/${_app}/rest/v2/vods/count" 2>/dev/null | jq -r '.number // 0' 2>/dev/null || echo 0)"
    log "  app=${_app} vodCount=${_n}"
    printf 'fallback_vod_count app=%s count=%s\n' "${_app}" "${_n}" >> "${EVIDENCE_DIR}/timeline.txt"
    _total_vod_count=$(( _total_vod_count + _n ))
  done
fi

# Convert bytes to GB for comparison
_total_vod_size_gb="$(awk -v bytes="${_total_vod_size_bytes}" 'BEGIN { printf "%.2f", bytes / (1024*1024*1024) }')"

log "AMS total vodCount=${_total_vod_count}  vodSize=${_total_vod_size_bytes} bytes (${_total_vod_size_gb} GB)"
printf 'ams_total_vod_count=%s\nams_total_vod_size_bytes=%s\nams_total_vod_size_gb=%s\n' \
  "${_total_vod_count}" "${_total_vod_size_bytes}" "${_total_vod_size_gb}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# Ground truth: AMS must have VoDs for this test to be meaningful.
# S17 drift: the app reset wiped the S16-era VoDs (was ~1006/24 GB in
# WebRTCAppEE) — with zero VoDs the gap has no live contrast to demonstrate,
# so SKIP with instructions rather than FAIL (the Pulse-side recording_gb==0
# plus the code-level webhook chain still back BUG-002).
if [ "${_total_vod_count}" -eq 0 ]; then
  {
    echo "SKIP"
    echo "Premise unmet: AMS currently has zero VoDs in every app — nothing to"
    echo "contrast recording_gb=0 against. Create one: enable mp4 muxing on a"
    echo "test app (POST /rest/v2/applications/settings/pulse-test with"
    echo "mp4MuxingEnabled=true), publish ~20 s, stop, re-run this scenario."
  } > "${EVIDENCE_DIR}/verdict.txt"
  cat "${EVIDENCE_DIR}/verdict.txt" >&2
  exit 77
fi
assert_gte "${_total_vod_count}" 1 "${SCENARIO} AMS vodCount > 0 (VoDs exist, ground truth)" || true

# ── Step 2: Pulse usage report ────────────────────────────────────────────────
log "Pulse: GET /api/v1/reports/usage"
capture_pulse "/reports/usage" "usage-report"
_usage_raw="$(curl -s -m 20 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/reports/usage" 2>/dev/null || echo '{}')"
printf '%s' "${_usage_raw}" | jq . > "${EVIDENCE_DIR}/pulse-usage-report.json" 2>/dev/null || true

_pulse_recording_gb="$(printf '%s' "${_usage_raw}" | \
  jq '.totals.recording_gb // 0' 2>/dev/null || echo 0)"
log "Pulse totals.recording_gb=${_pulse_recording_gb}"
printf 'pulse_recording_gb=%s\n' "${_pulse_recording_gb}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertion: recording_gb must be 0 ────────────────────────────────────────
assert_eq "${_pulse_recording_gb}" "0" "${SCENARIO} Pulse recording_gb=0 (vodReady webhook not delivered)" || true

# ── Document BUG-002 ─────────────────────────────────────────────────────────
{
  printf '=== BUG-002: recording_gb gap ===\n'
  if [ "${_vod_size_known}" = "1" ]; then
    printf 'AMS ground truth: vodCount=%s  vodSize=%s bytes (%s GB)\n' \
      "${_total_vod_count}" "${_total_vod_size_bytes}" "${_total_vod_size_gb}"
  else
    printf 'AMS ground truth: vodCount=%s  vodSize=UNKNOWN (applications/info returns 405 in build 20260504_1443 — AMS API drift, see S17 triage)\n' \
      "${_total_vod_count}"
  fi
  printf 'Pulse recording_gb: %s\n' "${_pulse_recording_gb}"
  printf '\nROOT CAUSE:\n'
  printf '  AMS 3.0.3 sends vodReady lifecycle hooks to listenerHookURL but\n'
  printf '  cannot HMAC-sign them (no HMAC fields in AMS 3.0.3 hook payload).\n'
  printf '  Pulse webhook listener (fail-closed) rejects unsigned deliveries.\n'
  printf '  Therefore Pulse never receives vodReady events → recording_gb=0.\n'
  printf '\nIMPACT:\n'
  printf '  Usage reports show recording_gb=0 despite AMS having %s VoDs (%s GB).\n' \
    "${_total_vod_count}" "${_total_vod_size_gb}"
  printf '\nREFERENCE:\n'
  printf '  O3 decision: AMS webhook unsigned — closed N/A for AMS 3.x\n'
  printf '  scenario-matrix.md TC-WH-03, TC-A-09\n'
} > "${EVIDENCE_DIR}/BUG-002-recording-gap.txt"

log "BUG-002 evidence written to ${EVIDENCE_DIR}/BUG-002-recording-gap.txt"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
