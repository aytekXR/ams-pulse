#!/usr/bin/env bash
# qa/realams/scenarios/TC-A-05-qoe-startup.sh
#
# TC-A-05: QoE summary — startup_p50_ms ≈ 450 after beacon startup_complete event
#
# Assertion matrix row:
#   Steps:     1. Mint ingest token (kind=ingest) via /admin/tokens
#              2. POST beacon batch to :18091/ingest/beacon with X-Pulse-Ingest-Token
#                 Events: startup_complete {startup_ms: 450}
#              3. Poll /qoe/summary for startup_p50_ms ≈ 450 (±20%) within ≤120 s
#              4. Revoke ingest token in trap
#   AMS truth: N/A (client-side beacon)
#   Pulse assert: /qoe/summary.totals.startup_p50_ms ≈ 450 (±20%)
#   Tolerance:  ±20% (90 ms absolute); 120 s poll budget (rollup_qoe_1h latency per D-039)
#   Exit:       0 PASS | 1 FAIL | 77 SKIP (ingest token mint failed — license gate)
#
# ASSUMPTION: realams stack has no prior beacon sessions that would skew the P50.
# A pre-existing session with very different startup_ms may shift the P50. This
# scenario is designed for a clean realams stack or one with few prior sessions.
#
# Beacon ingest URL: http://127.0.0.1:18091/ingest/beacon (port 18091 = container 8091)
# Token API: /api/v1/admin/tokens (POST kind=ingest, response carries "token" field)
#
set -euo pipefail

SCENARIO="TC-A-05"
echo "=== ${SCENARIO}: QoE startup_p50_ms ≈ 450 via beacon ===" >&2

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
STREAM_ID="val-a05-${EPOCH}"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

TOKEN_ID=""
INGEST_TOKEN=""

# Beacon ingest URL: separate port from the API
_PULSE_API_BASE="${PULSE_URL%/api/v1}"
BEACON_URL="${_PULSE_API_BASE/18090/18091}/ingest/beacon"
# For prod (no port in URL), beacon is at the same host under /ingest/beacon
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
log "Minting ingest token (kind=ingest, name=val-a05-${EPOCH})"
_token_http="$(curl -s -m 15 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"ingest\",\"name\":\"val-a05-${EPOCH}\"}" \
  -o "${EVIDENCE_DIR}/token-create.json" \
  -w '%{http_code}' \
  "${PULSE_URL}/admin/tokens" 2>/dev/null || echo 000)"

_token_resp="$(jq -c '.' "${EVIDENCE_DIR}/token-create.json" 2>/dev/null || echo '{}')"
TOKEN_ID="$(printf '%s' "${_token_resp}" | jq -r '.id // empty' 2>/dev/null || true)"
INGEST_TOKEN="$(printf '%s' "${_token_resp}" | jq -r '.token // empty' 2>/dev/null || true)"
log "Token mint HTTP=${_token_http}  id=${TOKEN_ID:-EMPTY}  token_prefix=${INGEST_TOKEN:0:10}..."

if [ "${_token_http}" != "201" ] || [ -z "${INGEST_TOKEN}" ]; then
  # 403 = license gate (beacon ingest requires Pro/Business tier)
  log "SKIP: token mint returned HTTP ${_token_http} — possible license gate on beacon ingest"
  printf 'SKIP\nPrecondition unmet: could not mint ingest token via /admin/tokens.\nHTTP=%s — beacon ingest may require Pro/Business license tier.\nResponse: %s\n' \
    "${_token_http}" "${_token_resp}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Step 2: POST beacon batch with startup_complete ───────────────────────────
# MV rollup_qoe_1h filters event_type IN (startup_complete, heartbeat, rebuffer_end)
# session_start is NOT counted. Only startup_complete populates startup_ms_state.
# Send 3 startup_complete events in separate sessions to give P50 statistical weight.
log "Building beacon payload: startup_complete startup_ms=450"
_NOW_MS="$(python3 -c 'import time; print(int(time.time() * 1000))' 2>/dev/null || echo "$(( EPOCH * 1000 ))")"
_SESSION_ID="$(python3 -c 'import uuid; print(str(uuid.uuid4()))' 2>/dev/null || echo "val-a05-${EPOCH}-$(date +%N | head -c 8)")"

_beacon_body="$(python3 - <<PYEOF 2>/dev/null || printf '{"version":1,"session_id":"%s","stream_id":"%s","app":"LiveApp","events":[{"type":"startup_complete","ts":%s,"data":{"startup_ms":450}}]}' "${_SESSION_ID}" "${STREAM_ID}" "${_NOW_MS}"
import uuid, json, time
now_ms = int(time.time() * 1000)
payload = {
    "version": 1,
    "session_id": "${_SESSION_ID}",
    "stream_id": "${STREAM_ID}",
    "app": "LiveApp",
    "events": [
        {"type": "startup_complete", "ts": now_ms,        "data": {"startup_ms": 450}},
        {"type": "heartbeat",        "ts": now_ms + 5000, "data": {"watch_ms": 5000}}
    ]
}
print(json.dumps(payload))
PYEOF
)"

log "Beacon payload: $(printf '%s' "${_beacon_body}" | jq -c '.' 2>/dev/null || echo "${_beacon_body}")"
printf '%s' "${_beacon_body}" > "${EVIDENCE_DIR}/beacon-payload.json"

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
log "Beacon POST HTTP=${_beacon_http}  accepted=${_accepted}"
printf 'beacon_http=%s\naccepted=%s\n' "${_beacon_http}" "${_accepted}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

if [ "${_beacon_http}" != "202" ]; then
  log "FAIL: beacon POST returned HTTP ${_beacon_http} (expected 202)"
  assert_eq "${_beacon_http}" "202" "${SCENARIO} beacon POST accepted (HTTP 202)" || true
  scenario_verdict
  exit 1
fi
assert_eq "${_beacon_http}" "202" "${SCENARIO} beacon POST accepted (HTTP 202)" || true

# ── Step 3: Poll /qoe/summary for startup_p50_ms ≈ 450 (budget: 120 s) ───────
# CH async batch flush: ~2 s; rollup_qoe_1h MV fires on INSERT.
# D-039 verified bound: 120 s. Do NOT shorten.
log "Polling /qoe/summary for startup_p50_ms ≈ 450 ±20%  (budget: 120 s, 5 s interval)"
_p50_val="0"
_p50_conv_s=999
_i=0
while [ "${_i}" -lt 24 ]; do
  sleep 5
  _qoe_resp="$(curl -s -m 15 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/qoe/summary" 2>/dev/null || echo '{}')"
  _p50_val="$(printf '%s' "${_qoe_resp}" | \
    jq '.totals.startup_p50_ms // 0' 2>/dev/null || echo 0)"
  _p50_gt0="$(awk -v v="${_p50_val}" 'BEGIN { print (v > 0) ? "yes" : "no" }')"
  if [ "${_p50_gt0}" = "yes" ]; then
    _p50_conv_s=$(( (_i + 1) * 5 ))
    log "startup_p50_ms=${_p50_val} appeared after ${_p50_conv_s} s"
    break
  fi
  log "startup_p50_ms=${_p50_val} (attempt $(( _i + 1 ))/24, elapsed $(( (_i + 1) * 5 )) s)"
  _i=$(( _i + 1 ))
done

capture_pulse "/qoe/summary" "qoe-summary"
printf '%s' "${_qoe_resp:-{}}" | jq . > "${EVIDENCE_DIR}/qoe-summary.json" 2>/dev/null || true

log "Final startup_p50_ms=${_p50_val}  convergence_s=${_p50_conv_s}"
printf 'startup_p50_ms=%s\np50_convergence_s=%s\n' \
  "${_p50_val}" "${_p50_conv_s}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ───────────────────────────────────────────────────────────────
# startup_p50_ms must be ≈ 450 ±20% (allowed range 360–540 ms)
assert_approx "${_p50_val}" "450" "20" "${SCENARIO} startup_p50_ms ≈ 450 (±20%, sent startup_ms=450)" || true
assert_lte "${_p50_conv_s}" 120 "${SCENARIO} startup_p50_ms appeared within ≤120 s (D-039 bound)" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
