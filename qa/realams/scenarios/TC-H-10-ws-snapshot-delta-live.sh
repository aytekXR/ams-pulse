#!/usr/bin/env bash
# qa/realams/scenarios/TC-H-10-ws-snapshot-delta-live.sh
#
# TC-H-10: WebSocket live feed — first frame is snapshot; delta frames appear while publishing
#
# Assertion matrix row:
#   Steps:     1. Precondition: python3 websockets module present
#              2. Open WS connection to /live/ws?token=...
#              3. Sleep 3 s — let initial snapshot frame arrive
#              4. Start publisher for ~12 s (aggregator push + 1 s rate limiter)
#              5. Stop publisher
#              6. Wait for WS client to finish (~45 s total window)
#              7. Assert first recorded frame type == "snapshot"
#              8. Assert at least one "delta" frame arrived after publish start
#   AMS truth: n/a (WebSocket channel is Pulse-internal)
#   Pulse assert: WS snapshot frame precedes any delta; delta appears during/after publish
#   Precondition: python3 with websockets module installed
#   Exit:      0 PASS | 1 FAIL | 77 SKIP
#
set -euo pipefail

SCENARIO="TC-H-10"
echo "=== ${SCENARIO}: WebSocket snapshot then delta ===" >&2

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
STREAM_ID="val-h10-$(openssl rand -hex 4)"
EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ───────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# ── Precondition: python3 websockets ──────────────────────────────────────────
if ! python3 -c "import websockets" 2>/dev/null; then
  printf 'SKIP\nPrecondition: python3 websockets module not available (pip install websockets)\n' \
    > "${EVIDENCE_DIR}/verdict.txt"
  exit 77
fi

# ── Build WebSocket URL from PULSE_URL ────────────────────────────────────────
_WS_BASE="${PULSE_URL}"
case "${_WS_BASE}" in
  https://*) _WS_BASE="wss${_WS_BASE#https}" ;;
  http://*)  _WS_BASE="ws${_WS_BASE#http}"   ;;
esac
_WS_URL="${_WS_BASE}/live/ws?token=${PULSE_TOKEN}"
log "WS_URL=${_WS_URL}"

# ── State flags ────────────────────────────────────────────────────────────────
_FRAMES_JSONL="${EVIDENCE_DIR}/ws-frames.jsonl"
_PY_SCRIPT="${EVIDENCE_DIR}/ws_client.py"
_PY_PID=""
_PUB_STARTED=0

# ── EXIT trap — set before any background process or publisher ─────────────────
cleanup() {
  if [ "${_PUB_STARTED}" = "1" ]; then
    stop_publisher "${STREAM_ID}" 2>/dev/null || true
  fi
  if [ -n "${_PY_PID}" ]; then
    kill "${_PY_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# ── Write python WS client to file ────────────────────────────────────────────
cat > "${_PY_SCRIPT}" << 'PYEOF'
import asyncio
import json
import sys
import time

import websockets


async def main():
    uri = sys.argv[1]
    outfile = sys.argv[2]
    duration = float(sys.argv[3])
    end_ts = time.time() + duration

    with open(outfile, "w") as fh:
        try:
            async with websockets.connect(uri) as ws:
                while time.time() < end_ts:
                    try:
                        msg = await asyncio.wait_for(ws.recv(), timeout=2.0)
                        try:
                            data = json.loads(msg)
                        except Exception:
                            data = {}
                        entry = {
                            "ts": time.time(),
                            "type": data.get("type", "unknown"),
                        }
                        fh.write(json.dumps(entry) + "\n")
                        fh.flush()
                    except asyncio.TimeoutError:
                        pass
                    except Exception as exc:
                        fh.write(
                            json.dumps(
                                {"ts": time.time(), "type": "error", "msg": str(exc)}
                            )
                            + "\n"
                        )
                        fh.flush()
                        break
        except Exception as exc:
            fh.write(
                json.dumps({"ts": time.time(), "type": "connect_error", "msg": str(exc)})
                + "\n"
            )


asyncio.run(main())
PYEOF

# ── Start WS client in background (45 s window) ───────────────────────────────
log "Launching WS client (45 s window) → ${_FRAMES_JSONL}"
python3 "${_PY_SCRIPT}" "${_WS_URL}" "${_FRAMES_JSONL}" 45 &
_PY_PID=$!

# ── Sleep 3 s — let snapshot arrive ───────────────────────────────────────────
log "Sleeping 3 s — waiting for snapshot frame"
sleep 3

# ── Start publisher ────────────────────────────────────────────────────────────
log "Starting publisher ${STREAM_ID} on ${APP} @ 1000 kbps"
start_publisher "${STREAM_ID}" "${APP}" 1000
_PUB_STARTED=1
_PUB_START_TS="$(date +%s)"
log "Publisher started at ${_PUB_START_TS}"

# Let the aggregator push and the 1 s rate limiter fire at least once
sleep 12

# ── Stop publisher ─────────────────────────────────────────────────────────────
log "Stopping publisher"
stop_publisher "${STREAM_ID}"
_PUB_STARTED=0
log "Publisher stopped"

# ── Wait for python client to finish ──────────────────────────────────────────
log "Waiting for WS client (up to 30 s remaining)"
wait "${_PY_PID}" 2>/dev/null || true
_PY_PID=""

_FRAME_COUNT="$(wc -l < "${_FRAMES_JSONL}" 2>/dev/null | tr -d ' ' || echo 0)"
log "WS client done. Frames collected: ${_FRAME_COUNT}"

# ── Frame summary ─────────────────────────────────────────────────────────────
jq -rs 'group_by(.type) | map({type: .[0].type, count: length})' \
  "${_FRAMES_JSONL}" > "${EVIDENCE_DIR}/ws-frame-summary.json" 2>/dev/null || true

# ── Parse assertions ───────────────────────────────────────────────────────────
_FIRST_TYPE="$(head -1 "${_FRAMES_JSONL}" 2>/dev/null \
  | jq -r '.type // "none"' 2>/dev/null || echo "none")"
log "First frame type: ${_FIRST_TYPE}"

_DELTA_AFTER_PUB="$(jq -rs --argjson pts "${_PUB_START_TS}" \
  '[.[] | select(.type == "delta" and .ts >= $pts)] | length' \
  "${_FRAMES_JSONL}" 2>/dev/null || echo 0)"
log "Delta frames after publish start (ts >= ${_PUB_START_TS}): ${_DELTA_AFTER_PUB}"

{
  printf 'ws_url: %s\n' "${_WS_URL}"
  printf 'pub_start_ts: %s\n' "${_PUB_START_TS}"
  printf 'frame_total: %s\n' "${_FRAME_COUNT}"
  printf 'first_frame_type: %s\n' "${_FIRST_TYPE}"
  printf 'delta_frames_after_pub: %s\n' "${_DELTA_AFTER_PUB}"
} >> "${EVIDENCE_DIR}/timeline.txt"

# ── Assertions ─────────────────────────────────────────────────────────────────
log "ASSERT: first WS frame type is snapshot"
assert_eq "${_FIRST_TYPE}" "snapshot" \
  "${SCENARIO} first WS frame type is snapshot (got: ${_FIRST_TYPE})" || true

log "ASSERT: at least one delta frame after publish start"
_delta_ok="no"
[ "${_DELTA_AFTER_PUB}" -gt 0 ] 2>/dev/null && _delta_ok="yes" || true
assert_eq "${_delta_ok}" "yes" \
  "${SCENARIO} delta frame(s) received after publishing started" || true

# ── Verdict ────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
