#!/usr/bin/env bash
# qa/realams/scenarios/TC-APP-01-multi-app-polling.sh
#
# TC-APP-01: Multi-app polling — open apps
#
# Assertion matrix row:
#   Steps:        1. Authed GET /rest/v2/applications → ground truth app list
#                 2. Probe each app (no-auth app-scope GET) to classify open vs blocked
#                 3. GET /api/v1/live/overview → Pulse app list
#   AMS truth:    ~8 accessible apps (open CIDR); ~8 blocked (remoteAllowedCIDR=127.0.0.1)
#   Pulse assert: live/overview apps[] includes every open AMS app that has active streams;
#                 blocked apps produce 403 warnings in logs (not crashes)
#   Exit:         0 PASS | 1 FAIL
#
# Note: Records which apps are open vs blocked at runtime — S16 list may have drifted.
#
set -euo pipefail

SCENARIO="TC-APP-01"
echo "=== ${SCENARIO}: Multi-app polling — open apps ===" >&2

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

# ── Step 1: AMS ground truth — full app list (server-scope, needs cookie) ────
log "AMS: GET /rest/v2/applications"
capture_ams "/rest/v2/applications" "ams-applications"
_apps_raw="$(curl -s -m 20 -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/rest/v2/applications" 2>/dev/null || echo '{}')"

# Response may be {"applications":[...]} or a bare array
if printf '%s' "${_apps_raw}" | jq -e '.applications' > /dev/null 2>&1; then
  _app_list="$(printf '%s' "${_apps_raw}" | jq '.applications')"
elif printf '%s' "${_apps_raw}" | jq -e 'type == "array"' > /dev/null 2>&1; then
  _app_list="${_apps_raw}"
else
  log "WARN: unexpected shape for /rest/v2/applications: ${_apps_raw}"
  _app_list="[]"
fi

_total_apps="$(printf '%s' "${_app_list}" | jq 'length' 2>/dev/null || echo 0)"
log "AMS total apps: ${_total_apps}"
printf '%s\n' "${_app_list}" | jq . > "${EVIDENCE_DIR}/ams-app-list.json" 2>/dev/null || true

# Extract app names (AMS returns app objects; name field varies by AMS version)
# Try .name then .application_name then treat each element as a string
# NB: elements are plain STRINGS on AMS 3.0.3 — `.name` on a string is a hard
# jq error (not null), so type-dispatch instead of `//`-chaining.
_app_names="$(printf '%s' "${_app_list}" | \
  jq -r '.[] | if type == "object" then (.name // .application_name // empty) else . end' \
  2>/dev/null | sort || true)"

log "AMS app names: $(printf '%s' "${_app_names}" | tr '\n' ' ')"
printf 'ams_app_count=%s\nams_apps=%s\n' \
  "${_total_apps}" "$(printf '%s' "${_app_names}" | tr '\n' ',')" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Step 2: Classify each app as open or blocked ─────────────────────────────
log "Classifying apps: open vs IP-blocked (remoteAllowedCIDR)"
_open_apps=""
_blocked_apps=""

while IFS= read -r _app; do
  [ -z "${_app}" ] && continue
  _http_code="$(curl -s -m 10 -o /dev/null -w '%{http_code}' \
    "${AMS_URL}/${_app}/rest/v2/broadcasts/list/0/1" 2>/dev/null || echo 0)"
  if [ "${_http_code}" = "200" ] || [ "${_http_code}" = "206" ]; then
    _open_apps="${_open_apps} ${_app}"
    log "  OPEN:    ${_app} (HTTP ${_http_code})"
  elif [ "${_http_code}" = "403" ]; then
    _blocked_apps="${_blocked_apps} ${_app}"
    log "  BLOCKED: ${_app} (HTTP 403 — remoteAllowedCIDR)"
  else
    log "  UNKNOWN: ${_app} (HTTP ${_http_code})"
  fi
done <<< "${_app_names}"

_open_count="$(printf '%s' "${_open_apps}" | wc -w | tr -d ' ')"
_blocked_count="$(printf '%s' "${_blocked_apps}" | wc -w | tr -d ' ')"
log "Open apps (${_open_count}): ${_open_apps}"
log "Blocked apps (${_blocked_count}): ${_blocked_apps}"
printf 'open_apps=%s\nblocked_apps=%s\n' \
  "$(printf '%s' "${_open_apps}" | xargs)" \
  "$(printf '%s' "${_blocked_apps}" | xargs)" \
  >> "${EVIDENCE_DIR}/timeline.txt"
printf 'open_count=%s\nblocked_count=%s\n' "${_open_count}" "${_blocked_count}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Step 3: Pulse live/overview ───────────────────────────────────────────────
log "Pulse: GET /api/v1/live/overview"
capture_pulse "/live/overview" "live-overview"
_overview_raw="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/overview" 2>/dev/null || echo '{}')"

_pulse_apps_json="$(printf '%s' "${_overview_raw}" | jq '.apps // []' 2>/dev/null || echo '[]')"
_pulse_app_names="$(printf '%s' "${_pulse_apps_json}" | jq -r '.[].app' 2>/dev/null | sort || true)"
log "Pulse live/overview apps: $(printf '%s' "${_pulse_app_names}" | tr '\n' ' ')"
printf '%s\n' "${_pulse_apps_json}" | jq . > "${EVIDENCE_DIR}/pulse-apps.json" 2>/dev/null || true

# ── Assertions ───────────────────────────────────────────────────────────────
# Pulse must have polled at least the open apps (coverage check)
assert_gte "${_open_count}" 1 "${SCENARIO} at least 1 open AMS app found" || true

# For each open app that has active streams on AMS, Pulse must show it
# (blocked apps must NOT appear — 403 should be swallowed, not crash)
_found_blocked_in_pulse=0
for _bapp in ${_blocked_apps}; do
  _in_pulse="$(printf '%s' "${_pulse_app_names}" | grep -c "^${_bapp}$" 2>/dev/null || echo 0)"
  if [ "${_in_pulse}" -gt 0 ]; then
    log "WARN: blocked app ${_bapp} appears in Pulse overview (unexpected)"
    _found_blocked_in_pulse=$(( _found_blocked_in_pulse + 1 ))
  fi
done

# Note: blocked apps appearing in overview with 0 streams is acceptable;
# appearing with non-zero counts would be a bug.
for _oapp in ${_open_apps}; do
  # Check if AMS has active streams for this open app
  _ams_stream_count="$(curl -s -m 10 \
    "${AMS_URL}/${_oapp}/rest/v2/broadcasts/list/0/1" \
    | jq 'if type == "array" then length else 0 end' 2>/dev/null || echo 0)"
  if [ "${_ams_stream_count}" -gt 0 ]; then
    _in_pulse="$(printf '%s' "${_pulse_app_names}" | grep -c "^${_oapp}$" 2>/dev/null || echo 0)"
    assert_gte "${_in_pulse}" 1 "${SCENARIO} open app ${_oapp} (has streams) appears in Pulse overview" || true
  fi
done

# Verify Pulse is still accessible (not crashed by 403 handling)
_health_code="$(curl -s -m 10 -o /dev/null -w '%{http_code}' \
  "${PULSE_HEALTH_URL}" 2>/dev/null || echo 0)"
assert_eq "${_health_code}" "200" "${SCENARIO} Pulse healthz still returns 200 after 403 app polling" || true

log "open_count=${_open_count}  blocked_count=${_blocked_count}  pulse_accessible=${_health_code}"

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
