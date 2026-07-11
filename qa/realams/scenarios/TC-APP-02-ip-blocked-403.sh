#!/usr/bin/env bash
# qa/realams/scenarios/TC-APP-02-ip-blocked-403.sh
#
# TC-APP-02: IP-blocked app 403 handling
#
# Assertion matrix row:
#   Steps:        1. Check pulse-realams-pulse-1 logs for 403 entries (blocked apps)
#                 2. Confirm Pulse still serves /api/v1/live/overview (not crashed)
#   AMS truth:    AMS returns HTTP 403 for apps with remoteAllowedCIDR=127.0.0.1
#   Pulse assert: Pulse logs warning per blocked app (403 count > 0);
#                 Pulse does NOT crash; live/overview still works for open apps
#   Exit:         0 PASS | 1 FAIL
#
set -euo pipefail

SCENARIO="TC-APP-02"
echo "=== ${SCENARIO}: IP-blocked app 403 handling ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
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

log "PULSE_URL=${PULSE_URL}"

# ── Step 1: Count 403 warnings in pulse-realams-pulse-1 logs ─────────────────
log "Extracting 403 warning count from pulse-realams-pulse-1 logs"
_log_403_count="$(sg docker -c "docker logs pulse-realams-pulse-1 2>&1" \
  | grep -c '403' 2>/dev/null || true)"
log "pulse-realams-pulse-1 log 403 count: ${_log_403_count}"

# Premise check (S17 drift finding): this AMS currently has NO IP-blocked apps
# (S16's 16-app inventory shrank to 4, all remoteAllowedCIDR-open). With no
# blocked app there is nothing for Pulse to 403 on — the scenario has no live
# trigger and must SKIP, not FAIL. Verify the premise via the authed app list:
# every app whose app-scope REST answers 200 from this VPS is open.
if [ "${_log_403_count}" = "0" ]; then
  _blocked=0
  _apps="$(curl -s -m 15 -b "${AMS_COOKIE_FILE}" "${AMS_URL}/rest/v2/applications" 2>/dev/null \
    | jq -r '.applications[]? // empty' 2>/dev/null || true)"
  for _app in ${_apps}; do
    _code="$(curl -s -m 10 -o /dev/null -w '%{http_code}' \
      "${AMS_URL}/${_app}/rest/v2/broadcasts/list/0/1" 2>/dev/null || echo 000)"
    log "  app=${_app} app-scope HTTP ${_code}"
    [ "${_code}" = "403" ] && _blocked=$((_blocked + 1))
  done
  if [ "${_blocked}" -eq 0 ]; then
    {
      echo "SKIP"
      echo "Premise unmet: no IP-blocked apps exist on this AMS today (all apps"
      echo "answer app-scope REST with 200 from this VPS), so the 403-handling"
      echo "path has no live trigger. S16 inventory (16 apps, 8 blocked) has"
      echo "drifted to $(printf '%s' "${_apps}" | wc -w) open apps. Re-run after"
      echo "creating a test app with remoteAllowedCIDR=127.0.0.1."
    } > "${EVIDENCE_DIR}/verdict.txt"
    cat "${EVIDENCE_DIR}/verdict.txt" >&2
    exit 77
  fi
fi

# Save a sample of 403 log lines for evidence
sg docker -c "docker logs pulse-realams-pulse-1 2>&1" \
  | grep '403' \
  | head -50 \
  > "${EVIDENCE_DIR}/pulse-403-log-lines.txt" 2>/dev/null || true

printf '403_log_count=%s\n' "${_log_403_count}" >> "${EVIDENCE_DIR}/timeline.txt"

assert_gte "${_log_403_count}" 1 "${SCENARIO} pulse-realams logs contain at least 1 '403' entry (blocked apps polled)" || true

# ── Step 2: Pulse must not crash — live/overview still accessible ─────────────
log "Pulse: GET /api/v1/live/overview (should still work)"
capture_pulse "/live/overview" "live-overview"
_overview_resp="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  "${PULSE_URL}/live/overview" 2>/dev/null || echo '{}')"

_overview_status="$(printf '%s' "${_overview_resp}" | jq -r '.total_publishers // "missing"' 2>/dev/null || echo parse_error)"
log "live/overview total_publishers=${_overview_status}"

# Assert overview responded with a parseable JSON object (not error)
_apps_present="$(printf '%s' "${_overview_resp}" | jq 'has("apps")' 2>/dev/null || echo false)"
assert_eq "${_apps_present}" "true" "${SCENARIO} live/overview response has apps field (not crashed)" || true

# Confirm Pulse healthz is also up
_health_code="$(curl -s -m 10 -o /dev/null -w '%{http_code}' \
  "${PULSE_HEALTH_URL}" 2>/dev/null || echo 0)"
assert_eq "${_health_code}" "200" "${SCENARIO} Pulse healthz returns 200 (not crashed by 403 handling)" || true

# ── Verdict ──────────────────────────────────────────────────────────────────
scenario_verdict
exit $?
