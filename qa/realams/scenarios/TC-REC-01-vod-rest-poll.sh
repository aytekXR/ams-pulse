#!/usr/bin/env bash
# qa/realams/scenarios/TC-REC-01-vod-rest-poll.sh
#
# TC-REC-01: VoD REST poll populates recording_gb — BUG-002 post-fix validation
#
# PURPOSE
#   Verify that after the BUG-002 fix is deployed, the Pulse VoD REST poll
#   correctly populates totals.recording_gb in GET /api/v1/reports/usage.
#   This scenario is intentionally silent/SKIP on stacks predating the fix.
#
# GATING — PULSE_HAS_VOD_POLL=1 must be set explicitly in the environment.
#   Without it this scenario exits 77 (SKIP) immediately, before any network
#   call.  This prevents false FAILs and false PASSes on pre-fix deployments.
#
#   Set PULSE_HAS_VOD_POLL=1 only after ALL THREE components are deployed:
#     1. server/pkg/amsclient: ListVods / ListVodsPaged
#     2. server/internal/collector/restpoller: pollVods + high-water-mark dedup
#     3. ClickHouse migration: mv_recording_1d materialized view
#   (per BUG-002-design-note-vod-rest-poll.md §3 and §4)
#
# GROUND TRUTH
#   S17 VoD fixture on the pulse-test AMS application (D-079 standing VoD;
#   agents/handoffs/decisions.md:3737-3738 "kept as standing ground truth").
#   AMS check: GET /pulse-test/rest/v2/vods/count → {"number": N}  (open-read,
#              no cookie needed; this scenario SKIPs if N < 1).
#   VoD size:  3125555 bytes (measured in S17 BUG-002 evidence run).
#   Expected:  3125555 / 1e9 = 0.003125555 GB
#              Derivation: accounting.go:281 const bytesToGB = 1.0 / 1e9,
#              line 306: recordingGB := float64(recordingBytes) * bytesToGB,
#              confirmed via codegraph S23 session.
#   Tolerance: ±20 % — covers AMS file-size rounding, ClickHouse flush lag,
#              MV materialisation delay, and VoD DTO field-name variations.
#
# STEPS
#   1. AMS probe: GET /pulse-test/rest/v2/vods/count.  Require number >= 1;
#      SKIP with reason if 0 (environment premise unmet, not a Pulse bug).
#   2. Bounded wait (<= 90 s, poll every 5 s): GET /api/v1/reports/usage
#      until totals.recording_gb > 0.  Uses jq -e for the numeric comparison.
#   3. Assertions:
#      A. recording_gb > 0  (primary fix verification)
#      B. recording_gb ≈ 0.003125555 GB ± 20 %  (byte-unit consistency)
#   4. Write fresh verdict/evidence artifacts per harness convention.
#
# EXIT CODES
#   0   PASS  — recording_gb > 0 AND within tolerance of expected byte total
#   1   FAIL  — recording_gb stayed 0 after 90 s, or value outside tolerance
#   77  SKIP  — PULSE_HAS_VOD_POLL unset (pre-fix stack), or pulse-test has
#              no VoDs (AMS environment premise unmet)
#
# RELATED
#   BUG-002:      docs/assessment/bugs/BUG-002-recording-gb-zero-webhook-blocked.md
#   Design note:  docs/assessment/bugs/BUG-002-design-note-vod-rest-poll.md §6.3
#   BUG-002 pre-fix evidence: TC-WH-03-vod-recording-gap.sh, TC-A-09-recording-zero.sh
#
set -euo pipefail

SCENARIO="TC-REC-01"
echo "=== ${SCENARIO}: VoD REST poll populates recording_gb (BUG-002 post-fix) ===" >&2

# ── Feature gate: SKIP on pre-fix stacks ─────────────────────────────────────
# Check BEFORE sourcing the harness so that stacks without the fix never reach
# the polling loop.  The Makefile treats exit 77 as SKIP (not FAIL).
if [ "${PULSE_HAS_VOD_POLL:-0}" != "1" ]; then
  echo "[${SCENARIO}] SKIP — PULSE_HAS_VOD_POLL is not set (assumed pre-fix stack)" >&2
  echo "  Set PULSE_HAS_VOD_POLL=1 only after deploying the BUG-002 VoD REST poll fix." >&2
  echo "  Required: amsclient.ListVods, restpoller.pollVods, mv_recording_1d MV." >&2
  exit 77
fi

# ── Harness bootstrap ────────────────────────────────────────────────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/auth.sh
source "${_DIR}/../harness/auth.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"
# shellcheck source=../harness/capture.sh
source "${_DIR}/../harness/capture.sh"

# ── Per-run evidence directory ────────────────────────────────────────────────
EVIDENCE_DIR="${EVIDENCE_ROOT}/S23-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ─────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

cleanup() { : ; }
trap cleanup EXIT

log "PULSE_URL=${PULSE_URL}  AMS_URL=${AMS_URL}"
log "PULSE_HAS_VOD_POLL=${PULSE_HAS_VOD_POLL}"

# ── Ground truth constants ────────────────────────────────────────────────────
# accounting.go:281  const bytesToGB = 1.0 / 1e9
# accounting.go:306  recordingGB := float64(v.recordingBytes) * bytesToGB
# S17 VoD fixture:   3125555 bytes  →  3125555 / 1e9 = 0.003125555 GB
readonly VOD_APP="pulse-test"
readonly VOD_BYTES_GROUND_TRUTH=3125555
readonly VOD_GB_EXPECTED="0.003125555"
readonly VOD_GB_TOLERANCE_PCT=20
readonly POLL_MAX_S=90
readonly POLL_INTERVAL_S=5

printf 'ground_truth_bytes=%s\nexpected_gb=%s\ntolerance_pct=%s\n' \
  "${VOD_BYTES_GROUND_TRUTH}" "${VOD_GB_EXPECTED}" "${VOD_GB_TOLERANCE_PCT}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── Step 1: AMS ground truth — vods/count for pulse-test ─────────────────────
# GET /{app}/rest/v2/vods/count is open-read on AMS 3.x (no cookie required).
# Confirmed from TC-WH-03 S17 fallback path and design note §6.3.
log "Step 1: AMS GET /${VOD_APP}/rest/v2/vods/count"
_vod_count_raw="$(curl -s -m 20 \
  "${AMS_URL}/${VOD_APP}/rest/v2/vods/count" 2>/dev/null || echo '{}')"
printf '%s' "${_vod_count_raw}" | jq . \
  > "${EVIDENCE_DIR}/ams-vods-count.json" 2>/dev/null || true

# Extract numeric count.  Use explicit null guard rather than // to document
# intent; .number is a JSON number (never boolean false), so // 0 would also
# be safe, but explicit is clearer given the harness false-green guidance.
_vod_count="$(printf '%s' "${_vod_count_raw}" | \
  jq 'if .number != null then .number else 0 end' 2>/dev/null || echo 0)"

log "AMS ${VOD_APP} vods/count: ${_vod_count}"
printf 'ams_vod_count=%s\n' "${_vod_count}" >> "${EVIDENCE_DIR}/timeline.txt"

# Premise check: pulse-test must have >= 1 VoD for the fix to have any data
# to poll.  SKIP rather than FAIL — absent VoDs are an environment problem,
# not a Pulse bug.
if ! awk -v n="${_vod_count}" 'BEGIN { exit (n >= 1) ? 0 : 1 }'; then
  {
    printf 'SKIP\n'
    printf 'Premise unmet: GET /%s/rest/v2/vods/count returned number=%s (need >= 1).\n' \
      "${VOD_APP}" "${_vod_count}"
    printf '\n'
    printf 'The S17 standing fixture VoD (D-079) may have been deleted from AMS.\n'
    printf 'Restore it: enable mp4MuxingEnabled=true on %s, publish a ~20 s RTMP\n' \
      "${VOD_APP}"
    printf 'stream to %s/<stream-id>, stop, restore mp4MuxingEnabled=false, re-run.\n' \
      "${VOD_APP}"
  } > "${EVIDENCE_DIR}/verdict.txt"
  cat "${EVIDENCE_DIR}/verdict.txt" >&2
  exit 77
fi

assert_gte "${_vod_count}" 1 \
  "${SCENARIO} AMS ${VOD_APP} vods/count >= 1 (VoD ground truth present)" || true

# ── Step 2: Bounded wait for recording_gb > 0 ────────────────────────────────
# VoD poll default interval: 60 s (design note §3.2 — one tick every ~60 s).
# On cold start (in-memory HWM = 0), all VoDs are emitted on the first cycle.
# 90 s budget: 60 s poll cycle + 30 s ClickHouse flush + MV materialisation.
#
# jq -e exits 0 when the expression evaluates to a truthy non-null value.
# '.totals.recording_gb > 0' returns the JSON boolean true/false — truthy when
# the field is positive, falsy when zero or absent.  No // operator is used
# here (harness landmine: jq // fires on false, not only null).
log "Step 2: polling /reports/usage every ${POLL_INTERVAL_S} s (max ${POLL_MAX_S} s)"

_elapsed=0
_recording_gb="0"
_usage_json="{}"

while [ "${_elapsed}" -lt "${POLL_MAX_S}" ]; do
  _usage_json="$(curl -s -m 20 \
    -H "Authorization: Bearer ${PULSE_TOKEN}" \
    "${PULSE_URL}/reports/usage" 2>/dev/null || echo '{}')"

  # Numeric extraction — jq outputs a bare number (no quotes), so -r is not
  # needed.  Explicit null guard documents intent.
  _recording_gb="$(printf '%s' "${_usage_json}" | \
    jq 'if .totals.recording_gb != null then .totals.recording_gb else 0 end' \
    2>/dev/null || echo "0")"

  log "  t=${_elapsed}s  recording_gb=${_recording_gb}"
  printf 'poll_t=%ss  recording_gb=%s\n' "${_elapsed}" "${_recording_gb}" \
    >> "${EVIDENCE_DIR}/timeline.txt"

  # jq -e exits 0 when '.totals.recording_gb > 0' is JSON true, 1 when false.
  # Redirect stdout to /dev/null — we only need the exit code here.
  if printf '%s' "${_usage_json}" | \
      jq -e '.totals.recording_gb > 0' > /dev/null 2>&1; then
    log "  recording_gb > 0 confirmed at t=${_elapsed}s"
    break
  fi

  sleep "${POLL_INTERVAL_S}"
  _elapsed=$(( _elapsed + POLL_INTERVAL_S ))
done

# Save final usage response for evidence archive.
printf '%s' "${_usage_json}" | jq . \
  > "${EVIDENCE_DIR}/pulse-usage-final.json" 2>/dev/null || true
capture_pulse "/reports/usage" "usage-post-wait"

printf 'wait_elapsed_s=%s\nfinal_recording_gb=%s\n' \
  "${_elapsed}" "${_recording_gb}" >> "${EVIDENCE_DIR}/timeline.txt"
log "Poll complete: elapsed=${_elapsed}s  final recording_gb=${_recording_gb}"

# ── Step 3: PASS assertions ───────────────────────────────────────────────────
log "Step 3: assertions"

# Assertion A: recording_gb > 0  (primary fix verification).
# There is no assert_gt helper in the harness (assert_gte tests >=, not >).
# Use _record_check directly with an awk result.  This function is defined in
# assert.sh (sourced above) and accumulates the result in checks.txt for
# scenario_verdict to read.
_a_status="$(awk -v gb="${_recording_gb}" \
  'BEGIN { print (gb > 0) ? "PASS" : "FAIL" }')"
_record_check "${_a_status}" \
  "${SCENARIO} recording_gb > 0 (VoD REST poll emitted recording event)" \
  "got=${_recording_gb}  elapsed=${_elapsed}s  max=${POLL_MAX_S}s" || true

# Assertion B: recording_gb within ±20 % of expected byte total.
# Expected: 3125555 bytes / 1e9 (accounting.go:281 bytesToGB) = 0.003125555 GB.
# Tolerance rationale:
#   - 20 % > rounding error from roundToDecimal(v, 6) (< 0.01 %)
#   - 20 % < 2x the expected value (catches double-count from restart without
#     persistent HWM, which would produce ~0.00625 GB — 100 % above expected)
#   - If AMS file size drifts by a few KB the check still passes; if the DTO
#     maps the wrong field (e.g. creationDate instead of fileSize, which would
#     produce ~1.7e12 GB), the check fails loudly.
assert_approx "${_recording_gb}" "${VOD_GB_EXPECTED}" "${VOD_GB_TOLERANCE_PCT}" \
  "${SCENARIO} recording_gb approx ${VOD_GB_EXPECTED} GB (${VOD_BYTES_GROUND_TRUTH} bytes / 1e9, tol=${VOD_GB_TOLERANCE_PCT}%)" \
  || true

# ── Verdict ───────────────────────────────────────────────────────────────────
# scenario_verdict reads checks.txt, writes verdict.txt (first line: PASS/FAIL),
# returns 0 on all-PASS or 1 on any FAIL.  The Makefile gates on:
#   rc==0 AND verdict.txt newer than pre-run stamp AND head -1 == PASS
# so a fresh positive verdict.txt is required — rc==0 alone is not enough.
log "Writing verdict"
scenario_verdict
exit $?
