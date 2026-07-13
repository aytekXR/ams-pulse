#!/usr/bin/env bash
# qa/realams/scenarios/TC-V-09-broadcaststatistics-deadcode.sh
#
# TC-V-09: BroadcastStatistics dead-code ABSENCE confirmation (BUG-001 FIXED)
#
# HISTORY: through S25 this scenario confirmed the dead code EXISTED with no
# callers (BUG-001 open: method at client.go:483, tested, never called).
# S26/D-088 FIXED BUG-001 by deleting the method + DTO + test + fixture, so
# the assertions inverted: the scenario now pins TOTAL ABSENCE — a
# reintroduction of BroadcastStatistics without a runtime consumer would
# regress BUG-001 and must fail here.
#
# Assertion matrix row (scenario-matrix.md):
#   Steps:   1. grep -rn BroadcastStatistics server/ (production .go files only)
#            2. Assert caller count OUTSIDE amsclient/client.go = 0
#            3. Assert definition references in client.go = 0  (deleted S26)
#            4. grep test files; assert test references = 0  (deleted S26)
#   AMS truth:    N/A (code inspection only)
#   Pulse assert: grep returns NOTHING anywhere in server/ — the symbol is gone.
#                 Viewer counts come from inline BroadcastDTO fields (the REAL
#                 poll path); the endpoint's wire shape stays documented in
#                 agents/handoffs/real-ams-captures/broadcast-statistics_test123.json
#                 and the qa/mock-ams /statistics stub is retained deliberately.
#   Verdict:      PASS confirms the deletion holds; FAIL means BroadcastStatistics
#                 was reintroduced — either wire it to a real consumer or delete it.
#   Exit:    0 PASS | 1 FAIL (never 77 SKIP — premise is always met for grep)
#
# This script performs read-only grep only (no network calls, no docker) and
# may be run at AUTHOR TIME for self-validation.
#
# BINDING RULES (memory: shell-harness-false-green-patterns):
#   - every assert_* ends with || true
#   - grep -c gets || true (never || echo 0)
#
set -euo pipefail

SCENARIO="TC-V-09"
echo "=== ${SCENARIO}: BroadcastStatistics dead-code confirmation (BUG-001) ===" >&2

# ── Harness bootstrap (env only — no auth, no docker, no publisher) ────────────
_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../harness/env.sh
source "${_DIR}/../harness/env.sh"
# shellcheck source=../harness/assert.sh
source "${_DIR}/../harness/assert.sh"

# ── Per-run identifiers ────────────────────────────────────────────────────────
EVIDENCE_DIR="${EVIDENCE_ROOT}/S18-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "${EVIDENCE_DIR}"
export EVIDENCE_DIR

# ── Timeline log ───────────────────────────────────────────────────────────────
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*" | tee -a "${EVIDENCE_DIR}/timeline.txt" >&2; }

# No cleanup needed (read-only grep scenario)
log "REPO_ROOT=${REPO_ROOT}  EVIDENCE_DIR=${EVIDENCE_DIR}"
log "BUG-001: BroadcastStatistics dead-code confirmation"

# ── 1. Grep production files (exclude _test.go) ────────────────────────────────
log "Grepping server/ for BroadcastStatistics (production .go files only)"
_PROD_GREP="$(grep -rn "BroadcastStatistics" \
  "${REPO_ROOT}/server" \
  --include="*.go" \
  2>/dev/null \
  | grep -v "_test\.go" \
  || true)"

# Save full grep output to evidence
printf '%s\n' "${_PROD_GREP}" > "${EVIDENCE_DIR}/grep-prod-BroadcastStatistics.txt"
log "Production grep output written to evidence/grep-prod-BroadcastStatistics.txt"
log "Production grep results:"
if [ -n "${_PROD_GREP}" ]; then
  printf '%s\n' "${_PROD_GREP}" >&2
else
  echo "(no production references found)" >&2
fi

# ── 2. Count production references outside the definition file ────────────────
# The definition file is server/pkg/amsclient/client.go.
# Any reference in OTHER files = a caller (which should not exist per BUG-001).
_CALLER_LINES=""
if [ -n "${_PROD_GREP}" ]; then
  _CALLER_LINES="$(printf '%s\n' "${_PROD_GREP}" \
    | grep -v "pkg/amsclient/client\.go" \
    || true)"
fi

_CALLER_COUNT=0
if [ -n "${_CALLER_LINES}" ]; then
  _CALLER_COUNT="$(printf '%s\n' "${_CALLER_LINES}" | grep -c "." || true)"
fi

log "Production callers (outside client.go): ${_CALLER_COUNT}"
if [ -n "${_CALLER_LINES}" ]; then
  log "Unexpected callers found:"
  printf '%s\n' "${_CALLER_LINES}" >&2
fi

# ── 3. Count definition references in client.go ───────────────────────────────
_DEFN_LINES=""
if [ -n "${_PROD_GREP}" ]; then
  _DEFN_LINES="$(printf '%s\n' "${_PROD_GREP}" \
    | grep "pkg/amsclient/client\.go" \
    || true)"
fi

_DEFN_COUNT=0
if [ -n "${_DEFN_LINES}" ]; then
  _DEFN_COUNT="$(printf '%s\n' "${_DEFN_LINES}" | grep -c "." || true)"
fi

log "Definition references in client.go: ${_DEFN_COUNT}"
log "Definition lines:"
printf '%s\n' "${_DEFN_LINES:-  (none)}" >&2

# ── 4. Grep test files to confirm test coverage ────────────────────────────────
log "Grepping server/ for BroadcastStatistics in test files"
_TEST_GREP="$(grep -rn "BroadcastStatistics" \
  "${REPO_ROOT}/server" \
  --include="*_test.go" \
  2>/dev/null \
  || true)"

printf '%s\n' "${_TEST_GREP}" > "${EVIDENCE_DIR}/grep-test-BroadcastStatistics.txt"
log "Test grep results:"
if [ -n "${_TEST_GREP}" ]; then
  printf '%s\n' "${_TEST_GREP}" >&2
else
  echo "(no test references found)" >&2
fi

_TEST_COUNT=0
if [ -n "${_TEST_GREP}" ]; then
  _TEST_COUNT="$(printf '%s\n' "${_TEST_GREP}" | grep -c "." || true)"
fi

log "Test references: ${_TEST_COUNT}"

# ── 5. Write summary record to evidence ───────────────────────────────────────
jq -n \
  --argjson caller_count "${_CALLER_COUNT}" \
  --argjson defn_count "${_DEFN_COUNT}" \
  --argjson test_count "${_TEST_COUNT}" \
  --arg verdict_pre "BUG-001: BroadcastStatistics dead code" \
  '{
    bug_id: "BUG-001",
    finding: "BroadcastStatistics method defined at server/pkg/amsclient/client.go:483 has zero production callers.",
    production_callers: $caller_count,
    definition_references_in_client_go: $defn_count,
    test_references: $test_count,
    impact: "/{app}/rest/v2/broadcasts/{id}/broadcast-statistics endpoint is never polled by Pulse. Viewer counts come from inline broadcast fields (hlsViewerCount, webRTCViewerCount, etc.) via the REAL poll path, not from this endpoint.",
    see_also: ["capture.sh compare_viewer_count()", "normalize.go:NormalizeBroadcast()"]
  }' > "${EVIDENCE_DIR}/bug-001-broadcaststatistics.json" 2>/dev/null || true

printf 'caller_count=%s  defn_count=%s  test_count=%s\n' \
  "${_CALLER_COUNT}" "${_DEFN_COUNT}" "${_TEST_COUNT}" \
  >> "${EVIDENCE_DIR}/timeline.txt"

# ── 6. Assertions ──────────────────────────────────────────────────────────────
# BUG-001: No production callers should exist outside the definition file.
# If _CALLER_COUNT > 0, a caller was added — update this test.
assert_eq "${_CALLER_COUNT}" "0" \
  "${SCENARIO} BUG-001: BroadcastStatistics production caller count outside client.go = 0 (dead code confirmed)" || true

# The definition must be GONE from client.go (deleted S26/D-088, BUG-001 FIXED).
assert_eq "${_DEFN_COUNT}" "0" \
  "${SCENARIO} BroadcastStatistics definition references in client.go = 0 (deleted S26/D-088)" || true

# The test was deleted with the method — no test references may remain.
assert_eq "${_TEST_COUNT}" "0" \
  "${SCENARIO} BroadcastStatistics test references = 0 (deleted with the method, S26/D-088)" || true

# ── Verdict ────────────────────────────────────────────────────────────────────
log "Writing verdict"
scenario_verdict
exit $?
