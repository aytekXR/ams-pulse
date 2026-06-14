export const meta = {
  name: 'pulse-wave-2-fixloop',
  description: 'Wave-2 fix-loop: BE-02 fixes D-W2-002 (live billing) properly + live-CH test; QA-01 re-gates',
  phases: [
    { title: 'Fix', detail: 'BE-02: accounting.go billing source/columns + live ClickHouse reconcile test' },
    { title: 'Re-gate', detail: 'QA-01: fix wave-1 gate script + re-verify (live billing reconcile is the real check)' },
  ],
}

const COMMIT = `
## Commit protocol (D-008 — binding)
1. VERIFY FIRST: commit only when acceptance passes. Partial work is reported, not committed.
2. Stage by EXPLICIT path inside your scope only — NEVER 'git add -A', '-u', or '.'. (D-011: a wave-2 agent already broke this by blanket-staging and swallowing another agent's files — do NOT repeat it. List each path explicitly.)
3. Message '<YOUR-AGENT-ID> wave-2-fix: <summary>' with verification evidence (commands + measured numbers) in the body. No push.
4. If .git/index.lock is busy: wait briefly and retry; never delete the lock.
5. Report your commit SHA in your structured output.
`

const AGENT_SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['status', 'committed', 'summary'],
  properties: {
    status: { type: 'string', enum: ['COMPLETE', 'PARTIAL', 'BLOCKED'] },
    reportPath: { type: 'string' },
    acceptance: { type: 'array', items: { type: 'object', additionalProperties: false,
      required: ['criterion', 'verdict'],
      properties: { criterion: { type: 'string' }, verdict: { type: 'string', enum: ['PASS', 'FAIL', 'WAIVED', 'NA'] }, measured: { type: 'string' } } } },
    committed: { type: 'boolean' },
    commitSha: { type: 'string' },
    gaps: { type: 'array', items: { type: 'string' } },
    summary: { type: 'string' },
  },
}

const GATE_SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['verdict', 'reportPath', 'summary'],
  properties: {
    verdict: { type: 'string', enum: ['PASS', 'PASS_WITH_LIMITATIONS', 'FAIL'] },
    reportPath: { type: 'string' },
    criteria: { type: 'array', items: { type: 'object', additionalProperties: false,
      required: ['name', 'verdict'],
      properties: { name: { type: 'string' }, verdict: { type: 'string', enum: ['PASS', 'FAIL', 'WAIVED'] }, measured: { type: 'string' }, budget: { type: 'string' } } } },
    defects: { type: 'array', items: { type: 'object', additionalProperties: false,
      required: ['id', 'owner', 'severity', 'repro'],
      properties: { id: { type: 'string' }, owner: { type: 'string' }, severity: { type: 'string', enum: ['critical', 'major', 'minor'] }, repro: { type: 'string' }, criterionViolated: { type: 'string' } } } },
    waivers: { type: 'array', items: { type: 'string' } },
    committed: { type: 'boolean' },
    summary: { type: 'string' },
  },
}

// ---------- Phase 1: Fix (BE-02) ----------

phase('Fix')
log('Wave-2 fix-loop: BE-02 fixes D-W2-002 (live billing) properly + live-CH test; then QA-01 re-gates.')

const fix = await agent(
  `You are BE-02 (backend product-plane) in the Pulse MVP build. ORCH-00 dispatched you to FIX the one wave-3 blocker from the wave-2 gate, defect D-W2-002. Read first: CLAUDE.md, your charter agents/definitions/BE-02-backend-productplane.md, the gate report qa/wave-2/gate-report.md (defect D-W2-002 section), your WO-204 report agents/handoffs/wave-2/WO-204-report.md, docs/ARCHITECTURE.md §3-4, and the actual schemas contracts/db/clickhouse/0001_init.sql (viewer_sessions, rollup_audience_1d, rollup_usage_1d, the materialized views) — do NOT trust the comments in accounting.go, read the DDL.

## The defect (confirmed by ORCH-00 against the DDL)
server/internal/reports/accounting.go queries column names that do NOT exist in ClickHouse, so GET /api/v1/reports/usage returns 500 and 'pulse diag --reconcile' fails with CH error 47 on a LIVE stack. The in-memory unit test passes (drift 0%) only because it bypasses ClickHouse entirely (a.conn == nil) — that blind spot is what let this ship.

Two layers, BOTH required:
1. **Wrong columns** (minimum): accounting.go uses 'bucket_ts' (actual: 'bucket'), 'watch_s_state' (actual aggregate: 'watch_time_s'), 'peak_viewers_state' (actual aggregate: 'peak_concurrency'). Lines ~148, 169-170, 299, 301, plus the misleading comment block ~159-164.
2. **Wrong table** (the correct fix per WO-204): ComputeUsage/Reconcile source from 'rollup_audience_1d', but that table's peak_concurrency is maxState(toUInt32(1)) = always 1, and egress is a hardcoded 1000-kbps model. WO-204 specified 'rollup_usage_1d' — the SummingMergeTree F6 billing table with real plain columns viewer_minutes (Float64), peak_concurrency (UInt32), egress_bytes (UInt64), recording_bytes (UInt64), and a tenant dimension. Source billing usage from rollup_usage_1d (plain SummingMergeTree columns — NO sumMerge/maxMerge needed; use sum()/max()/groupArray as appropriate), with the documented raw viewer_sessions fallback for partial days. Verify the mv_usage_1d materialized view actually populates rollup_usage_1d from your seeded sessions; if the MV is wrong, fix the query side to reconcile against raw truth within ±1% (do NOT edit contracts/ — if the DDL/MV itself is broken, STOP and report it as a change request for ORCH-00).

## Acceptance (run them; report measured numbers)
- 'CGO_ENABLED=0 go build ./... && go vet ./... && go test ./...' green.
- NEW ClickHouse integration test (build tag 'integration', like the existing CH tests): spin up /tmp/clickhouse, run migrations, seed a KNOWN-TRUTH month of viewer_sessions (fixed counts/bitrates/durations, ≥2 tenants by stream_pattern), then (a) call ComputeUsage and assert viewer-minutes/peak/egress match truth, (b) call Reconcile and assert DriftPct ≤ 1.0 against raw, (c) assert tenant attribution is correct. This test MUST exercise the real a.conn path (NOT the nil-conn shortcut). It is the regression guard for this whole defect class.
- Live manual proof for your report: start /tmp/clickhouse + 'pulse serve', migrate, seed, then 'curl GET /api/v1/reports/usage' returns 200 with sane figures, and 'pulse diag --reconcile' prints drift ≤ 1%. Paste the outputs.

## Hard rules
- CGO_ENABLED=0 always. Contracts are FROZEN (D-004): do NOT edit anything under contracts/ — if the DDL/MV is genuinely wrong, report a CR, don't patch the contract. Metrics live in ClickHouse, config in meta — never cross them. Stay inside BE-02 scope (server/internal/{reports,query,api,...} + declared cmd/ edits).
- Environment: /tmp/clickhouse present (v26.6.1). macOS arm64, no Docker.

## Scope discipline
This is a focused fix. Do NOT also implement the tenant CRUD endpoints, kafka-lag-in-healthz, or other wave-2 gaps — ORCH-00 scheduled those for the validation sweep (D-010). Touch only what D-W2-002 requires (accounting.go, its test, and any query/statement code that shares the broken column names — grep for watch_s_state/peak_viewers_state/bucket_ts across internal/reports and internal/query and fix every hit).
${COMMIT}
## Output
Update your report at agents/handoffs/wave-2/WO-204-report.md with a "D-W2-002 fix" section (root cause, files changed, the live curl + reconcile outputs, the new integration test name). Then return ONLY the StructuredOutput object.`,
  { label: 'BE-02 fix: D-W2-002 live billing', phase: 'Fix', schema: AGENT_SCHEMA })

log(`BE-02 fix: status=${fix ? fix.status : 'null'}, committed=${fix ? fix.committed : '?'}, sha=${fix ? (fix.commitSha || '') : ''}`)

// ---------- Phase 2: Re-gate (QA-01) ----------

phase('Re-gate')
const regate = await agent(
  `You are QA-01 (QA & verification) in the Pulse MVP build. ORCH-00 dispatched you to RE-GATE wave 2 after BE-02's fix of the major billing defect D-W2-002. VERIFY, NEVER FIX product code (you may fix your OWN qa/ gate scripts — they are your scope).

Read first: CLAUDE.md, your charter agents/definitions/QA-01-qa.md, the prior gate report qa/wave-2/gate-report.md, and BE-02's updated agents/handoffs/wave-2/WO-204-report.md (its D-W2-002 fix section).

## Tasks
1. **Fix your own wave-1 gate script** (defects D-W2-001 / D-W2-003, QA-01 scope): qa/wave-1/run-gate.sh POST /api/v1/alerts/rules is missing the now-required 'name' field (~line 380) → add '"name":"gate-test-rule"' to the JSON body so the script exits 0 again. This is a test-code fix, allowed.
2. **Re-verify the closed blocker D-W2-002 against the LIVE stack** (this is the real check the in-memory test missed): start /tmp/clickhouse + pulse, migrate, seed known truth, then assert GET /api/v1/reports/usage returns 200 with figures within ±1% of computed truth, and 'pulse diag --reconcile' prints drift ≤ 1% (exit 0). Run BE-02's new integration test ('CGO_ENABLED=0 go test -tags integration ./internal/reports/...'). If billing live still fails, that is a FAIL — do not waive it.
3. **Regression sweep:** re-run the full mechanical gate — qa/wave-1/run-gate.sh (now fixed, must exit 0), qa/budgets/run-budget-tests.sh (8/8), qa/wave-2/run-gate.sh, and full build/lint/test across server, web, sdk. Confirm nothing regressed from the fix.

## Acceptance
Append a "## Re-gate (D-W2-002 fix)" section to qa/wave-2/gate-report.md with PASS/FAIL per re-checked criterion + measured numbers + repro commands, and an updated overall verdict. Waivers stay limited to D-002 + D-007.5. Scripts must exit nonzero on failure.

Environment: /tmp/clickhouse present (v26.6.1); CGO_ENABLED=0; macOS arm64; no Docker.
${COMMIT}
Return ONLY the StructuredOutput object (verdict PASS|PASS_WITH_LIMITATIONS|FAIL, per-criterion verdicts, remaining defects, waivers, committed, summary). The updated gate report IS your completion report.`,
  { label: 'QA-01 re-gate: wave-2 fix', phase: 'Re-gate', schema: GATE_SCHEMA })

log(`QA-01 re-gate verdict: ${regate ? regate.verdict : 'null'} — ${regate ? (regate.defects || []).length : '?'} remaining defect(s).`)

return {
  fix: fix ? { status: fix.status, committed: fix.committed, sha: fix.commitSha || '', acceptance: fix.acceptance || [], summary: fix.summary } : null,
  regate: regate ? { verdict: regate.verdict, defects: regate.defects || [], waivers: regate.waivers || [], report: regate.reportPath, summary: regate.summary } : null,
}
