#!/usr/bin/env bash
# qa/realams/scenarios/TC-A-09-recording-zero.sh
#
# TC-A-09: recording_gb == 0 despite AMS VoDs — BUG-002 live-evidenced
#
# Assertion matrix row:
#   Steps:     1. Sum per-app vods/count across all AMS apps (S17: pulse-test has ≥1 VoD)
#              2. Assert sum >= 1 (AMS has VoDs — VoD ground truth)
#              3. GET /reports/usage → assert recording_gb == 0
#              4. Document BUG-002: webhook vodReady not delivered (AMS 3.x unsigned)
#   AMS truth: per-app GET /{app}/rest/v2/vods/count sum >= 1
#              (S17 created one test VoD on pulse-test; see scenario-matrix.md S17 correction §6)
#   Pulse assert: /reports/usage.totals.recording_gb == 0 (vodReady never delivered)
#   Exit:       0 PASS | 1 FAIL | 77 SKIP (AMS has zero VoDs — premise unmet)
#
# BUG-002 root cause:
#   AMS 3.x sends vodReady lifecycle hooks to listenerHookURL but cannot HMAC-sign them
#   (no HMAC fields in AMS 3.0.3 hook payload). Pulse webhook listener (fail-closed)
#   rejects unsigned deliveries. Therefore Pulse never receives vodReady events →
#   recording_gb stays 0. O3 decision: AMS webhook unsigned — closed N/A for AMS 3.x.
#
set -euo pipefail

SCENARIO="TC-A-09"
echo "=== ${SCENARIO}: recording_gb=0 (BUG-002 — vodReady webhook gap) ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

cleanup() { : ; }
trap cleanup EXIT

log "PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"

# ── Step 1: AMS ground truth — per-app vods/count ────────────────────────────
# S17 correction: GET /rest/v2/applications/info → HTTP 405 on AMS 3.0.3.
# Ground truth via per-app GET /{app}/rest/v2/vods/count (app-scope, no cookie needed).
# S17 created one test VoD on pulse-test (mp4 muxing temporarily enabled, then restored).
log "AMS: GET /rest/v2/applications to discover app list"
_apps_json="$(curl -s -m 15 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/rest/v2/applications" 2>/dev/null || echo '{}')"
printf '%s' "${_apps_json}" | jq . > "${EVIDENCE_DIR}/ams-applications.json" 2>/dev/null || true

# /rest/v2/applications returns {"applications":["name",...]} (S17 live finding)
_total_vod_count=0
for _app in $(printf '%s' "${_apps_json}" | jq -r '.applications[]? // empty' 2>/dev/null); do
  _n="$(curl -s -m 15 \
    "${AMS_URL}/${_app}/rest/v2/vods/count" 2>/dev/null \
    | jq '.number // 0' 2>/dev/null || echo 0)"
  log "  app=${_app} vodCount=${_n}"
  printf 'app=%s vod_count=%s\n' "${_app}" "${_n}" >> "${EVIDENCE_DIR}/timeline.txt"
  _total_vod_count=$(( _total_vod_count + _n ))
done

log "AMS total vodCount across all apps: ${_total_vod_count}"
printf 'ams_total_vod_count=%s\n' "${_total_vod_count}" >> "${EVIDENCE_DIR}/timeline.txt"

# Premise: AMS must have VoDs for BUG-002 to be demonstrable
if [ "${_total_vod_count}" -eq 0 ]; then
  {
    echo "SKIP"
    echo "Premise unmet: AMS currently has zero VoDs in every app."
    echo ""
    echo "S17 created a test VoD on pulse-test (mp4 muxing enabled, 20 s stream,"
    echo "then restored). If it has since been deleted, create one:"
    echo "  1. POST /LiveApp/rest/v2/applications/settings/pulse-test with mp4MuxingEnabled=true"
    echo "  2. Publish a 20 s RTMP stream to pulse-test/<id>"
    echo "  3. Restore mp4MuxingEnabled=false"
    echo "  4. Re-run this scenario"
  } > "${EVIDENCE_DIR}/verdict.txt"
  cat "${EVIDENCE_DIR}/verdict.txt" >&2
  exit 77
fi

assert_gte "${_total_vod_count}" 1 "${SCENARIO} AMS vodCount >= 1 (VoDs exist — BUG-002 ground truth)" || true

# ── Step 2: Pulse usage report — recording_gb ────────────────────────────────
log "Pulse: GET /reports/usage"
_usage_http="$(curl -s -m 30 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -o "${EVIDENCE_DIR}/usage-report.json" \
  -w '%{http_code}' \
  "${PULSE_URL}/reports/usage" 2>/dev/null || echo 000)"

_usage_resp="$(jq -c '.' "${EVIDENCE_DIR}/usage-report.json" 2>/dev/null || echo '{}')"
_recording_gb="$(printf '%s' "${_usage_resp}" | \
  jq '.totals.recording_gb // -1' 2>/dev/null || echo -1)"
log "Pulse recording_gb=${_recording_gb}  usage_http=${_usage_http}"
capture_pulse "/reports/usage" "usage-report"
printf 'usage_http=%s\npulse_recording_gb=%s\n' \
  "${_usage_http}" "${_recording_gb}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertion: recording_gb must be 0 (vodReady webhook never delivered) ───────
assert_eq "${_recording_gb}" "0" "${SCENARIO} Pulse recording_gb=0 (vodReady webhook not delivered — BUG-002)" || true

# ── Document BUG-002 ──────────────────────────────────────────────────────────
{
  printf '=== BUG-002: recording_gb gap (TC-A-09 evidence) ===\n'
  printf 'AMS total vodCount: %s (across all apps)\n' "${_total_vod_count}"
  printf 'Pulse recording_gb: %s\n' "${_recording_gb}"
  printf '\nROOT CAUSE:\n'
  printf '  AMS 3.x sends vodReady lifecycle hooks to listenerHookURL but cannot\n'
  printf '  HMAC-sign them (no HMAC fields in AMS 3.0.3 hook payload).\n'
  printf '  Pulse webhook listener (fail-closed) rejects unsigned deliveries.\n'
  printf '  Therefore Pulse never receives vodReady events → recording_gb=0.\n'
  printf '\nIMPACT:\n'
  printf '  Usage reports show recording_gb=0 despite AMS having %s VoD(s).\n' "${_total_vod_count}"
  printf '\nRESOLUTION (O3 decision):\n'
  printf '  AMS webhook unsigned — closed N/A for AMS 3.x.\n'
  printf '  Fix requires HMAC signing support in AMS 4.x or a polling-based\n'
  printf '  VoD counter fallback in Pulse (not yet implemented).\n'
  printf '\nREFERENCES:\n'
  printf '  scenario-matrix.md TC-A-09, TC-WH-03\n'
  printf '  docs/assessment/capability-map.md §recording (§VoD gap section)\n'
} > "${EVIDENCE_DIR}/BUG-002-recording-gap.txt"
log "BUG-002 documented in ${EVIDENCE_DIR}/BUG-002-recording-gap.txt"

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
