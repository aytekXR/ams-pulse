export const meta = {
  name: 'pulse-wave-3-mvp',
  description: 'Pulse Wave 3-MVP: F9 anomaly detection + F10 synthetic probes — implement, gate, document',
  phases: [
    { title: 'Implement', detail: 'parallel: [BE-01(301) -> BE-02(302)] | FE-01(303)' },
    { title: 'Gate', detail: 'QA-01(304): probe round-trip + anomaly false-alarm gate' },
    { title: 'Docs', detail: 'DOC-01(305): only on PASS / PASS_WITH_LIMITATIONS' },
  ],
}

const HARD = `
## Hard rules (CLAUDE.md + docs/ARCHITECTURE.md sec.3-4 — non-negotiable)
- Contracts are FROZEN (D-004). Do NOT edit anything under contracts/. The F9/F10 contracts already exist (/anomalies, /probes CRUD, /probes/{id}/results; AnomalyFlag, Probe, ProbeWrite, ProbeResult; meta anomaly_baselines + probes; CH probe_results). If you genuinely need a change, STOP that item and file a changeRequest in your output — never work around the freeze.
- Single-writer scope map (agents/manifest.yaml): edit ONLY files in your charter scope (+ the declared cmd/ edits your WO permits). Never touch another agent's scope.
- AMS wire formats ONLY in server/pkg/amsclient + server/internal/collector. Metrics live in ClickHouse, config in the meta store — never cross them. probe_results is ClickHouse (time-series); probes config + anomaly_baselines are meta. The web UI consumes ONLY the public API via generated types.
- Numeric/quality targets are TEST TARGETS — measure and report: F9 default sensitivity <1 false alarm/node-week; F10 probe results visible alongside organic QoE with CLEAR synthetic labeling. CGO_ENABLED=0 always; React 19 + RR7 + Vite 6 + TS strict; recharts; no external fonts/CDNs.
`

const ENV = `
## Environment (binding)
- Working dir: /Users/ae/repo/ant-marketplace, branch main. Work directly on main; no branches.
- macOS arm64; Go 1.26.4 (CGO_ENABLED=0), Node v26, npm 11.12.1. NO Docker (D-002).
- ClickHouse single binary at /tmp/clickhouse (v26.6.1) for integration tests (build tag 'integration'). web/pulse_secret.key is a gitignored dev key — never commit it.
`

const COMMIT = `
## Commit protocol (D-008 — binding; you commit your own work)
1. VERIFY FIRST: run your WO acceptance criteria; commit ONLY when they pass. Partial/blocked work is REPORTED, not committed.
2. Stage by EXPLICIT path inside your scope only — NEVER 'git add -A', '-u', or '.'. (D-011: a wave-2 agent blanket-staged and swallowed another agent's files — do NOT repeat it. List each path.) Include the declared cmd/ edits your WO permits.
3. Message '<YOUR-AGENT-ID> <WO-ID>: <summary>' with verification evidence (commands + measured numbers) in the body. No push.
4. If .git/index.lock is busy: wait briefly and retry (bounded). NEVER delete the lock.
5. Report your commit SHA in your structured output.
`

const OUTPUT = `
## Output
Write your completion report to the path your WO names (agents/handoffs/wave-3/<WO>-report.md) with the sections it requests (downstream interface signatures, sensitivity math, measured numbers, deps, cmd edits, gaps/CRs). Then return ONLY the forced StructuredOutput object.
`

const AGENT_SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['status', 'reportPath', 'committed', 'summary'],
  properties: {
    status: { type: 'string', enum: ['COMPLETE', 'PARTIAL', 'BLOCKED'] },
    reportPath: { type: 'string' },
    acceptance: { type: 'array', items: { type: 'object', additionalProperties: false,
      required: ['criterion', 'verdict'],
      properties: { criterion: { type: 'string' }, verdict: { type: 'string', enum: ['PASS', 'FAIL', 'WAIVED', 'NA'] }, measured: { type: 'string' } } } },
    gaps: { type: 'array', items: { type: 'string' } },
    cmdEditsDeclared: { type: 'array', items: { type: 'string' } },
    changeRequests: { type: 'array', items: { type: 'string' } },
    committed: { type: 'boolean' },
    commitSha: { type: 'string' },
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

function mk(id, role, charter, wo, prereqs, extra) {
  return `You are ${id}, the ${role} agent in the Pulse MVP build, Wave 3-MVP (F9 anomaly detection + F10 synthetic probes, minimal-but-working per decision D-001). ORCH-00 dispatched you via the Workflow orchestrator. You are autonomous: do the whole job, then commit and report.

READ FIRST, in this exact order, before writing code:
1. CLAUDE.md.
2. Your charter: agents/definitions/${charter}
3. Your work order: agents/handoffs/wave-3/${wo} — THIS IS YOUR TASK. Execute ALL of it.
4. Prereq reading: ${prereqs}
5. docs/ARCHITECTURE.md sec.3 (rules) and sec.4 (budgets); agents/handoffs/decisions.md D-001 + D-012 (wave-3 scope/structure).

Then implement the work order completely and run EVERY acceptance criterion it lists, reporting MEASURED numbers.
${extra ? '\n' + extra + '\n' : ''}${HARD}${ENV}${COMMIT}${OUTPUT}`
}

// ---------- Phase 1: Implement ----------

phase('Implement')
log('Wave 3-MVP dispatch: [BE-01(301) -> BE-02(302)] | FE-01(303), then QA-01 gate(304), then DOC-01(305) on pass.')

const lanes = await parallel([
  // Lane A — backend chain, sequential (shared go.mod/cmd): BE-01 -> BE-02
  async () => {
    const be301 = await agent(
      mk('BE-01', 'backend data-plane', 'BE-01-backend-dataplane.md', 'WO-301.md',
        'wave-2 agents/handoffs/wave-2/WO-202-report.md (your EventSink/store patterns) and the INT-01 design notes atop contracts/db/clickhouse/0001_init.sql (probe_results placement)',
        'You are the FIRST link. BE-02 runs after you and depends on the domain.ProbeConfigSource interface + ProbeResult struct you define — put their EXACT Go signatures in WO-301-report.md. The HLS probe path MUST actually work (manifest + first segment, real TTFB/bitrate/success); webrtc/rtmp/dash may be minimal-but-honest (no faked success) — document coverage.'),
      { label: 'BE-01:WO-301 probe runner + CH store', phase: 'Implement', schema: AGENT_SCHEMA })

    const be302 = await agent(
      mk('BE-02', 'backend product-plane', 'BE-02-backend-productplane.md', 'WO-302.md',
        'agents/handoffs/wave-3/WO-301-report.md FIRST (BE-01 just finished — the ProbeConfigSource/ProbeResult signatures + probe_results reader you build on), then your wave-2 reports for the alert-evaluator + tier-gating patterns you reuse',
        'Two features: F9 anomaly detection (rolling baselines in anomaly_baselines meta + flags computed on read; CALIBRATE default sensitivity for the PRD target <1 false alarm/node-week — document the math) and the F10 product surface (probe CRUD + GET /probes/{id}/results + your meta-backed ProbeConfigSource impl + serve wiring of BE-01 runner). You share cmd/pulse with BE-01 sequentially — declare every cmd edit (D-005). Verify go build/vet/test ALL pass before committing.'),
      { label: 'BE-02:WO-302 anomaly + probe API', phase: 'Implement', schema: AGENT_SCHEMA })

    return { be301, be302 }
  },

  // Lane B — Frontend (disjoint tree: web/)
  () => agent(
    mk('FE-01', 'frontend', 'FE-01-frontend.md', 'WO-303.md',
      'your wave-2 agents/handoffs/wave-2/WO-205-report.md, and the BE reports WO-301-report.md / WO-302-report.md for API shapes (read them if present; otherwise build to the frozen OpenAPI types and note any mismatch as a gap)',
      'Regenerate API types FIRST (npm run generate:api), generated types ONLY (git-grep proof). The PRD F10 acceptance is the labeling: probe (synthetic) results must be CLEARLY distinguished from organic beacon QoE — a labeled section/badge, never silently mixed into organic charts. Both surfaces are tier-gate-aware (anomalies Enterprise, probes Pro+) with upsell, never a broken page. Include screenshots against seeded fixtures.'),
    { label: 'FE-01:WO-303 anomalies + probes UI', phase: 'Implement', schema: AGENT_SCHEMA }),
])

const [chain, fe] = lanes
const implReports = [chain && chain.be301, chain && chain.be302, fe].filter(Boolean)
log(`Implement done: ${implReports.length}/3 agents. Statuses: ` + implReports.map(r => (r && r.status) || 'null').join(', '))

// ---------- Phase 2: Gate (barrier — QA verifies the integrated tree after all commits) ----------

phase('Gate')
const gate = await agent(
  `You are QA-01, the QA & verification agent in the Pulse MVP build, Wave 3-MVP. ORCH-00 dispatched you AFTER the implementation agents committed. Independently verify F9 (anomaly detection) + F10 (synthetic probes) END-TO-END on the local stack. VERIFY, NEVER FIX product code (you may write/extend your own qa/ test content).

READ FIRST, in order:
1. CLAUDE.md, your charter agents/definitions/QA-01-qa.md.
2. Your work order: agents/handoffs/wave-3/WO-304.md — execute ALL of it.
3. The implementation reports agents/handoffs/wave-3/WO-301-report.md, WO-302-report.md, WO-303-report.md (interface shapes, sensitivity math, dev license keys, tier matrix).
4. decisions.md D-001 + D-012; docs/ARCHITECTURE.md sec.4.

Gate: (1) F10 probe round-trip — create a probe via POST /probes pointing at a local/httptest HLS origin, let BE-01's runner execute, assert a probe_results row lands in ClickHouse (/tmp/clickhouse) and is returned by GET /probes/{id}/results with measured TTFB/bitrate/success; degrade the origin → failure result with correct error_code; confirm synthetic labeling. (2) F9 anomaly — drive a steady synthetic metric stream through the baseline updater at default sensitivity for a simulated node-week, assert the flag rate maps to <1 false alarm/node-week; inject a genuine sustained deviation, assert it IS flagged. (3) Tier gates (anomalies Enterprise, probes Pro+). (4) Regression: wave-1 + wave-2 gates still green (qa/wave-1/run-gate.sh, qa/wave-2/run-gate.sh, qa/budgets/run-budget-tests.sh), full build/lint/test across server/web/sdk, SDK size gate. (5) kin-openapi conformance on the new endpoints.

WAIVERS limited to the D-002 (no Docker) and D-007.5 (no Kafka broker) classes — you may NOT waive anything else; only ORCH-00 waives. A failed probe round-trip or an anomaly false-alarm rate over budget is a FAIL/defect, not a waiver.

Environment: /tmp/clickhouse (v26.6.1); CGO_ENABLED=0; macOS arm64; no Docker. ${COMMIT}
Write the gate report to qa/wave-3/gate-report.md (PASS/FAIL per criterion with measured numbers, repro commands, defect list with owner/severity/repro). Scripts exit nonzero on failure. The gate report IS your completion report. Return ONLY the forced StructuredOutput object.`,
  { label: 'QA-01:WO-304 wave-3 gate', phase: 'Gate', schema: GATE_SCHEMA })

log(`QA-01 gate verdict: ${gate ? gate.verdict : 'null'} — ${gate ? (gate.defects || []).length : '?'} defect(s).`)

// ---------- Phase 3: Docs (only on PASS / PASS_WITH_LIMITATIONS) ----------

phase('Docs')
let doc = null
if (gate && (gate.verdict === 'PASS' || gate.verdict === 'PASS_WITH_LIMITATIONS')) {
  doc = await agent(
    mk('DOC-01', 'documentation', 'DOC-01-docs.md', 'WO-305.md',
      'the implementation reports WO-301..303-report.md and the QA gate report qa/wave-3/gate-report.md',
      'Every documented command/config MUST run against the wave-3 build — test them. Flip F9/F10 to shipped (MVP) only for what QA verified; label the minimal-but-working scope (D-001) and Phase-3 deltas honestly.'),
    { label: 'DOC-01:WO-305 wave-3 docs', phase: 'Docs', schema: AGENT_SCHEMA })
} else {
  log('Gate did not pass — skipping DOC-01. ORCH-00 will run a fix-loop.')
}

return {
  implementation: implReports.map(r => ({ status: r.status, report: r.reportPath, committed: r.committed, sha: r.commitSha || '', crs: r.changeRequests || [], gaps: r.gaps || [], summary: r.summary })),
  gate: gate ? { verdict: gate.verdict, defects: gate.defects || [], waivers: gate.waivers || [], report: gate.reportPath, summary: gate.summary } : null,
  docs: doc ? { status: doc.status, report: doc.reportPath, committed: doc.committed, summary: doc.summary } : 'skipped (gate not passed)',
}
