#!/usr/bin/env bash
# qa/realams/scenarios/TC-WH-11-signing-proxy-e2e.sh
#
# TC-WH-11: Signing proxy end-to-end — HMAC-signed AMS webhook accepted; tampered rejected
#
# SETTINGS-MUTATING — requires ALLOW_SETTINGS_MUTATION=1
#
# Assertion matrix row:
#   Steps:  1. Guard: ALLOW_SETTINGS_MUTATION=1, python3 present, PULSE_WEBHOOK_SECRET set,
#              AMS_COOKIE_FILE non-empty
#           2. Derive PULSE_BASE from PULSE_URL; snapshot AMS settings
#           3. Write python signing proxy (stdlib-only) to file
#           4. Start proxy on PROXY_PORT (default 18800)
#           5. Mutate AMS listenerHookURL → proxy
#           6. Publish ~10 s, graceful stop, sleep 8 s (let hooks drain)
#           7. ASSERT: proxy log contains a liveStreamStarted entry with pulse_status == 200
#           8. POST tampered X-Ams-Signature: sha256=deadbeef directly to Pulse
#           9. ASSERT: HTTP response code is 401
#   EXIT trap: restore settings-before.json, kill proxy, stop publisher
#   Exit:   0 PASS | 1 FAIL | 77 SKIP
#
set -euo pipefail

SCENARIO="TC-WH-11"
echo "=== ${SCENARIO}: Signing proxy e2e ===" >&2

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
STREAM_ID="val-wh11-$(openssl rand -hex 4)"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Derive PULSE_BASE from PULSE_URL (strips /api/v1 suffix) ─────────────────
PULSE_BASE="${PULSE_URL%/api/v1}"

# ── Timeline log ───────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Guard: settings mutation ────────────────────────────────────────────────────
if [ "${ALLOW_SETTINGS_MUTATION:-0}" != "1" ]; then
  printf 'SKIP\nALLOW_SETTINGS_MUTATION != 1 — settings mutation not permitted\n' \
    > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Preconditions ──────────────────────────────────────────────────────────────
if ! python3 -c "import http.server, hmac, hashlib, urllib.request, json" 2>/dev/null; then
  printf 'SKIP\nPrecondition: python3 stdlib not fully available\n' \
    > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

if [ -z "${PULSE_WEBHOOK_SECRET:-}" ]; then
  printf 'SKIP\nPrecondition: PULSE_WEBHOOK_SECRET is not set\n' \
    > "${EVIDENCE_DIR}/verdict.txt"
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

# ── Proxy config ───────────────────────────────────────────────────────────────
PROXY_PORT="${PROXY_PORT:-18800}"
_PROXY_HOST="${PROXY_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}' || echo '127.0.0.1')}"
_PROXY_URL="http://${_PROXY_HOST}:${PROXY_PORT}/"
_PROXY_SCRIPT="${EVIDENCE_DIR}/signing_proxy.py"
_PROXY_LOGFILE="${EVIDENCE_DIR}/proxy-events.jsonl"
_PROXY_PID=""
_SETTINGS_MUTATED=0
_PUB_STARTED=0

# ── EXIT trap — set before any background process or publisher ─────────────────
cleanup() {
  if [ "${_PUB_STARTED}" = "1" ]; then
    stop_publisher "${STREAM_ID}" 2>/dev/null || true
  fi
  if [ -n "${_PROXY_PID}" ]; then
    kill "${_PROXY_PID}" 2>/dev/null || true
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

# ── Write python signing proxy (stdlib only) ───────────────────────────────────
# The proxy:
#   - Accepts AMS hook POSTs (JSON or form-encoded)
#   - Transcodes form bodies to JSON
#   - Signs with HMAC-SHA256 → X-Ams-Signature: sha256=<hex>
#   - When PULSE_WEBHOOK_REQUIRE_TIMESTAMP=true, signs over "{unix}.{body}" string
#   - Forwards to PULSE_WEBHOOK_URL
#   - Logs {ts, action, pulse_status} to LOGFILE
#
cat > "${_PROXY_SCRIPT}" << 'PYEOF'
import hashlib
import hmac
import http.server
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

PULSE_WEBHOOK_URL = sys.argv[1]
SECRET = sys.argv[2].encode("utf-8")
LISTEN_PORT = int(sys.argv[3])
LOGFILE = sys.argv[4]
REQUIRE_TS = os.environ.get("PULSE_WEBHOOK_REQUIRE_TIMESTAMP", "false").lower() == "true"


def compute_sig(body_bytes):
    if REQUIRE_TS:
        ts_str = str(int(time.time()))
        msg = (ts_str + "." + body_bytes.decode("utf-8", errors="replace")).encode("utf-8")
    else:
        msg = body_bytes
    return hmac.new(SECRET, msg, hashlib.sha256).hexdigest()


class SigningProxyHandler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        raw_body = self.rfile.read(length)
        ct = self.headers.get("Content-Type", "")

        # Transcode application/x-www-form-urlencoded → JSON
        if "application/x-www-form-urlencoded" in ct:
            params = urllib.parse.parse_qs(
                raw_body.decode("utf-8", errors="replace"), keep_blank_values=True
            )
            flat = {k: v[0] if len(v) == 1 else v for k, v in params.items()}
            body_bytes = json.dumps(flat).encode("utf-8")
        else:
            body_bytes = raw_body

        sig_hex = compute_sig(body_bytes)

        req = urllib.request.Request(
            PULSE_WEBHOOK_URL,
            data=body_bytes,
            headers={
                "Content-Type": "application/json",
                "X-Ams-Signature": "sha256=" + sig_hex,
            },
            method="POST",
        )
        try:
            resp = urllib.request.urlopen(req, timeout=10)
            pulse_status = resp.getcode()
        except urllib.error.HTTPError as exc:
            pulse_status = exc.code
        except Exception:
            pulse_status = -1

        try:
            action = json.loads(body_bytes).get("action", "unknown")
        except Exception:
            action = "unknown"

        entry = {
            "ts": time.time(),
            "action": action,
            "pulse_status": pulse_status,
            "sig_prefix": "sha256=" + sig_hex[:8] + "...",
        }
        with open(LOGFILE, "a") as fh:
            fh.write(json.dumps(entry) + "\n")

        self.send_response(200)
        self.end_headers()

    def log_message(self, fmt, *args):
        pass


def main():
    srv = http.server.HTTPServer(("0.0.0.0", LISTEN_PORT), SigningProxyHandler)
    srv.serve_forever()


main()
PYEOF

# ── Start proxy in background ──────────────────────────────────────────────────
log "Starting signing proxy on ${_PROXY_HOST}:${PROXY_PORT}"
PULSE_WEBHOOK_REQUIRE_TIMESTAMP="${PULSE_WEBHOOK_REQUIRE_TIMESTAMP:-false}" \
python3 "${_PROXY_SCRIPT}" \
  "${PULSE_BASE}/webhook/ams" \
  "${PULSE_WEBHOOK_SECRET}" \
  "${PROXY_PORT}" \
  "${_PROXY_LOGFILE}" &
_PROXY_PID=$!
sleep 1   # let proxy bind

# ── Mutate AMS settings ────────────────────────────────────────────────────────
log "Mutating listenerHookURL → ${_PROXY_URL}"
_SETTINGS_MODIFIED="${EVIDENCE_DIR}/settings-modified.json"
jq --arg hookurl "${_PROXY_URL}" \
   '.listenerHookURL = $hookurl' \
   "${EVIDENCE_DIR}/settings-before.json" > "${_SETTINGS_MODIFIED}"

curl -s -m 20 \
  -X PUT \
  -H "Content-Type: application/json" \
  -b "${AMS_COOKIE_FILE}" \
  --data-binary "@${_SETTINGS_MODIFIED}" \
  "${AMS_URL}/rest/v2/applications/settings/${APP}" > /dev/null
_SETTINGS_MUTATED=1
log "Settings mutated — AMS hooks now forwarded through signing proxy"

# ── Publish ~10 s, stop, wait for hooks to drain ─────────────────────────────
log "Starting publisher ${STREAM_ID} on ${APP} @ 1000 kbps"
start_publisher "${STREAM_ID}" "${APP}" 1000
_PUB_STARTED=1
sleep 10
log "Stopping publisher"
stop_publisher "${STREAM_ID}"
_PUB_STARTED=0
log "Publisher stopped — sleeping 8 s for hook drain"
sleep 8

# ── Tampered-signature probe (direct POST to Pulse, skip proxy) ───────────────
log "Posting tampered X-Ams-Signature to Pulse webhook endpoint"
_TAMPERED_BODY='{"action":"liveStreamStarted","streamId":"val-wh11-tamper","app":"LiveApp"}'
_TAMPER_HTTP_CODE="$(curl -s -m 10 -o /dev/null -w '%{http_code}' \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Ams-Signature: sha256=deadbeef" \
  -d "${_TAMPERED_BODY}" \
  "${PULSE_BASE}/webhook/ams" 2>/dev/null || echo 000)"
log "Tampered-signature response code: ${_TAMPER_HTTP_CODE}"

# ── Parse proxy log ────────────────────────────────────────────────────────────
_STARTED_STATUS="$(jq -rs \
  '[.[] | select(.action == "liveStreamStarted")] | first | .pulse_status // -1' \
  "${_PROXY_LOGFILE}" 2>/dev/null || echo -1)"
log "liveStreamStarted pulse_status: ${_STARTED_STATUS}"

_EVENT_SUMMARY="$(jq -rs 'group_by(.action) | map({action: .[0].action, count: length})' \
  "${_PROXY_LOGFILE}" 2>/dev/null || echo '[]')"
printf '[INFO] proxy_event_summary: %s\n' "${_EVENT_SUMMARY}" >> "${EVIDENCE_DIR}/timeline.txt"
printf '[INFO] tampered_http_code: %s\n' "${_TAMPER_HTTP_CODE}" >> "${EVIDENCE_DIR}/timeline.txt"
printf '[INFO] liveStreamStarted_pulse_status: %s\n' "${_STARTED_STATUS}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ─────────────────────────────────────────────────────────────────
log "ASSERT: signed liveStreamStarted delivered with pulse_status 200"
_delivered_ok="no"
[ "${_STARTED_STATUS}" = "200" ] && _delivered_ok="yes" || true
assert_eq "${_delivered_ok}" "yes" \
  "${SCENARIO} signed liveStreamStarted accepted by Pulse (pulse_status=200; got=${_STARTED_STATUS})" || true

log "ASSERT: tampered X-Ams-Signature rejected with HTTP 401"
assert_eq "${_TAMPER_HTTP_CODE}" "401" \
  "${SCENARIO} tampered X-Ams-Signature: sha256=deadbeef rejected with 401 (got: ${_TAMPER_HTTP_CODE})" || true

# ── Verdict ────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
