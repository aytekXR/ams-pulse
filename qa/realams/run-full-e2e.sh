#!/usr/bin/env bash
# qa/realams/run-full-e2e.sh — one-command full Pulse<->AMS validation.
#
# Runs the phased plan from docs/testing/full-e2e-validation-run.md:
#   preflight -> console auth -> Go unit(+70.2% floor) -> Go integration ->
#   web unit -> sdk -> qa units -> CI e2e -> playwright -> real-AMS validate-all ->
#   new TC-*-1x pack -> budgets -> aggregate REPORT.md.
#
# Exit 0 = all phases PASS/SKIP; exit 1 = any phase FAIL.
# A phase SKIP (rc 77) is recorded, never fatal. No secrets: everything comes
# from harness/env.sh. Run ON THE VPS (needs live PULSE_URL/AMS_URL + docker).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# env.sh exports PULSE_URL, PULSE_HEALTH_URL, AMS_URL, PULSE_TOKEN, EVIDENCE_ROOT, REPO_ROOT.
# shellcheck source=harness/env.sh
source "${SCRIPT_DIR}/harness/env.sh"

RUN_ID="FULL-$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="${EVIDENCE_ROOT}/${RUN_ID}"
mkdir -p "${RUN_DIR}"
declare -A RESULT

phase() { # phase <name> <cmd...>
  local name="$1"; shift
  echo "── PHASE ${name} ──" | tee -a "${RUN_DIR}/run.log"
  ( "$@" ) >"${RUN_DIR}/phase-${name}.log" 2>&1
  local rc=$?
  case "${rc}" in
    0)  RESULT[${name}]=PASS ;;
    77) RESULT[${name}]=SKIP ;;
    *)  RESULT[${name}]=FAIL ;;
  esac
  echo "PHASE ${name} → ${RESULT[${name}]} (rc=${rc})" | tee -a "${RUN_DIR}/run.log"
}

go_docker() { # go_docker "<sh -c command>"
  docker run --rm \
    -v "${REPO_ROOT}":/repo \
    -v pulse-gocache:/go/pkg/mod \
    -v pulse-gocache-build:/root/.cache/go-build \
    -v /tmp/clickhouse:/tmp/clickhouse \
    -w /repo/server -e GOFLAGS=-buildvcs=false -e CGO_ENABLED=0 \
    golang:1.25 sh -c "$1"
}
export -f go_docker

# ── 00 preflight ──────────────────────────────────────────────────────────────
phase 00-preflight bash -c '
  for t in docker jq curl ffmpeg python3; do
    command -v "$t" >/dev/null || { echo "missing tool: $t"; exit 1; }
  done
  curl -sf --max-time 10 "'"${PULSE_HEALTH_URL:-${PULSE_URL}/healthz}"'" >/dev/null \
    || { echo "pulse health check failed"; exit 1; }
  c=$(curl -s --max-time 10 -o /dev/null -w "%{http_code}" "'"${AMS_URL}"'/rest/v2/version")
  case "$c" in 200|401|403) : ;; *) echo "ams unreachable (http=$c)"; exit 1 ;; esac
  df -h "'"${EVIDENCE_ROOT}"'" | tail -1'

# ── 01 console auth (ONE attempt — never loops) ───────────────────────────────
phase 01-auth make -C "${SCRIPT_DIR}" auth

# ── 10 Go unit + race + coverage floor 70.2% ──────────────────────────────────
phase 10-go-unit go_docker '
  go test ./... -race -timeout 900s -coverprofile=/tmp/c.out -covermode=atomic \
  && go tool cover -func=/tmp/c.out | awk "/^total:/ { print; if (\$3+0 < 70.2) { exit 1 } }"'

# ── 11 Go integration (needs /tmp/clickhouse v26.6.1) ─────────────────────────
phase 11-go-integration bash -c '
  [ -x /tmp/clickhouse ] || { echo "no /tmp/clickhouse — see docs/testing/e2e-ams-test-design.md §7.4"; exit 77; }
  go_docker "go test -tags integration ./... -timeout 900s"'

# ── 12 web unit + coverage gate ───────────────────────────────────────────────
phase 12-web-unit bash -c 'cd "'"${REPO_ROOT}"'/web" && npm test'

# ── 13 sdk + 15 KB size gate ──────────────────────────────────────────────────
phase 13-sdk bash -c 'cd "'"${REPO_ROOT}"'/sdk/beacon-js" && npm test && npm run size'

# ── 14 qa module units (mock-ams + licensegen) ────────────────────────────────
phase 14-qa-units go_docker '
  cd /repo/qa/mock-ams && go test -race -count=1 -timeout 300s ./... \
  && cd /repo/qa/licensegen && go test -race -count=1 -timeout 300s ./...'

# ── 20 CI e2e stack (the 13 named assertions) ─────────────────────────────────
phase 20-ci-e2e bash -c '
  command -v gh >/dev/null || { echo "gh CLI absent — run e2e.yml from the GitHub UI"; exit 77; }
  gh workflow run e2e.yml || exit 1
  sleep 25
  RID=$(gh run list --workflow=e2e.yml -L1 --json databaseId -q ".[0].databaseId")
  gh run watch "$RID" --exit-status'

# ── 30 playwright route-mocked ────────────────────────────────────────────────
phase 30-playwright bash -c 'cd "'"${REPO_ROOT}"'/web" && npx playwright test'

# ── 40 real-AMS existing scenarios (validate-all) ─────────────────────────────
phase 40-realams-existing make -C "${SCRIPT_DIR}" validate-all

# ── 41 new TC-*-1x pack ───────────────────────────────────────────────────────
phase 41-realams-new bash -c '
  shopt -s nullglob
  scripts=("'"${SCRIPT_DIR}"'"/scenarios/TC-*-1[0-9]-*.sh)
  [ "${#scripts[@]}" -gt 0 ] || { echo "no TC-*-1x scenarios installed"; exit 77; }
  fails=0
  for s in "${scripts[@]}"; do
    echo "── ${s}"
    bash "$s"; rc=$?
    if [ "$rc" -ne 0 ] && [ "$rc" -ne 77 ]; then fails=$((fails+1)); fi
  done
  [ "$fails" -eq 0 ] || exit 1'

# ── 50 budgets ────────────────────────────────────────────────────────────────
phase 50-budgets bash "${REPO_ROOT}/qa/budgets/run-budget-tests.sh"

# ── aggregate ─────────────────────────────────────────────────────────────────
{
  echo "# Pulse Full E2E — ${RUN_ID}"
  echo
  echo "Prod baseline: v0.4.0-98-g641b4e2 · AMS: 3.0.3 EE (source-verified at ams-v3.0.3)"
  echo
  echo "| Phase | Result |"
  echo "|---|---|"
  for k in $(printf '%s\n' "${!RESULT[@]}" | sort); do
    echo "| ${k} | ${RESULT[${k}]} |"
  done
  echo
  echo "## Scenario verdicts (this run)"
  find "${EVIDENCE_ROOT}" -newer "${RUN_DIR}/run.log" -name verdict.txt 2>/dev/null | sort | while read -r v; do
    echo "- \`${v#"${EVIDENCE_ROOT}"/}\` → $(head -1 "$v")"
  done
} | tee "${RUN_DIR}/REPORT.md"

for k in "${!RESULT[@]}"; do
  [ "${RESULT[${k}]}" = FAIL ] && exit 1
done
exit 0
