#!/usr/bin/env bash
# qa/realams/harness/assert.sh
#
# Assertion helpers for scenario scripts.
# SOURCE this file after env.sh.
#
# EVIDENCE_DIR must be set by the scenario script BEFORE any assert is called.
# CHECKS file is ${EVIDENCE_DIR}/checks.txt — accumulates PASS/FAIL lines.
#
# Each assert function:
#   - Appends a result line to CHECKS
#   - Returns 0 on pass, 1 on fail
#   - Does NOT call exit (callers aggregate failures)
#
# Call scenario_verdict at the end of a scenario to:
#   - Write ${EVIDENCE_DIR}/verdict.txt
#   - Return 0 (all PASS) or 1 (any FAIL)
#
set -euo pipefail

# ── Internal: record a check result ──────────────────────────────────────────
_record_check() {
  local status="$1"  # PASS or FAIL
  local label="$2"
  local detail="$3"
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  local checks_file="${EVIDENCE_DIR}/checks.txt"
  printf '[%s] %s  %s  %s\n' "$ts" "$status" "$label" "$detail" >> "$checks_file"
  if [ "$status" = "FAIL" ]; then
    printf '[assert] FAIL: %s  (%s)\n' "$label" "$detail" >&2
    return 1
  else
    printf '[assert] PASS: %s\n' "$label" >&2
    return 0
  fi
}

# ── assert_eq A B LABEL ────────────────────────────────────────────────────────
# Passes when A == B (string comparison).
assert_eq() {
  local a="$1" b="$2" label="$3"
  if [ "$a" = "$b" ]; then
    _record_check PASS "$label" "eq: \"${a}\""
  else
    _record_check FAIL "$label" "expected=\"${b}\" got=\"${a}\""
  fi
}

# ── assert_approx A B TOL_PCT LABEL ───────────────────────────────────────────
# Passes when |A - B| / |B| * 100 <= TOL_PCT.
# A and B must be numeric (integer or float). Uses awk for arithmetic.
assert_approx() {
  local a="$1" b="$2" tol="$3" label="$4"
  local result
  result="$(awk -v a="$a" -v b="$b" -v tol="$tol" 'BEGIN {
    if (b == 0) {
      # avoid div-by-zero: pass only if a == 0 as well
      print (a == 0) ? "PASS" : "FAIL"
    } else {
      diff = (a - b < 0) ? (b - a) : (a - b)
      pct = diff / (b < 0 ? -b : b) * 100
      print (pct <= tol) ? "PASS" : "FAIL"
    }
  }')"
  if [ "$result" = "PASS" ]; then
    _record_check PASS "$label" "approx: ${a} ≈ ${b} (tol=${tol}%)"
  else
    _record_check FAIL "$label" "approx: got=${a} expected≈${b} tol=${tol}%"
  fi
}

# ── assert_gte A B LABEL ──────────────────────────────────────────────────────
# Passes when A >= B (numeric).
assert_gte() {
  local a="$1" b="$2" label="$3"
  local result
  result="$(awk -v a="$a" -v b="$b" 'BEGIN { print (a >= b) ? "PASS" : "FAIL" }')"
  if [ "$result" = "PASS" ]; then
    _record_check PASS "$label" "gte: ${a} >= ${b}"
  else
    _record_check FAIL "$label" "gte: ${a} < ${b} (required >= ${b})"
  fi
}

# ── assert_lte A B LABEL ──────────────────────────────────────────────────────
# Passes when A <= B (numeric).
assert_lte() {
  local a="$1" b="$2" label="$3"
  local result
  result="$(awk -v a="$a" -v b="$b" 'BEGIN { print (a <= b) ? "PASS" : "FAIL" }')"
  if [ "$result" = "PASS" ]; then
    _record_check PASS "$label" "lte: ${a} <= ${b}"
  else
    _record_check FAIL "$label" "lte: ${a} > ${b} (required <= ${b})"
  fi
}

# ── assert_within A B ABS_DELTA LABEL ────────────────────────────────────────
# Passes when |A - B| <= ABS_DELTA (numeric absolute tolerance).
assert_within() {
  local a="$1" b="$2" delta="$3" label="$4"
  local result
  result="$(awk -v a="$a" -v b="$b" -v d="$delta" 'BEGIN {
    diff = (a - b < 0) ? (b - a) : (a - b)
    print (diff <= d) ? "PASS" : "FAIL"
  }')"
  if [ "$result" = "PASS" ]; then
    _record_check PASS "$label" "within: |${a} - ${b}| <= ${delta}"
  else
    _record_check FAIL "$label" "within: |${a} - ${b}| > ${delta}"
  fi
}

# ── scenario_verdict ──────────────────────────────────────────────────────────
# Reads CHECKS file, computes overall PASS/FAIL, writes verdict.txt.
# Returns 0 if all checks passed, 1 if any failed.
#
# Call once at the end of each scenario script.
scenario_verdict() {
  local checks_file="${EVIDENCE_DIR}/checks.txt"
  local verdict_file="${EVIDENCE_DIR}/verdict.txt"

  if [ ! -f "$checks_file" ]; then
    {
      echo "FAIL"
      echo "No checks recorded — scenario may have crashed before any assert."
    } > "$verdict_file"
    echo "[verdict] FAIL — no checks recorded" >&2
    return 1
  fi

  local total fail_count
  total="$(wc -l < "$checks_file" | tr -d ' ')"
  fail_count="$(grep -c '^\[.*\] FAIL ' "$checks_file" || true)"
  local pass_count=$(( total - fail_count ))

  {
    if [ "$fail_count" -eq 0 ]; then
      echo "PASS"
    else
      echo "FAIL"
    fi
    echo "total=${total}  pass=${pass_count}  fail=${fail_count}"
    echo ""
    if [ "$fail_count" -gt 0 ]; then
      echo "--- Failed checks ---"
      grep '^\[.*\] FAIL ' "$checks_file" || true
    fi
  } > "$verdict_file"

  if [ "$fail_count" -eq 0 ]; then
    echo "[verdict] PASS  (${pass_count}/${total} checks passed)" >&2
    return 0
  else
    echo "[verdict] FAIL  (${fail_count}/${total} checks failed)" >&2
    return 1
  fi
}
