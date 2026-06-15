export const meta = {
  name: 'pulse-val-3b',
  description: 'V3b fix-loop: tier gating, alerting correctness, security, WS/fleet, UI — then full re-gate + docs',
  phases: [
    { title: 'Implement', detail: 'parallel: [BE-02-B alerting -> BE-02-C gating/WS/fleet/security] | FE-01 UI' },
    { title: 'Verify', detail: 'QA-01 full re-gate (waves 1/2/3 + new VDs) + missing tests' },
    { title: 'Docs', detail: 'DOC-01 reconcile + honest feature status (on pass)' },
  ],
}

const ANTISTALL = `
## ANTI-STALL RULES (MANDATORY — a prior run hung 9h on a foreground process)
- NEVER run a server/ClickHouse in the foreground (\`pulse serve\`, \`clickhouse server\`). If you must, background it with a hard cap \`(timeout 60 /tmp/pulse serve ... &)\`, poll briefly, then KILL. PREFER in-process Go tests over real servers.
- EVERY build/test command gets a timeout: \`timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'\`; \`timeout 200 go test -timeout 150s ./internal/<your-pkgs>/...\`. NEVER bare \`go test ./...\` without -timeout. \`npm run test\` only (vitest run), never watch mode, wrapped in \`timeout 180\`.
- If a command emits nothing for ~60s, kill it and take another path. Stay tightly scoped; don't over-explore.
`
const HARD = `
## Scope + quality
- Stay STRICTLY in your charter scope. Do NOT edit contracts/ (frozen; INT-01 already added the \`business\` tier to the enum AND license.go). Metrics in ClickHouse, config in meta. Web UI uses only generated public-API types. CGO_ENABLED=0.
- For EACH fix add/fix a test that would CATCH the bug. These defects survived prior gates because tests asserted the wrong thing — your guard test must fail on the OLD behavior.
`
const ENV = `
## Environment
- /Users/ae/repo/ant-marketplace, branch main. CGO_ENABLED=0; Go 1.26.4; Node v26; macOS arm64; NO Docker; no Kafka broker. ClickHouse /tmp/clickhouse (v26.6.1). *.db*/web/pulse_secret.key gitignored — never commit. Tier model: free|pro|business|enterprise per PRD §7.11 (INT-01 set the entitlement matrix in internal/license/license.go — read it).
`
const COMMIT = `
## Commit (D-008)
Verify (bounded build + your guard tests pass), THEN commit by EXPLICIT path in your scope only (never -A/-u/.; D-011 — list each path). Message '<ID> V3b <VDs>: <summary>' + evidence. No push. Do NOT commit broken/non-building code. Report the SHA. Do NOT touch agents/handoffs/RESUME-PROMPT.md (ORCH-00's file).
`
const SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['status', 'committed', 'fixed', 'summary'],
  properties: {
    status: { type: 'string', enum: ['COMPLETE', 'PARTIAL', 'BLOCKED'] },
    fixed: { type: 'array', items: { type: 'object', additionalProperties: false, required: ['vd', 'verdict'], properties: { vd: { type: 'string' }, verdict: { type: 'string', enum: ['FIXED', 'PARTIAL', 'DEFERRED'] }, test: { type: 'string' } } } },
    committed: { type: 'boolean' }, commitSha: { type: 'string' }, reportPath: { type: 'string' }, summary: { type: 'string' },
  },
}
const GATE = {
  type: 'object', additionalProperties: false,
  required: ['verdict', 'summary'],
  properties: {
    verdict: { type: 'string', enum: ['PASS', 'PASS_WITH_LIMITATIONS', 'FAIL'] },
    checks: { type: 'array', items: { type: 'object', additionalProperties: false, required: ['name', 'verdict'], properties: { name: { type: 'string' }, verdict: { type: 'string', enum: ['PASS', 'FAIL'] }, measured: { type: 'string' } } } },
    remainingDefects: { type: 'array', items: { type: 'string' } }, committed: { type: 'boolean' }, reportPath: { type: 'string' }, summary: { type: 'string' },
  },
}
function head(id, role, charter) {
  return `You are ${id}, the ${role} agent, Pulse MVP V3b fix-loop. Read first: CLAUDE.md, your charter agents/definitions/${charter}, agents/handoffs/validation/V2-triage-report.md (each VD = file:line + fix), and (for tier work) internal/license/license.go (INT-01's entitlement matrix). Apply ONLY your assigned VDs.`
}

phase('Implement')
log('V3b: [BE-02-B alerting -> BE-02-C gating/WS/fleet/security] | FE-01 -> QA full re-gate -> DOC.')

const impl = await parallel([
  async () => {
    const beB = await agent(
      `${head('BE-02', 'backend product-plane', 'BE-02-backend-productplane.md')}
Your VDs (alerting correctness): VD-28 (muted=true must SUPPRESS notifications — add \`if rule.Muted { return }\` in evaluator fire()/resolve(); guard test: a muted rule whose condition fires delivers NOTHING), VD-29 (implement group_by storm grouping — when group_by="stream_id"/"app", emit ONE notification per group key, not per stream; test: N=5 streams same app → 1 notification), VD-30 (node_down must fire for genuinely offline nodes: add LastSeenAt to LiveNodeStats, evict nodes not seen within 3×PollInterval in the aggregator, fire node_down on absence — not the CPU>95 proxy), VD-32 (rebuffer_ratio/error_rate alert metrics: replace the (1-HealthScore) heuristic with real rollup_qoe_1h queries now that QoE rollups are wired; test the >5% threshold), VD-33 (cron parseCronSimple must support ranges like '1-5' → {1,2,3,4,5}; test a weekday-range window), VD-36-server (extend the report cron parser to accept standard 5-field cron so UI presets like '0 6 1 * *' work; test 5-field parsing), VD-34 (fix the wave2 test that never asserts alerts DO fire outside a maintenance window — make it t.Error on 0 notifications).
Acceptance (BOUNDED): \`timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'\` then \`timeout 200 go test -timeout 150s ./internal/alert/... ./internal/reports/...\` green + guard tests.
${ANTISTALL}${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3b-BE02B-report.md. Return ONLY the StructuredOutput object.`,
      { label: 'BE-02-B: alerting correctness', phase: 'Implement', schema: SCHEMA })

    const beC = await agent(
      `${head('BE-02', 'backend product-plane', 'BE-02-backend-productplane.md')}
Read agents/handoffs/validation/V3b-BE02B-report.md FIRST. Your VDs (gating + WS + fleet + security): VD-01+VD-35 (apply the §7.11 entitlement matrix at every gate site — PagerDuty/webhook channels + reports endpoints + multi-tenant + API tokens + Prometheus require BUSINESS; anomaly requires ENTERPRISE; use the CheckX helpers INT-01 added to license.go; add s.lic.Check* to all 5 report handlers + channel creation + tenant CRUD; Free/Pro get 403; tests per tier boundary), VD-15 (beacon ingest requires Pro+ — add a license check in beacon.Handler; test Free→reject), VD-02 (WS /live/ws must broadcast a LiveOverview shape, NOT LiveSnapshot — construct LiveOverview with total_publishers/protocol_mix/apps from the aggregator snapshot in server.go push code; test the serialized shape matches the OpenAPI LiveOverview), VD-39 (FleetNodes() must return real role from cluster discovery, not hardcoded 'standalone' — pass clusterDiscovery into query.New(); call NodeRole()), VD-S1 (metrics token: use subtle.ConstantTimeCompare, not !=), VD-S2 (WebSocket: remove InsecureSkipVerify=true / set explicit allowed origins), VD-S3 (bearer middleware: enforce token kind — admin routes require kind='api', ingest require kind='ingest'). Declare cmd/pulse edits.
Acceptance (BOUNDED): \`timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'\` then \`timeout 220 go test -timeout 180s ./internal/api/... ./internal/license/...\` green incl. kin-openapi conformance + per-tier gate tests.
${ANTISTALL}${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3b-BE02C-report.md. Return ONLY the StructuredOutput object.`,
      { label: 'BE-02-C: gating/WS/fleet/security', phase: 'Implement', schema: SCHEMA })

    return { beB, beC }
  },
  () => agent(
    `${head('FE-01', 'frontend', 'FE-01-frontend.md')}
Regenerate API types FIRST (\`timeout 180 npm run generate:api\`) so the new \`business\` tier + contract fixes are in the generated types; GENERATED types only (git-grep proof). Your VDs: VD-01 (fix the tier upsell logic + copy: the gate must check the REAL required tier per feature — reports/tenants/channels need \`business\`, anomalies need \`enterprise\` — not a hardcoded \`tier==='free'\`; the "requires Business tier" copy must match the actual gate; show the correct upgrade target per feature), VD-36 (report schedule cron presets — ensure the preset values match what the server now accepts (5-field per BE-02-B VD-36); keep the presets working), VD-02 (live dashboard: read the WS payload as LiveOverview — total_publishers/protocol_mix/apps — matching BE-02-C's fix; verify field mapping), VD-X3-B (client.ts: rename the analytics param from \`granularity\` to \`interval\` to match the spec).
Acceptance (BOUNDED, each \`timeout 180\`): \`npm run build\` && \`npm run lint\` && \`npm run test\` (vitest run) all green; component tests for the corrected per-tier gating (business vs enterprise vs free) and WS LiveOverview mapping.
${ANTISTALL}${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3b-FE-report.md. Return ONLY the StructuredOutput object.`,
    { label: 'FE-01: tier copy + WS + params', phase: 'Implement', schema: SCHEMA }),
])

const [chain, fe] = impl
log(`V3b impl: BE-02-B=${chain && chain.beB ? chain.beB.status : 'null'}, BE-02-C=${chain && chain.beC ? chain.beC.status : 'null'}, FE-01=${fe ? fe.status : 'null'}`)

phase('Verify')
const qa = await agent(
  `You are QA-01 (verify, never fix product code; you may add qa/ tests). Pulse MVP V3b FULL re-gate after the validation fix-loop. REBUILD bounded first: \`timeout 160 bash -c 'cd server && CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/'\`; \`timeout 180 bash -c 'cd sdk/beacon-js && npm run build'\`. Read the V3b-*-report.md files + the V2 triage.

Verify with BOUNDED commands (prefer Go tests over live servers — see ANTISTALL):
1. Tier gating (VD-01/35/15): per-tier matrix — Free/Pro blocked from business features (reports, PagerDuty/webhook, multi-tenant, API/Prometheus → 403), Business allowed, anomalies Enterprise-only. Run the BE per-tier tests + a live spot-check if bounded.
2. Alerting (VD-28/29/30/32/33/36): muted suppresses; group_by groups; node_down fires on absence; cron ranges/5-field parse. Run \`timeout 200 go test -timeout 150s ./internal/alert/...\`.
3. Security (VD-S1/S2/S3): constant-time metrics token, WS origin, token-kind scope.
4. WS/fleet (VD-02/39): LiveOverview shape; real fleet roles.
5. FULL REGRESSION (BOUNDED): \`timeout 320 go test -timeout 280s ./...\` (server) ; \`timeout 220 bash -c 'cd web && npm run build && npm run lint && npm run test'\` ; \`timeout 180 bash -c 'cd sdk/beacon-js && npm run test && npm run size'\` ; the wave-1/2/3 gate scripts (bounded).
6. Add any cheap missing qa/ tests you own (e.g. a non-tautological viewer-count check VD-05) — optional, time-permitting.

${ANTISTALL}
Waivers D-002/D-007.5 only. Write agents/handoffs/validation/V3b-QA-gate-report.md (PASS/FAIL per VD + measured + repro + still-open defects with owner/severity). Commit qa/ artifacts (D-008). Return ONLY the StructuredOutput object.`,
  { label: 'QA-01: V3b full re-gate', phase: 'Verify', schema: GATE })

log(`V3b QA verdict: ${qa ? qa.verdict : 'null'} — remaining: ${qa ? (qa.remainingDefects || []).length : '?'}`)

phase('Docs')
let doc = null
if (qa && (qa.verdict === 'PASS' || qa.verdict === 'PASS_WITH_LIMITATIONS')) {
  doc = await agent(
    `${head('DOC-01', 'documentation', 'DOC-01-docs.md')}
The validation fix-loop (V3a+V3b) changed real behavior. Reconcile the docs to TRUTH: read the V3a-*/V3b-* reports + qa/wave-3 + V3b-QA-gate-report. Update docs/ARCHITECTURE.md + README.md feature-status to reflect what is NOW actually functional (beacon round-trip, geo/device analytics, ingest health, tier model with Business tier, alerting muted/group_by, etc.) and HONESTLY list remaining known limitations (the V2 P3/deferred items + any still-open). Update the tier/pricing docs to the 4-tier model (§7.11). Update docs/runbooks/alerting.md + reports.md for the corrected behavior. Every documented command must run (bounded). Do NOT overstate — label Phase-3/deferred items.
Acceptance: docs match the verified build; no documented-but-unimplemented behavior.
${ANTISTALL}${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3b-DOC-report.md. Return ONLY the StructuredOutput object.`,
    { label: 'DOC-01: reconcile docs', phase: 'Docs', schema: SCHEMA })
} else {
  log('V3b gate did not pass — skipping DOC; ORCH-00 will iterate.')
}

return {
  be02b: chain && chain.beB ? { status: chain.beB.status, sha: chain.beB.commitSha || '', fixed: chain.beB.fixed || [], summary: chain.beB.summary } : null,
  be02c: chain && chain.beC ? { status: chain.beC.status, sha: chain.beC.commitSha || '', fixed: chain.beC.fixed || [], summary: chain.beC.summary } : null,
  fe: fe ? { status: fe.status, sha: fe.commitSha || '', fixed: fe.fixed || [], summary: fe.summary } : null,
  qa: qa ? { verdict: qa.verdict, checks: qa.checks || [], remainingDefects: qa.remainingDefects || [], report: qa.reportPath, summary: qa.summary } : null,
  doc: doc ? { status: doc.status, sha: doc.commitSha || '', summary: doc.summary } : 'skipped',
}
