#!/usr/bin/env bash
# qa/realams/scenarios/TC-A-06-qoe-rebuffer.sh
#
# TC-A-06: QoE summary — rebuffer_ratio ≈ 0.2 after beacon rebuffer+heartbeat events
#
# Assertion matrix row:
#   Steps:     1. Mint ingest token (kind=ingest) via /admin/tokens
#              2. POST beacon batch to :18091/ingest/beacon with X-Pulse-Ingest-Token
#                 Events: rebuffer_end {duration_ms: 2000} + heartbeat {watch_ms: 10000}
#              3. rebuffer_ratio = rebuffer_total_ms / watch_time_ms = 2000/10000 = 0.2
#              4. Poll /qoe/summary for rebuffer_ratio ≈ 0.2 (±0.05) within ≤120 s
#              5. Revoke ingest token in trap
#   AMS truth: N/A (client-side beacon)
#   Pulse assert: /qoe/summary.totals.rebuffer_ratio ≈ 0.2 (±0.05 absolute)
#   Tolerance:  ±0.05 absolute; 120 s poll budget (rollup_qoe_1h latency)
#   Exit:       0 PASS | 1 FAIL | 77 SKIP (ingest token mint failed — license gate)
#
# ASSUMPTION: realams stack rebuffer_ratio baseline is 0 or near 0 before this run.
# If prior sessions had rebuffers, the aggregated ratio may differ from 0.2.
#
# Beacon ingest URL: http://127.0.0.1:18091/ingest/beacon (port 18091 = container 8091)
#
set -euo pipefail

SCENARIO="TC-A-06"
echo "=== ${SCENARIO}: QoE rebuffer_ratio ≈ 0.2 via beacon ===" >&2

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
STREAM_ID="val-a06-${EPOCH}"
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
log "Minting ingest token (kind=ingest, name=val-a06-${EPOCH})"
_token_http="$(curl -s -m 15 \
  -X POST \
  -H "Authorization: Bearer ${PULSE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"kind\":\"ingest\",\"name\":\"val-a06-${EPOCH}\"}" \
  -o "${EVIDENCE_DIR}/token-create.json" \
  -w '%{http_code}' \
  "${PULSE_URL}/admin/tokens" 2>/dev/null || echo 000)"

_token_resp="$(jq -c '.' "${EVIDENCE_DIR}/token-create.json" 2>/dev/null || echo '{}')"
TOKEN_ID="$(printf '%s' "${_token_resp}" | jq -r '.id // empty' 2>/dev/null || true)"
INGEST_TOKEN="$(printf '%s' "${_token_resp}" | jq -r '.token // empty' 2>/dev/null || true)"
log "Token mint HTTP=${_token_http}  id=${TOKEN_ID:-EMPTY}  token_prefix=${INGEST_TOKEN:0:10}..."

if [ "${_token_http}" != "201" ] || [ -z "${INGEST_TOKEN}" ]; then
  log "SKIP: token mint returned HTTP ${_token_http} — possible license gate on beacon ingest"
  printf 'SKIP\nPrecondition unmet: could not mint ingest token via /admin/tokens.\nHTTP=%s — beacon ingest may require Pro/Business license tier.\nResponse: %s\n' \
    "${_token_http}" "${_token_resp}" > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Step 2: POST beacon batch with rebuffer_end + heartbeat ───────────────────
# rollup_qoe_1h MV formula: rebuffer_ratio = rebuffer_total_ms / watch_time_ms
# rebuffer_end.data.duration_ms=2000 → rebuffer_total_ms += 2000
# heartbeat.data.watch_ms=10000   → watch_time_ms += 10000
# ratio = 2000 / 10000 = 0.2
log "Building beacon payload: rebuffer_end duration_ms=2000 + heartbeat watch_ms=10000"
log "Expected rebuffer_ratio: 2000/10000 = 0.2"

_beacon_body="$(python3 - <<PYEOF 2>/dev/null || printf '{}'
import uuid, json, time
now_ms = int(time.time() * 1000)
payload = {
    "version": 1,
    "session_id": str(uuid.uuid4()),
    "stream_id": "${STREAM_ID}",
    "app": "LiveApp",
    "events": [
        {"type": "rebuffer_end", "ts": now_ms,        "data": {"duration_ms": 2000}},
        {"type": "heartbeat",    "ts": now_ms + 1000, "data": {"watch_ms": 10000}}
    ]
}
print(json.dumps(payload))
PYEOF
)"

if [ "${_beacon_body}" = '{}' ]; then
  # Fallback: build without python3
  _SID="val-a06-${EPOCH}-fallback"
  _NOW_MS="$(( EPOCH * 1000 ))"
  _beacon_body="{\"version\":1,\"session_id\":\"${_SID}\",\"stream_id\":\"${STREAM_ID}\",\"app\":\"LiveApp\",\"events\":[{\"type\":\"rebuffer_end\",\"ts\":${_NOW_MS},\"data\":{\"duration_ms\":2000}},{\"type\":\"heartbeat\",\"ts\":$(( _NOW_MS + 1000 )),\"data\":{\"watch_ms\":10000}}]}"
  log "Using fallback beacon payload (python3 not available)"
fi

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
log "Beacon POST HTTP=${_beacon_http}  accepted=${_accepted}  (expected 2: rebuffer_end+heartbeat)"
printf 'beacon_http=%s\naccepted=%s\n' "${_beacon_http}" "${_accepted}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

if [ "${_beacon_http}" != "202" ]; then
  log "FAIL: beacon POST returned HTTP ${_beacon_http} (expected 202)"
  assert_eq "${_beacon_http}" "202" "${SCENARIO} beacon POST accepted (HTTP 202)" || true
  scenario_verdict
  exit 1
fi
assert_eq "${_beacon_http}" "202" "${SCENARIO} beacon POST accepted (HTTP 202)" || true

# ── Step 3: Poll /qoe/summary for rebuffer_ratio ≈ 0.2 (budget: 120 s) ───────
log "Polling /qoe/summary for rebuffer_ratio ≈ 0.2 ±0.05  (budget: 120 s, 5 s interval)"
_ratio_val="0"
_ratio_gt0="no"
_ratio_conv_s=999
_i=0
while [ "${_i}" -lt 24 ]; do
  sleep 5
  _qoe_resp="$(curl -s -m 15 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/qoe/summary" 2>/dev/null || echo '{}')"
  _ratio_val="$(printf '%s' "${_qoe_resp}" | \
    jq '.totals.rebuffer_ratio // 0' 2>/dev/null || echo 0)"
  _ratio_gt0="$(awk -v v="${_ratio_val}" 'BEGIN { print (v > 0) ? "yes" : "no" }')"
  if [ "${_ratio_gt0}" = "yes" ]; then
    _ratio_conv_s=$(( (_i + 1) * 5 ))
    log "rebuffer_ratio=${_ratio_val} appeared after ${_ratio_conv_s} s"
    break
  fi
  log "rebuffer_ratio=${_ratio_val} (attempt $(( _i + 1 ))/24, elapsed $(( (_i + 1) * 5 )) s)"
  _i=$(( _i + 1 ))
done

capture_pulse "/qoe/summary" "qoe-summary"
printf '%s' "${_qoe_resp:-{}}" | jq . > "${EVIDENCE_DIR}/qoe-summary.json" 2>/dev/null || true

log "Final rebuffer_ratio=${_ratio_val}  convergence_s=${_ratio_conv_s}"
printf 'rebuffer_ratio=%s\nratio_convergence_s=%s\n' \
  "${_ratio_val}" "${_ratio_conv_s}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ───────────────────────────────────────────────────────────────
# rebuffer_ratio ≈ 0.2 ±0.05 absolute (sent 2000ms rebuffer / 10000ms watch)
# Use assert_within for absolute tolerance
assert_within "${_ratio_val}" "0.2" "0.05" "${SCENARIO} rebuffer_ratio ≈ 0.2 (±0.05 abs; 2000/10000)" || true
assert_lte "${_ratio_conv_s}" 120 "${SCENARIO} rebuffer_ratio appeared within ≤120 s (rollup latency)" || true

# ── Verdict ───────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
