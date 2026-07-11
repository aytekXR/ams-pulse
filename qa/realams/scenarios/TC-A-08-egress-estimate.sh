#!/usr/bin/env bash
# qa/realams/scenarios/TC-A-08-egress-estimate.sh
#
# TC-A-08: Usage report — egress_gb is a bitrate×watch-time ESTIMATE (not CDN truth)
#
# Assertion matrix row — S17 CORRECTION overrides S16 row:
#   S16 row said:  "egress_gb == 0 (unimplemented)"
#   S17 CORRECTION: prod showed egress_gb=0.0025 — it is a bitrate×watch_time ESTIMATE,
#                   NOT a CDN byte counter. The field is always present and >= 0.
#   Steps:     1. GET /reports/usage (no time filter — full history)
#              2. Assert egress_gb >= 0 AND is a numeric field (not absent/null)
#              3. Inspect egress_method — documents estimation methodology
#              4. Cross-check estimate semantics via accounting.go:
#                 EgressMethodBitrateXWatchTime = "bitrate_x_watch_time"
#                 Formula: (viewer_minutes * avg_bitrate_kbps * 60 * 1000) / 8 / 1e9
#              5. Write verdict.txt documenting whether stack shows 0 (no beacon
#                 history) or an estimate, AND the egress_method value observed
#   Pulse assert: egress_gb >= 0 AND egress_method field is present
#   Exit:       0 PASS | 1 FAIL
#
# Semantics documentation (accounting.go:29–47):
#   EgressMethodBitrateXWatchTime — bitrate × watch-time estimate from beacon heartbeats
#   NOT a CDN-delivered-bytes counter. The estimate will be 0 when no beacon
#   sessions exist (no viewer_minutes in rollup_qoe_1h).
#   egress_method field documents the methodology in every response (F6 spec).
#
set -euo pipefail

SCENARIO="TC-A-08"
echo "=== ${SCENARIO}: Usage report — egress_gb estimate semantics ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
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

log "PULSE_URL=${PULSE_URL}"

# ── Step 1: GET /reports/usage ────────────────────────────────────────────────
log "GET /reports/usage (no time filter — full history)"
_usage_http="$(curl -s -m 30 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -o "${EVIDENCE_DIR}/usage-report.json" \
  -w '%{http_code}' \
  "${PULSE_URL}/reports/usage" 2>/dev/null || echo 000)"

_usage_resp="$(jq -c '.' "${EVIDENCE_DIR}/usage-report.json" 2>/dev/null || echo '{}')"
log "Usage report HTTP=${_usage_http}"
capture_pulse "/reports/usage" "usage-report"

# ── Step 2: Extract egress_gb and egress_method ───────────────────────────────
_egress_gb="$(printf '%s' "${_usage_resp}" | \
  jq '.totals.egress_gb // "ABSENT"' 2>/dev/null || echo '"ABSENT"')"
_egress_method="$(printf '%s' "${_usage_resp}" | \
  jq -r '.egress_method // "ABSENT"' 2>/dev/null || echo "ABSENT")"
_recording_gb="$(printf '%s' "${_usage_resp}" | \
  jq '.totals.recording_gb // "ABSENT"' 2>/dev/null || echo '"ABSENT"')"
_viewer_minutes="$(printf '%s' "${_usage_resp}" | \
  jq '.totals.viewer_minutes // 0' 2>/dev/null || echo 0)"

log "egress_gb=${_egress_gb}  egress_method=${_egress_method}  viewer_minutes=${_viewer_minutes}"
printf 'usage_http=%s\negress_gb=%s\negress_method=%s\nrecording_gb=%s\nviewer_minutes=%s\n' \
  "${_usage_http}" "${_egress_gb}" "${_egress_method}" "${_recording_gb}" "${_viewer_minutes}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Step 3: Numeric check — egress_gb >= 0 ───────────────────────────────────
_egress_is_number="$(awk -v v="${_egress_gb}" 'BEGIN {
  # jq outputs a bare number (no quotes) when present; "ABSENT" is not numeric
  if (v + 0 == v && v != "ABSENT") print "yes"; else print "no"
}')"
_egress_gte0="$(awk -v v="${_egress_gb}" 'BEGIN {
  if (v ~ /^[0-9.eE+-]+$/) print (v + 0 >= 0) ? "yes" : "no"; else print "no"
}')"

log "egress_gb is_number=${_egress_is_number}  gte0=${_egress_gte0}"

# ── Step 4: Document semantics ────────────────────────────────────────────────
# Determine whether stack reports 0 (no beacon history) or an estimate
_egress_float="$(awk -v v="${_egress_gb}" 'BEGIN { printf "%.6f", (v+0) }' 2>/dev/null || echo "0.000000")"
_has_history="$(awk -v vm="${_viewer_minutes}" 'BEGIN { print (vm > 0) ? "yes" : "no" }')"

{
  printf '=== TC-A-08: egress_gb estimate semantics ===\n'
  printf 'Source: GET /reports/usage  HTTP=%s\n\n' "${_usage_http}"
  printf 'OBSERVED VALUES:\n'
  printf '  totals.egress_gb     = %s\n' "${_egress_gb}"
  printf '  egress_method        = %s\n' "${_egress_method}"
  printf '  totals.viewer_minutes= %s\n' "${_viewer_minutes}"
  printf '  totals.recording_gb  = %s\n' "${_recording_gb}"
  printf '\nS17 CORRECTION (overrides S16 "egress_gb==0 unimplemented"):\n'
  printf '  egress_gb is a bitrate×watch-time ESTIMATE, not a CDN byte counter.\n'
  printf '  Formula (accounting.go:42-47):\n'
  printf '    egress_gb = (viewer_minutes * avg_bitrate_kbps * 60 * 1000) / 8 / 1e9\n'
  printf '  Default bitrate_kbps=1000 when not known.\n'
  printf '  EgressMethod constant: "bitrate_x_watch_time" (accounting.go:32)\n'
  printf '\nSEMANTICS FOR THIS RUN:\n'
  if [ "${_has_history}" = "yes" ]; then
    printf '  Stack has viewer_minutes > 0 → egress_gb=%s is a non-zero ESTIMATE.\n' "${_egress_float}"
    printf '  This matches the S17 prod observation (0.0025) — the field is live.\n'
  else
    printf '  Stack has viewer_minutes=0 → egress_gb=%s (no beacon sessions to estimate from).\n' "${_egress_float}"
    printf '  A value of 0 here does NOT mean "unimplemented" — it means no viewer\n'
    printf '  heartbeat data is in the rollup yet. Run TC-A-06 first to inject data.\n'
  fi
  printf '\negress_method field MUST always be present per F6 spec.\n'
  printf 'Observed: "%s"\n' "${_egress_method}"
} > "${EVIDENCE_DIR}/egress-semantics.txt"

log "Egress semantics documented in ${EVIDENCE_DIR}/egress-semantics.txt"

# ── Assertions ───────────────────────────────────────────────────────────────
# Usage report must return HTTP 200
assert_eq "${_usage_http}" "200" "${SCENARIO} /reports/usage returns HTTP 200" || true

# egress_gb must be a number >= 0 (not absent, not negative)
assert_eq "${_egress_is_number}" "yes" "${SCENARIO} egress_gb is a numeric field (not absent)" || true
assert_eq "${_egress_gte0}" "yes" "${SCENARIO} egress_gb >= 0 (ESTIMATE; 0 means no beacon history)" || true

# egress_method must be present (F6 spec: always included)
_method_present="$([ "${_egress_method}" != "ABSENT" ] && echo yes || echo no)"
assert_eq "${_method_present}" "yes" "${SCENARIO} egress_method field present (F6 spec)" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
