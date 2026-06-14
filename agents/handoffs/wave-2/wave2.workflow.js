export const meta = {
  name: 'pulse-wave-2',
  description: 'Pulse MVP Wave 2: beacons/QoE/reports/fleet/API/Helm — implement, gate, document',
  phases: [
    { title: 'Implement', detail: 'parallel: [BE-01(202)->BE-02(203)->BE-02(204)] | SDK-01(201) | FE-01(205) | INFRA-01(206)' },
    { title: 'Gate', detail: 'QA-01(207): beacon round-trip + billing reconcile +/-1% + budgets' },
    { title: 'Docs', detail: 'DOC-01(208): only on PASS / PASS_WITH_LIMITATIONS' },
  ],
}

// ---------- shared rule blocks (binding, embedded in every agent prompt) ----------

const HARD = `
## Hard rules (CLAUDE.md + docs/ARCHITECTURE.md sec.3-4 — non-negotiable)
- Contracts are FROZEN (D-004). Do NOT edit anything under contracts/. If you genuinely need a contract change, STOP that work item and surface it as a changeRequest in your structured output for ORCH-00 to rule on — never work around the freeze silently or invent a shape.
- Single-writer scope map (agents/manifest.yaml): edit ONLY files inside your charter scope (plus the declared cmd/ edits your work order explicitly permits). Never touch another agent's scope.
- AMS wire formats live ONLY in server/pkg/amsclient + server/internal/collector. Metrics live in ClickHouse, config in the meta store — never cross them. The web UI consumes ONLY the public API via generated types (no hand-rolled API shapes).
- Beacon ingest is hostile-input territory: token auth (constant-time compare, hashed at rest), per-token rate limits, body size caps, strict schema validation, never echo tokens.
- Numeric budgets are TEST TARGETS — measure and report actual numbers (never estimate): stream <=10s, viewer counts +/-2%, dashboard <2s @500, 13-month query <3s, SDK <15KB gzip, ingest degradation visible <=15s, alert <30s, statements <60s & +/-1%, node discovery <=2min.
- CGO_ENABLED=0 ALWAYS; single binary pulse serve|migrate|diag; React 19 + RR7 + Vite 6 + TS strict; recharts; no external fonts/CDNs.
`

const ENV = `
## Environment (binding)
- Working dir: /Users/ae/repo/ant-marketplace, branch main. Work directly on main; do NOT create branches.
- macOS arm64; Go 1.26.4 (CGO_ENABLED=0), Node v26, npm 11.12.1. NO Docker (D-002) — author+lint infra, do not run a daemon.
- ClickHouse single binary present at /tmp/clickhouse (v26.6.1). Use 'clickhouse server' / 'clickhouse local' as your tests need; never assume an always-on daemon.
- web/pulse_secret.key is a gitignored local dev key — NEVER commit it. ClickHouse artifact/binary dirs are gitignored too.
`

const COMMIT = `
## Commit protocol (D-008 — binding; you commit your own work)
1. VERIFY FIRST: run your work order's acceptance criteria. Commit ONLY when they pass. Partial/blocked work is REPORTED in your output, NOT committed.
2. Stage by EXPLICIT path inside your scope only — NEVER 'git add -A', '-u', or '.' (parallel agents share this one working tree; blanket staging swallows their in-flight files). Include the declared cmd/ edits your WO permits.
3. Commit message: '<YOUR-AGENT-ID> <WO-ID>: <summary>' with verification evidence (commands run, key measured numbers) in the body. Do NOT push.
4. If .git/index.lock is busy: wait briefly and retry (bounded). NEVER delete the lock.
5. Put your resulting commit SHA in your structured output.
`

const OUTPUT = `
## Output
Write your completion report to the report path your work order names (agents/handoffs/wave-2/<WO>-report.md) with the sections it requests (interfaces/signatures for downstream agents, formulas, measured numbers, deps, cmd edits, gaps/CRs). Then return ONLY the forced StructuredOutput object.
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
  return `You are ${id}, the ${role} agent in the Pulse MVP build, Wave 2. ORCH-00 dispatched you via the Workflow orchestrator. You are autonomous: do the whole job, then commit and report.

READ FIRST, in this exact order, before writing any code:
1. CLAUDE.md (repo guidance).
2. Your charter: agents/definitions/${charter}
3. Your work order: agents/handoffs/wave-2/${wo} — THIS IS YOUR TASK. Execute ALL of it; skip nothing.
4. Prereq reading: ${prereqs}
5. docs/ARCHITECTURE.md sec.3 (architecture rules) and sec.4 (numeric budgets).

Then implement the work order completely and run EVERY acceptance criterion it lists, reporting MEASURED numbers.
${extra ? '\n' + extra + '\n' : ''}${HARD}${ENV}${COMMIT}${OUTPUT}`
}

// ---------- Phase 1: Implement (parallel lanes; BE chain is sequential within its lane) ----------

phase('Implement')
log('Wave 2 dispatch: BE chain [202->203->204] | SDK-01(201) | FE-01(205) | INFRA-01(206), then QA-01 gate(207), then DOC-01(208) on pass.')

const lanes = await parallel([
  // Lane A — backend chain, strictly sequential (shared go.mod/cmd): BE-01 -> BE-02 -> BE-02
  async () => {
    const be202 = await agent(
      mk('BE-01', 'backend data-plane', 'BE-01-backend-dataplane.md', 'WO-202.md',
        'your wave-1 report agents/handoffs/wave-1/WO-102-report.md, agents/handoffs/wave-1/fixloop-BE-01-report.md, and the re-gate section of qa/wave-1/gate-report.md',
        'You are the FIRST link in the backend chain. BE-02 runs after you and depends on your new interfaces — your WO-202-report.md MUST list exact Go signatures (interfaces added/changed in internal/domain, the health-score formula, EventSink shape) so BE-02 can build against them. Kafka is verified with an in-process fake only (D-007.5, no broker here). Geo = mmdb reader only, no bundled DB (D-007.4).'),
      { label: 'BE-01:WO-202 dataplane', phase: 'Implement', schema: AGENT_SCHEMA })

    const be203 = await agent(
      mk('BE-02', 'backend product-plane', 'BE-02-backend-productplane.md', 'WO-203.md',
        'agents/handoffs/wave-2/WO-202-report.md FIRST (BE-01 just finished — its new interfaces/signatures), then agents/handoffs/wave-1/WO-103-report.md gaps G1-G8 (several close here)',
        'IMPORTANT additions to your work order: (a) per D-006, ALSO implement the CR-3 server endpoint POST /admin/sources/{sourceId}/test (the AmsSourceStatus contract already exists; wire it through collector/amsclient so the FE onboarding workaround can be removed) — it is NOT in your numbered list, add it. (b) Beacon ingest under server/internal/collector/beacon/ is YOUR scope this wave (D-007.2 exception). (c) You share cmd/pulse with BE-01 sequentially — declare every cmd edit (D-005). Verify go build/vet/test ALL pass before committing.'),
      { label: 'BE-02:WO-203 product-plane I', phase: 'Implement', schema: AGENT_SCHEMA })

    const be204 = await agent(
      mk('BE-02', 'backend product-plane', 'BE-02-backend-productplane.md', 'WO-204.md',
        'agents/handoffs/wave-2/WO-203-report.md FIRST (your own prior order, same package tree) and agents/handoffs/wave-2/WO-202-report.md (BE-01 viewer_sessions/rollup shapes you read from)',
        'This is the second half of BE-02 wave-2 load (split from one order to avoid context exhaustion). Reconciliation must be within +/-1% and statement generation <60s — PROVE both with a seeded synthetic month and report the measured numbers. Wire pulse diag --reconcile (declare the cmd edit).'),
      { label: 'BE-02:WO-204 reports/exports', phase: 'Implement', schema: AGENT_SCHEMA })

    return { be202, be203, be204 }
  },

  // Lane B — SDK (disjoint tree: sdk/)
  () => agent(
    mk('SDK-01', 'beacon SDK', 'SDK-01-beacon-sdk.md', 'WO-201.md',
      'agents/handoffs/wave-1/WO-101-report.md (contract rulings: timestamps, enrichment block)',
      'Hard gate: npm run size MUST stay under 15 KB gzipped — report the measured size. Zero runtime deps, tree-shakeable. The SDK is open-sourced (a launch asset) — code quality matters. Server ingest endpoint is BE-02 (WO-203), not you.'),
    { label: 'SDK-01:WO-201 beacon SDK', phase: 'Implement', schema: AGENT_SCHEMA }),

  // Lane C — Frontend (disjoint tree: web/)
  () => agent(
    mk('FE-01', 'frontend', 'FE-01-frontend.md', 'WO-205.md',
      'your wave-1 reports agents/handoffs/wave-1/WO-104-report.md and fixloop-FE-01-report.md; the feature READMEs web/src/features/*/README.md (acceptance source)',
      'Regenerate API types FIRST (npm run generate:api) and use GENERATED types ONLY — prove it with a git-grep in your report. Each new surface needs loading/error/empty states. Reports view must be tier-gate-aware (Business upsell when entitlement missing, never a broken page). Include screenshots of every surface against seeded fixtures.'),
    { label: 'FE-01:WO-205 frontend', phase: 'Implement', schema: AGENT_SCHEMA }),

  // Lane D — Infra (disjoint tree: deploy/, .github/, Makefile, .gitignore)
  () => agent(
    mk('INFRA-01', 'infrastructure', 'INFRA-01-infra.md', 'WO-206.md',
      'agents/handoffs/wave-1/WO-106-report.md (your wave-1 infra report) and server/cmd/pulse/config.go for the PULSE_* env surface',
      'D-002 honesty rule: no Docker/cluster here — author + lint-validate only (helm lint, helm template golden files, docker compose config, actionlint) and LABEL execution status honestly. Cross-check Helm values against every PULSE_* env the binary reads.'),
    { label: 'INFRA-01:WO-206 helm/compose/ci', phase: 'Implement', schema: AGENT_SCHEMA }),
])

const [chain, sdk, fe, infra] = lanes
const implReports = [chain && chain.be202, chain && chain.be203, chain && chain.be204, sdk, fe, infra].filter(Boolean)
log(`Implement phase done: ${implReports.length}/6 agents returned. Statuses: ` +
  implReports.map(r => (r && r.status) || 'null').join(', '))

// ---------- Phase 2: Gate (barrier is correct: QA verifies the FULLY integrated tree after all commits) ----------

phase('Gate')
const gate = await agent(
  `You are QA-01, the QA & verification agent in the Pulse MVP build, Wave 2. ORCH-00 dispatched you AFTER all six implementation agents committed. Your job: independently verify wave-2 acceptance END-TO-END on the local stack. VERIFY, NEVER FIX — if you find a defect, report it (owner, severity, minimal repro); do not edit product code.

READ FIRST, in order:
1. CLAUDE.md, then your charter agents/definitions/QA-01-qa.md.
2. Your work order: agents/handoffs/wave-2/WO-207.md — execute ALL of it.
3. The six implementation completion reports just written: agents/handoffs/wave-2/WO-201-report.md, WO-202-report.md, WO-203-report.md, WO-204-report.md, WO-205-report.md, WO-206-report.md (use them for dev license keys, tier matrix, health-score formula, interface shapes).
4. docs/ARCHITECTURE.md sec.4 (the budget table) and qa/wave-1/gate-report.md (wave-1 baseline + carried D-W1-006).

Gate (manifest wave 2): "beacon->dashboard round trip; billing reconciles +/-1%" PLUS the wave-2 budget rows of ARCHITECTURE sec.4. Drive the REAL built SDK against the REAL pulse ingest listener; run a known-truth billing scenario and assert every figure within +/-1% and statement gen <60s; verify pulse diag --reconcile agrees; check ingest degradation visible <=15s, node discovery <=2min, 13-month query <3s, /metrics bounded cardinality, SDK size <15KB. Re-run the wave-1 gate (qa/wave-1/run-gate.sh) and full build/lint/test across server, web, sdk for regressions. Write TestAMSVersionMatrix content (D-W1-006) against mock-ams profiles and document which assertions need real AMS containers.

WAIVERS are limited to the D-002 (no Docker) and D-007.5 (no Kafka broker) classes — you may NOT waive anything else; only ORCH-00 waives, in the decision log. If a budget or the round-trip/billing gate fails on real measurement, that is a FAIL or a defect, not a waiver.

Environment: ClickHouse at /tmp/clickhouse (v26.6.1); CGO_ENABLED=0; macOS arm64; no Docker. ${COMMIT}
Write the gate report to qa/wave-2/gate-report.md (PASS/FAIL per criterion with measured numbers, repro commands, defect list with owner/severity/repro, waived items). Scripts must exit nonzero on failure. The gate report IS your completion report. Then return ONLY the forced StructuredOutput object (verdict PASS|PASS_WITH_LIMITATIONS|FAIL, per-criterion verdicts with measured/budget, defects, waivers, committed, summary).`,
  { label: 'QA-01:WO-207 wave-2 gate', phase: 'Gate', schema: GATE_SCHEMA })

log(`QA-01 gate verdict: ${gate ? gate.verdict : 'null'} — ${gate ? (gate.defects || []).length : '?'} defect(s).`)

// ---------- Phase 3: Docs (only on PASS / PASS_WITH_LIMITATIONS) ----------

phase('Docs')
let doc = null
if (gate && (gate.verdict === 'PASS' || gate.verdict === 'PASS_WITH_LIMITATIONS')) {
  doc = await agent(
    mk('DOC-01', 'documentation', 'DOC-01-docs.md', 'WO-208.md',
      'the six wave-2 implementation reports (WO-201..206-report.md) and the QA gate report qa/wave-2/gate-report.md (its doc defects are yours to fix)',
      'Every documented command/config MUST run against the wave-2 build — test them. No documented-but-unimplemented behavior (label Phase-3 roadmap items). Flip the feature-status table (F2 full, F3, F4, F6, F7, F8) to shipped only for what QA verified.'),
    { label: 'DOC-01:WO-208 docs', phase: 'Docs', schema: AGENT_SCHEMA })
} else {
  log('Gate did not pass — skipping DOC-01. ORCH-00 will run a fix-loop.')
}

// ---------- Return summary for ORCH-00 ----------

return {
  implementation: implReports.map(r => ({ status: r.status, report: r.reportPath, committed: r.committed, sha: r.commitSha || '', crs: r.changeRequests || [], gaps: r.gaps || [], summary: r.summary })),
  gate: gate ? { verdict: gate.verdict, defects: gate.defects || [], waivers: gate.waivers || [], report: gate.reportPath, summary: gate.summary } : null,
  docs: doc ? { status: doc.status, report: doc.reportPath, committed: doc.committed, summary: doc.summary } : 'skipped (gate not passed)',
}
