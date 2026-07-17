#!/usr/bin/env bash
# qa/realams/scenarios/TC-WH-10-hook-wire-capture.sh
#
# TC-WH-10: Webhook wire capture — verify AMS delivers liveStreamStarted + liveStreamEnded
#
# SETTINGS-MUTATING — requires ALLOW_SETTINGS_MUTATION=1
#
# Assertion matrix row:
#   Steps:  1. Guard: ALLOW_SETTINGS_MUTATION=1 and python3 present and AMS_COOKIE_FILE non-empty
#           2. Snapshot AMS app settings to settings-before.json
#           3. Start python capture server on HOOK_PORT (default 18799)
#           4. Mutate settings: listenerHookURL + webhookStreamStatusUpdatePeriodMs=2000
#           5. Publish ~8 s, graceful stop, sleep 8 s (let hooks drain)
#           6. Assert liveStreamStarted and liveStreamEnded captured in JSONL
#           7. Log content_type sample and liveStreamStatus count as INFO
#           8. Copy capture JSONL to agents/handoffs/real-ams-captures/
#   EXIT trap: restore settings-before.json, kill capture server, stop publisher
#   Exit:   0 PASS | 1 FAIL | 77 SKIP
#
set -euo pipefail

SCENARIO="TC-WH-10"
echo "=== ${SCENARIO}: Webhook wire capture ===" >&2

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

# ── Per-run identifiers ────────────────────────────────────────────────────────
APP="${AMS_APP:-LiveApp}"
STREAM_ID="val-wh10-$(openssl rand -hex 4)"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ───────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Guard: settings mutation ────────────────────────────────────────────────────
if [ "${ALLOW_SETTINGS_MUTATION:-0}" != "1" ]; then
  printf 'SKIP\nALLOW_SETTINGS_MUTATION != 1 — settings mutation not permitted\n' \
    > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Preconditions ──────────────────────────────────────────────────────────────
if ! python3 -c "import http.server, json" 2>/dev/null; then
  printf 'SKIP\nPrecondition: python3 not available\n' > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

if [ ! -s "${AMS_COOKIE_FILE}" ]; then
  printf 'SKIP\nPrecondition: AMS_COOKIE_FILE missing or empty (%s)\n' \
    "${AMS_COOKIE_FILE}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Snapshot AMS settings (before mutation) ────────────────────────────────────
log "Snapshotting AMS settings for ${APP}"
curl -s -m 20 \
  -b "${AMS_COOKIE_FILE}" \
  "${AMS_URL}/rest/v2/applications/settings/${APP}" \
  | jq . > "${EVIDENCE_DIR}/settings-before.json" 2>/dev/null || {
    printf 'SKIP\nCould not retrieve AMS settings for %s\n' "${APP}" \
      > "${EVIDENCE_DIR}/verdict.txt"
    exit 77
  }

if [ ! -s "${EVIDENCE_DIR}/settings-before.json" ]; then
  printf 'SKIP\nAMS settings snapshot is empty for %s — check AMS_COOKIE_FILE\n' "${APP}" \
    > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Hook server config ─────────────────────────────────────────────────────────
HOOK_PORT="${HOOK_PORT:-18799}"
_HOOK_HOST="${HOOK_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}' || echo '127.0.0.1')}"
_HOOK_URL="http://${_HOOK_HOST}:${HOOK_PORT}/"
_CAPTURE_JSONL="${EVIDENCE_DIR}/hooks-captured.jsonl"
_PY_SCRIPT="${EVIDENCE_DIR}/capture_server.py"
_SERVER_PID=""
_SETTINGS_MUTATED=0
_PUB_STARTED=0

# ── EXIT trap — set before any background process or publisher ─────────────────
cleanup() {
  if [ "${_PUB_STARTED}" = "1" ]; then
    stop_publisher "${STREAM_ID}" 2>/dev/null || true
  fi
  if [ -n "${_SERVER_PID}" ]; then
    kill "${_SERVER_PID}" 2>/dev/null || true
  fi
  if [ "${_SETTINGS_MUTATED}" = "1" ] && [ -s "${EVIDENCE_DIR}/settings-before.json" ]; then
    log "CLEANUP: restoring AMS settings for ${APP}"
    curl -s -m 20 \
      -X PUT \
      -H "Content-Type: application/json" \
      -b "${AMS_COOKIE_FILE}" \
      --data-binary "@${EVIDENCE_DIR}/settings-before.json" \
      "${AMS_URL}/rest/v2/applications/settings/${APP}" > /dev/null 2>&1 || true
    log "CLEANUP: settings restored"
  fi
}
trap cleanup EXIT

# ── Write python capture server ────────────────────────────────────────────────
cat > "${_PY_SCRIPT}" << 'PYEOF'
import http.server
import json
import sys
import time


class CaptureHandler(http.server.BaseHTTPRequestHandler):
    outfile = ""

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length).decode("utf-8", errors="replace")
        ct = self.headers.get("Content-Type", "")
        entry = {"ts": time.time(), "content_type": ct, "body": body}
        with open(CaptureHandler.outfile, "a") as fh:
            fh.write(json.dumps(entry) + "\n")
        self.send_response(200)
        self.end_headers()

    def log_message(self, fmt, *args):
        pass


def main():
    port = int(sys.argv[1])
    CaptureHandler.outfile = sys.argv[2]
    srv = http.server.HTTPServer(("0.0.0.0", port), CaptureHandler)
    srv.serve_forever()


main()
PYEOF

# ── Start capture server in background ────────────────────────────────────────
log "Starting capture server on ${_HOOK_HOST}:${HOOK_PORT}"
python3 "${_PY_SCRIPT}" "${HOOK_PORT}" "${_CAPTURE_JSONL}" &
_SERVER_PID=$!
sleep 1   # let server bind

# ── Mutate AMS settings ────────────────────────────────────────────────────────
log "Mutating listenerHookURL → ${_HOOK_URL}  webhookStreamStatusUpdatePeriodMs=2000"
_SETTINGS_MODIFIED="${EVIDENCE_DIR}/settings-modified.json"
jq --arg hookurl "${_HOOK_URL}" \
   '.listenerHookURL = $hookurl | .webhookStreamStatusUpdatePeriodMs = 2000' \
   "${EVIDENCE_DIR}/settings-before.json" > "${_SETTINGS_MODIFIED}"

curl -s -m 20 \
  -X PUT \
  -H "Content-Type: application/json" \
  -b "${AMS_COOKIE_FILE}" \
  --data-binary "@${_SETTINGS_MODIFIED}" \
  "${AMS_URL}/rest/v2/applications/settings/${APP}" > /dev/null
_SETTINGS_MUTATED=1
log "Settings mutated"

# ── Publish ~8 s ──────────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} on ${APP} @ 1000 kbps"
start_publisher "${STREAM_ID}" "${APP}" 1000
_PUB_STARTED=1
sleep 8
log "Stopping publisher"
stop_publisher "${STREAM_ID}"
_PUB_STARTED=0
log "Publisher stopped — sleeping 8 s for hook drain"
sleep 8

# ── Count captured events ──────────────────────────────────────────────────────
_started_count="$(jq -rs \
  '[.[] | .body | try fromjson | select(.action == "liveStreamStarted")] | length' \
  "${_CAPTURE_JSONL}" 2>/dev/null || echo 0)"
_ended_count="$(jq -rs \
  '[.[] | .body | try fromjson | select(.action == "liveStreamEnded")] | length' \
  "${_CAPTURE_JSONL}" 2>/dev/null || echo 0)"
_status_count="$(jq -rs \
  '[.[] | .body | try fromjson | select(.action == "liveStreamStatus")] | length' \
  "${_CAPTURE_JSONL}" 2>/dev/null || echo 0)"
_sample_ct="$(jq -rs 'first | .content_type // "none"' \
  "${_CAPTURE_JSONL}" 2>/dev/null || echo none)"

log "started_count=${_started_count}  ended_count=${_ended_count}  status_count=${_status_count}"

# ── INFO lines to timeline (not asserts) ──────────────────────────────────────
printf '[INFO] hook_url: %s\n' "${_HOOK_URL}" >> "${EVIDENCE_DIR}/timeline.txt"
printf '[INFO] content_type_sample: %s\n' "${_sample_ct}" >> "${EVIDENCE_DIR}/timeline.txt"
printf '[INFO] liveStreamStatus_events: %s\n' "${_status_count}" >> "${EVIDENCE_DIR}/timeline.txt"

# ── Copy capture to shared artefacts ──────────────────────────────────────────
_CAPTURES_DIR="${REPO_ROOT}/agents/handoffs/real-ams-captures"
mkdir -p "${_CAPTURES_DIR}"
cp "${_CAPTURE_JSONL}" \
  "${_CAPTURES_DIR}/TC-WH-10-$(date -u +%Y%m%dT%H%M%SZ)-hooks.jsonl" 2>/dev/null || true
log "Capture JSONL copied to ${_CAPTURES_DIR}"

# ── Assertions ─────────────────────────────────────────────────────────────────
log "ASSERT: liveStreamStarted captured"
_started_ok="no"
[ "${_started_count}" -gt 0 ] 2>/dev/null && _started_ok="yes" || true
assert_eq "${_started_ok}" "yes" \
  "${SCENARIO} liveStreamStarted hook captured (count=${_started_count})" || true

log "ASSERT: liveStreamEnded captured"
_ended_ok="no"
[ "${_ended_count}" -gt 0 ] 2>/dev/null && _ended_ok="yes" || true
assert_eq "${_ended_ok}" "yes" \
  "${SCENARIO} liveStreamEnded hook captured (count=${_ended_count})" || true

# ── Verdict ────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
