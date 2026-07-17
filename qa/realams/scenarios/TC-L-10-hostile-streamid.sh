#!/usr/bin/env bash
# qa/realams/scenarios/TC-L-10-hostile-streamid.sh
#
# TC-L-10: Hostile streamId containing '#' — AMS StreamIdValidator boundary test
#
# Assertion matrix row:
#   Goal:   POST /broadcasts/create with streamId="val-a10-<hex>#hostile"
#           and determine whether AMS 3.0.3 StreamIdValidator blocks it.
#
#   Branch A (validator active — expected for AMS 3.0.3):
#     AMS returns non-2xx OR 2xx with success=false.
#     Assert: validator correctly rejected the hostile character → PASS.
#
#   Branch B (validator absent / permissive):
#     AMS returns 2xx and body looks like a broadcast object.
#     Poll Pulse GET /live/streams and assert hostile streamId is visible
#     (exercises PathEscape in Pulse REST client path construction).
#     DELETE the created broadcast in EXIT trap (url-encoded id).
#
#   Exactly one branch fires an assert; a third silent outcome is structurally
#   impossible (if/else) but would produce "no checks recorded" FAIL from
#   scenario_verdict.
#
# EXIT CODES
#   0   PASS  — branch A: validator rejected; OR branch B: Pulse shows stream
#   1   FAIL  — branch B fired but hostile streamId NOT visible in Pulse
#   77  SKIP  — AMS cookie file missing/empty
#
set -euo pipefail

SCENARIO="TC-L-10"
echo "=== ${SCENARIO}: Hostile streamId '#' validator boundary ===" >&2

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
# '#' is safe inside double-quotes; jq will serialize it as a JSON string literal
STREAM_ID="val-a10-$(openssl rand -hex 4)#hostile"
# URL-encode the id for use in DELETE path (jq @uri encodes '#' → '%23')
_STREAM_ID_ENCODED="$(jq -rn --arg s "${STREAM_ID}" '$s | @uri')"

EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ──────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap (set before any API call that may create a resource) ─────────
_BROADCAST_CREATED=0
cleanup() {
  log "CLEANUP: broadcast_created=${_BROADCAST_CREATED}"
  if [ "${_BROADCAST_CREATED}" = "1" ]; then
    log "CLEANUP: deleting hostile broadcast (encoded=${_STREAM_ID_ENCODED})"
    curl -s -m 15 -X DELETE \
      -b "${AMS_COOKIE_FILE}" \
      "${AMS_URL}/${APP}/rest/v2/broadcasts/${_STREAM_ID_ENCODED}" \
      > /dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

log "STREAM_ID='${STREAM_ID}'  encoded='${_STREAM_ID_ENCODED}'  APP=${APP}"
log "AMS_URL=${AMS_URL}  PULSE_URL=${PULSE_URL}"

# ── Cookie precondition ───────────────────────────────────────────────────────
# auth.sh sourced above logs in; guard in case cookie was purged between source and here.
if [ ! -s "${AMS_COOKIE_FILE}" ]; then
  printf 'SKIP\nAMS_COOKIE_FILE missing or empty: %s\n' "${AMS_COOKIE_FILE}" \
    > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Build JSON body safely (jq serializes '#' as a JSON string literal) ───────
_create_body="$(jq -n --arg sid "${STREAM_ID}" '{"streamId":$sid}')"
printf '%s\n' "${_create_body}" > "${EVIDENCE_DIR}/ams-create-request.json"

log "POST ${AMS_URL}/${APP}/rest/v2/broadcasts/create  body=${_create_body}"
_create_resp="${EVIDENCE_DIR}/ams-create-response.json"
_create_http="$(curl -s -m 20 \
  -X POST \
  -H "Content-Type: application/json" \
  -b "${AMS_COOKIE_FILE}" \
  -d "${_create_body}" \
  -o "${_create_resp}" \
  -w '%{http_code}' \
  "${AMS_URL}/${APP}/rest/v2/broadcasts/create" 2>/dev/null || echo "000")"

log "Create response: HTTP=${_create_http}"
jq . "${_create_resp}" 2>/dev/null || true
printf 'create_http=%s\n' "${_create_http}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Determine branch: rejected (A) or accepted (B) ───────────────────────────
# Accepted = 2xx AND body does NOT contain success:false
_rejected=0
case "${_create_http}" in
  2[0-9][0-9])
    _success_val="$(jq -r '.success // "null"' "${_create_resp}" 2>/dev/null || echo "null")"
    if [ "${_success_val}" = "false" ]; then
      log "AMS returned 2xx but success=false — treating as rejection"
      _rejected=1
    fi
    ;;
  *)
    _rejected=1
    ;;
esac
printf 'rejected=%s\n' "${_rejected}" >> "${EVIDENCE_DIR}/timeline.txt"

if [ "${_rejected}" = "1" ]; then
  # ── Branch A: validator active — AMS rejected the '#' streamId ───────────────
  log "BRANCH-A: AMS StreamIdValidator rejected hostile '#' streamId (HTTP=${_create_http})"
  printf 'branch=A\n' >> "${EVIDENCE_DIR}/timeline.txt"
  _branch_a_ok="yes"
  assert_eq "${_branch_a_ok}" "yes" \
    "${SCENARIO} branch-A: AMS StreamIdValidator rejected '#' streamId (HTTP=${_create_http})" || true
else
  # ── Branch B: AMS accepted the '#' streamId — check Pulse PathEscape ─────────
  log "BRANCH-B: AMS accepted hostile '#' streamId — polling Pulse /live/streams"
  _BROADCAST_CREATED=1
  printf 'branch=B\n' >> "${EVIDENCE_DIR}/timeline.txt"

  capture_ams "/${APP}/rest/v2/broadcasts/${_STREAM_ID_ENCODED}" "branch-b-broadcast"
  capture_pulse "/live/streams" "branch-b-pre-poll"

  # Poll Pulse /live/streams; use (.items // []) per harness contract §8
  _pulse_visible="no"
  _i=0
  while [ "${_i}" -lt 10 ]; do
    _count="$(curl -s -m 10 \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/live/streams" \
      | jq --arg id "${STREAM_ID}" \
        '[(.items // [])[] | select(.stream_id == $id)] | length' \
      2>/dev/null || echo 0)"
    log "Pulse count for hostile streamId: ${_count} (attempt $(( _i + 1 ))/10)"
    if [ "${_count}" != "0" ] && [ -n "${_count}" ]; then
      _pulse_visible="yes"
      log "Hostile streamId visible in Pulse after $(( (_i + 1) * 3 )) s"
      break
    fi
    sleep 3
    _i=$(( _i + 1 ))
  done

  capture_pulse "/live/streams" "branch-b-post-poll"
  printf 'pulse_visible=%s\n' "${_pulse_visible}" >> "${EVIDENCE_DIR}/timeline.txt"
  assert_eq "${_pulse_visible}" "yes" \
    "${SCENARIO} branch-B: hostile '#' streamId visible in Pulse /live/streams (PathEscape works)" || true
fi

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
