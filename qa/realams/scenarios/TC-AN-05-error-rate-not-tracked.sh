#!/usr/bin/env bash
# qa/realams/scenarios/TC-AN-05-error-rate-not-tracked.sh
#
# TC-AN-05: error_rate NOT tracked by anomaly detector — known F9 gap confirmed
#
# Assertion matrix row:
#   Steps:     1. Mint ingest token (kind=ingest) via /admin/tokens
#              2. POST beacon batch with multiple "error" events to :18091/ingest/beacon
#              3. Wait 60 s (≥ anomaly detector tick; baseline needs error_rate metric)
#              4. Assert /anomalies has NO error_rate findings
#              5. PASS means the F9 gap is confirmed (not an unexpected failure)
#   AMS truth: N/A (client-side beacon)
#   Pulse assert: /anomalies items[] contains no error_rate anomaly flags
#   Exit:       0 PASS | 1 FAIL | 77 SKIP (ingest token mint failed)
#
# PASS semantics: error_rate is intentionally NOT tracked by the anomaly detector
# (capability-map.md §F9: "NOT tracked by anomaly detector: error_rate (from beacon
# rollups) — filed as F9 finding-1"). A PASS confirms this documented gap is still
# present; a FAIL would indicate error_rate detection was implemented.
#
# Capability map cross-link:
#   docs/assessment/capability-map.md §F9 / "NOT tracked by anomaly detector"
#   server/internal/anomaly/anomaly.go — anomaly metrics: viewer_count, cpu_pct, mem_pct
#
# Beacon ingest URL: http://127.0.0.1:18091/ingest/beacon (port 18091 = container 8091)
#
set -euo pipefail

SCENARIO="TC-AN-05"
echo "=== ${SCENARIO}: error_rate NOT tracked by anomaly detector (F9 gap) ===" >&2

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run identifiers ──────────────────────────────────────────────────────
EPOCH="$(date +%s)"
STREAM_ID="val-an05-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

TOKEN_ID=""
INGEST_TOKEN=""

# Beacon ingest URL
_PULSE_API_BASE="${PULSE_URL%/api/v1}"
BEACON_URL="${_PULSE_API_BASE/18090/18091}/ingest/beacon"
if [ "${PULSE_TARGET:-realams}" = "prod" ]; then
  BEACON_URL="${_PULSE_API_BASE}/ingest/beacon"
fi

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Cleanup trap ─────────────────────────────────────────────────────────────
cleanup() {
  if [ -n "${TOKEN_ID}" ]; then
    log "CLEANUP: revoking ingest token ${TOKEN_ID}"
    curl -s -m 10 -X DELETE \
      -H "Authorization: Bearer ${PULSE_TOKEN}" \
      "${PULSE_URL}/admin/tokens/${TOKEN_ID}" > /dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

log "STREAM_ID=${STREAM_ID}  PULSE_URL=${PULSE_URL}  BEACON_URL=${BEACON_URL}"

# ── Step 1: Mint ingest token ─────────────────────────────────────────────────
log "Minting ingest token (kind=ingest, name=val-an05-${EPOCH})"
_token_http="$(curl -s -m 15 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"ingest\",\"name\":\"val-an05-${EPOCH}\"}" \
  -o "${EVIDENCE_DIR}/token-create.json" \
  -w '%{http_code}' \
  "${PULSE_URL}/admin/tokens" 2>/dev/null || echo 000)"

_token_resp="$(jq -c '.' "${EVIDENCE_DIR}/token-create.json" 2>/dev/null || echo '{}')"
TOKEN_ID="$(printf '%s' "${_token_resp}" | jq -r '.id // empty' 2>/dev/null || true)"
INGEST_TOKEN="$(printf '%s' "${_token_resp}" | jq -r '.token // empty' 2>/dev/null || true)"
log "Token mint HTTP=${_token_http}  id=${TOKEN_ID:-EMPTY}"

if [ "${_token_http}" != "201" ] || [ -z "${INGEST_TOKEN}" ]; then
  log "SKIP: token mint returned HTTP ${_token_http} — possible license gate"
  printf 'SKIP\nPrecondition unmet: could not mint ingest token via /admin/tokens.\nHTTP=%s — beacon ingest may require Pro/Business license tier.\n' \
    "${_token_http}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Step 2: POST several "error" events ──────────────────────────────────────
# 5 error events in a single session to simulate high error rate.
# Error event schema (beacon-event.schema.json §ErrorData):
#   {"type": "error", "ts": <epoch_ms>, "data": {"code": "<code>", "fatal": true}}
log "Building beacon payload: 5 error events (MEDIA_ERR_DECODE, fatal=true)"

_beacon_body="$(python3 - <<PYEOF 2>/dev/null || printf '{}'
import uuid, json, time
now_ms = int(time.time() * 1000)
session_id = str(uuid.uuid4())
events = []
for i in range(5):
    events.append({
        "type": "error",
        "ts": now_ms + i * 100,
        "data": {
            "code": "MEDIA_ERR_DECODE",
            "message": "Decode error in test session",
            "fatal": True
        }
    })
payload = {
    "version": 1,
    "session_id": session_id,
    "stream_id": "${STREAM_ID}",
    "app": "LiveApp",
    "events": events
}
print(json.dumps(payload))
PYEOF
)"

if [ "${_beacon_body}" = '{}' ]; then
  # Fallback: build 5 error events without python3
  _SID="val-an05-${EPOCH}-fb"
  _NOW_MS="$(( EPOCH * 1000 ))"
  _beacon_body="{\"version\":1,\"session_id\":\"${_SID}\",\"stream_id\":\"${STREAM_ID}\",\"app\":\"LiveApp\",\"events\":["
  _beacon_body="${_beacon_body}{\"type\":\"error\",\"ts\":${_NOW_MS},\"data\":{\"code\":\"MEDIA_ERR_DECODE\",\"fatal\":true}},"
  _beacon_body="${_beacon_body}{\"type\":\"error\",\"ts\":$(( _NOW_MS + 100 )),\"data\":{\"code\":\"MEDIA_ERR_NETWORK\",\"fatal\":false}},"
  _beacon_body="${_beacon_body}{\"type\":\"error\",\"ts\":$(( _NOW_MS + 200 )),\"data\":{\"code\":\"MEDIA_ERR_DECODE\",\"fatal\":true}},"
  _beacon_body="${_beacon_body}{\"type\":\"error\",\"ts\":$(( _NOW_MS + 300 )),\"data\":{\"code\":\"MEDIA_ERR_DECODE\",\"fatal\":false}},"
  _beacon_body="${_beacon_body}{\"type\":\"error\",\"ts\":$(( _NOW_MS + 400 )),\"data\":{\"code\":\"MEDIA_ERR_DECODE\",\"fatal\":true}}"
  _beacon_body="${_beacon_body}]}"
  log "Using fallback beacon payload (python3 not available)"
fi

printf '%s' "${_beacon_body}" > "${EVIDENCE_DIR}/beacon-payload.json"
log "Beacon payload events: $(printf '%s' "${_beacon_body}" | jq '[.events | length] | .[0]' 2>/dev/null || echo "5") error events"

_beacon_http="$(curl -s -m 15 \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Pulse-Ingest-Token: ${INGEST_TOKEN}" \
  -d "${_beacon_body}" \
  -o "${EVIDENCE_DIR}/beacon-resp.json" \
  -w '%{http_code}' \
  "${BEACON_URL}" 2>/dev/null || echo 000)"

_beacon_resp="$(jq -c '.' "${EVIDENCE_DIR}/beacon-resp.json" 2>/dev/null || echo '{}')"
_accepted="$(printf '%s' "${_beacon_resp}" | jq '.accepted // -1' 2>/dev/null || echo -1)"
log "Beacon POST HTTP=${_beacon_http}  accepted=${_accepted}  (expected 5 error events)"
printf 'beacon_http=%s\naccepted=%s\n' "${_beacon_http}" "${_accepted}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

if [ "${_beacon_http}" != "202" ]; then
  log "FAIL: beacon POST returned HTTP ${_beacon_http} (expected 202)"
  assert_eq "${_beacon_http}" "202" "${SCENARIO} beacon error events accepted (HTTP 202)" || true
  scenario_verdict
  exit 1
fi
assert_eq "${_beacon_http}" "202" "${SCENARIO} beacon error events accepted (HTTP 202)" || true

# ── Step 3: Wait 60 s for anomaly detector tick ───────────────────────────────
# Anomaly detector tick: 60 s in prod, 5 s in CI/realams.
# error_rate would need to appear in anomaly_baselines and deviate — but it
# is intentionally NOT tracked (F9 finding-1). 60 s gives ample time for any
# ticker that might scan error_rate to run.
log "Waiting 60 s for anomaly detector to tick (if it were tracking error_rate)"
sleep 60
log "60 s elapsed — checking /anomalies"

# ── Step 4: Assert no error_rate anomaly findings ─────────────────────────────
capture_pulse "/anomalies" "anomalies"
_anomaly_http="$(curl -s -m 15 \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -o "${EVIDENCE_DIR}/anomalies.json" \
  -w '%{http_code}' \
  "${PULSE_URL}/anomalies" 2>/dev/null || echo 000)"

_anomaly_resp="$(jq -c '.' "${EVIDENCE_DIR}/anomalies.json" 2>/dev/null || echo '{}')"
log "Anomaly HTTP=${_anomaly_http}"

_error_rate_count="$(printf '%s' "${_anomaly_resp}" | \
  jq '[(.items // [])[] | select(.metric == "error_rate")] | length' \
  2>/dev/null || echo 0)"
_total_items="$(printf '%s' "${_anomaly_resp}" | \
  jq '(.items // []) | length' 2>/dev/null || echo 0)"
_all_metrics="$(printf '%s' "${_anomaly_resp}" | \
  jq -r '[(.items // [])[] | .metric] | unique | join(",")' 2>/dev/null || echo "")"

log "Total anomaly items: ${_total_items}  error_rate_count=${_error_rate_count}  all_metrics=${_all_metrics:-none}"
printf 'anomaly_http=%s\ntotal_items=%s\nerror_rate_count=%s\nall_metrics=%s\n' \
  "${_anomaly_http}" "${_total_items}" "${_error_rate_count}" "${_all_metrics:-none}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ───────────────────────────────────────────────────────────────
# /anomalies must return HTTP 200
assert_eq "${_anomaly_http}" "200" "${SCENARIO} /anomalies returns HTTP 200" || true

# No error_rate anomaly findings (F9 gap confirmed — PASS means gap is present)
assert_eq "${_error_rate_count}" "0" "${SCENARIO} no error_rate anomaly items (F9 gap: error_rate not tracked)" || true

# ── Document F9 gap ───────────────────────────────────────────────────────────
{
  printf '=== TC-AN-05: error_rate not tracked — F9 gap confirmed ===\n'
  printf 'Beacon events sent: 5 error events (MEDIA_ERR_DECODE) to stream %s\n' "${STREAM_ID}"
  printf 'Wait period: 60 s (≥ anomaly detector tick)\n'
  printf 'error_rate anomaly findings: %s (expected 0)\n' "${_error_rate_count}"
  printf 'Total anomaly findings: %s\n' "${_total_items}"
  printf 'All metrics in anomalies: %s\n' "${_all_metrics:-none}"
  printf '\nF9 GAP (capability-map.md §F9 finding-1):\n'
  printf '  "NOT tracked by anomaly detector: error_rate (from beacon rollups)"\n'
  printf '  The anomaly detector (server/internal/anomaly/anomaly.go) tracks:\n'
  printf '    viewer_count, cpu_pct, mem_pct\n'
  printf '  It does NOT track: error_rate, rebuffer_ratio\n'
  printf '\nPASS INTERPRETATION:\n'
  printf '  PASS confirms the gap is still present (expected behavior per F9 spec).\n'
  printf '  A FAIL would indicate error_rate detection was unexpectedly implemented.\n'
  printf '\nREFERENCES:\n'
  printf '  docs/assessment/capability-map.md §F9 ("NOT tracked by anomaly detector")\n'
  printf '  server/internal/anomaly/anomaly.go — anomaly metric list\n'
  printf '  scenario-matrix.md TC-AN-05\n'
} > "${EVIDENCE_DIR}/F9-error-rate-gap.txt"
log "F9 gap documented in ${EVIDENCE_DIR}/F9-error-rate-gap.txt"

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
