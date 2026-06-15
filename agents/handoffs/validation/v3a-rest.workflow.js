export const meta = {
  name: 'pulse-val-3a-rest',
  description: 'V3a re-run (INT-01 done): data-plane + ingest pipelines + SDK header, anti-stall hardened',
  phases: [
    { title: 'Implement', detail: 'parallel: [BE-01 -> BE-02-A1 -> BE-02-A2] | SDK-01' },
    { title: 'Verify', detail: 'QA-01 mini: header match, ingest->sink, geo/device, health>0 (bounded)' },
  ],
}

const ANTISTALL = `
## ANTI-STALL RULES (MANDATORY — the prior V3a run hung for 9 h on a foreground process; do NOT repeat)
- NEVER run a long-running server in the foreground. Do NOT run \`/tmp/pulse serve\`, \`clickhouse server\`, \`pulse serve\`, or any blocking process directly — it will hang your shell forever. If you truly need a live process: start it backgrounded with a hard cap, e.g. \`(timeout 60 /tmp/pulse serve ... &)\`, capture the PID, poll with a bounded \`timeout\`, then KILL it. Strongly PREFER in-process Go tests (httptest / a test ClickHouse via the existing integration harness) over starting real servers.
- EVERY build/test command gets an explicit timeout: \`timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'\`; \`timeout 180 go test -timeout 150s ./internal/<your-pkgs>/...\`. NEVER run bare \`go test ./...\` without \`-timeout\`. Scope tests to YOUR packages, not the whole tree.
- For the web/SDK: use \`npm run test\` (it is \`vitest run\`, exits). NEVER run \`vitest\`/\`npx vitest\` without \`run\` (watch mode hangs). Put a \`timeout 180\` on npm commands.
- If any command produces no output for ~60s, assume it hung: kill it (Ctrl-C/kill) and take a different path. Do not wait.
- Stay tightly scoped to your assigned VDs. Do not over-explore. Read the triage rows (they have file:line + fix), make the change, add the guarding test, verify with bounded commands, commit, return.
`
const HARD = `
## Scope + quality
- Stay STRICTLY in your charter scope (agents/manifest.yaml). Do NOT edit contracts/ (INT-01 already amended it — \`business\` is in the enum AND in license.go). Metrics in ClickHouse, config in meta. AMS formats only in amsclient+collector. CGO_ENABLED=0.
- For EACH fix add/fix a test that would have CAUGHT the bug (these survived prior gates because tests bypassed the real path). Do NOT apply tier GATING here (that is V3b) — only the data-flow VDs assigned to you.
`
const ENV = `
## Environment
- /Users/ae/repo/ant-marketplace, branch main. CGO_ENABLED=0; Go 1.26.4; Node v26; macOS arm64; NO Docker; no Kafka broker. ClickHouse at /tmp/clickhouse (v26.6.1). *.db* + web/pulse_secret.key gitignored — never commit.
`
const COMMIT = `
## Commit (D-008)
Verify (bounded build + your new tests pass), THEN commit by EXPLICIT path in your scope only (never -A/-u/.; D-011). Message '<ID> V3a <VDs>: <summary>' + evidence. No push. index.lock busy ⇒ wait+retry. Report the SHA. If you could NOT finish/verify, report PARTIAL and do NOT commit broken code (the prior run left a non-building tree).
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
    remainingDefects: { type: 'array', items: { type: 'string' } }, committed: { type: 'boolean' }, summary: { type: 'string' },
  },
}
function head(id, role, charter) {
  return `You are ${id}, the ${role} agent, Pulse MVP V3a fix-loop (re-run). Read first: CLAUDE.md, your charter agents/definitions/${charter}, and agents/handoffs/validation/V2-triage-report.md (each VD row = exact file:line + fix). Apply ONLY your assigned VDs.`
}

phase('Implement')
log('V3a re-run (hardened): [BE-01 -> BE-02-A1 -> BE-02-A2] | SDK-01 -> QA mini.')

const impl = await parallel([
  async () => {
    const be1 = await agent(
      `${head('BE-01', 'backend data-plane', 'BE-01-backend-dataplane.md')}
Your VDs: VD-07 (pass geoResolver+uaParser into restpoller.Config in serve.go), VD-08 (in beacon batchToDomain extract client IP X-Forwarded-For/RemoteAddr + User-Agent, call geoResolver+uaParser, populate Enrichment before stitching — FINISH the signature change cleanly, no unused imports), VD-03 (implement IsEdgeStream() from NodeInfo.Role + dedup edge viewer counts in aggregator), VD-20a (bridge HealthTracker → aggregator: call UpdateIngestHealth() in onIngestStats so LiveStream.HealthScore is non-zero), VD-22 (emit EventIngestStats from REST NormalizeBroadcast mapping BroadcastDTO fields), VD-40 (Version through ClusterNodeDTO→NodeInfo→FleetNode), VD-17 (valid test mmdb so TestGeo_MMDBFixture runs not skips), VD-16+VD-25 (doc-accuracy comments).
BE-02 runs after you and depends on EventIngestStats + IsEdgeStream + the health bridge — list new/changed signatures in your report. Acceptance (BOUNDED commands): \`timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'\` then \`timeout 200 go test -timeout 150s ./internal/collector/... ./internal/aggregator/... ./internal/cluster/...\` green + new tests (esp. LiveStream.HealthScore>0 after ingest stats; beacon enrichment populated).
${ANTISTALL}${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3a-BE01-report.md. Return ONLY the StructuredOutput object.`,
      { label: 'BE-01: data-plane', phase: 'Implement', schema: SCHEMA })

    const be2a1 = await agent(
      `${head('BE-02', 'backend product-plane', 'BE-02-backend-productplane.md')}
Read BE-01's agents/handoffs/validation/V3a-BE01-report.md FIRST. Your VDs (the three heavy pipeline/query fixes): VD-10 (main-port /ingest/beacon MUST persist to EventSink — wire an EventSink into api.Server so the DEFAULT deployment actually stores beacons; align body cap to 64 KB), VD-06 (implement geo + device breakdown queries in internal/query, GROUP BY geo_country / client_device; wire into handleGeoAnalytics/handleDeviceAnalytics — currently stubs returning []), VD-11 (implement /qoe/summary from rollup_qoe_1h in internal/query incl. real startup_p50_ms; fix bitrate timeline field name to bitrate_kbps_p50). Declare cmd/pulse edits. Do NOT apply tier gating (V3b).
Acceptance (BOUNDED): \`timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'\` then \`timeout 240 go test -timeout 200s ./internal/api/... ./internal/query/...\` (+ \`-tags integration\` for the CH-backed query tests you add) green; NEW tests: a beacon POST to the DEFAULT main port lands in a test EventSink; seeded geo/device queries return NON-EMPTY rows.
${ANTISTALL}${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3a-BE02A1-report.md. Return ONLY the StructuredOutput object.`,
      { label: 'BE-02-A1: ingest+geo+qoe', phase: 'Implement', schema: SCHEMA })

    const be2a2 = await agent(
      `${head('BE-02', 'backend product-plane', 'BE-02-backend-productplane.md')}
Read agents/handoffs/validation/V3a-BE01-report.md and V3a-BE02A1-report.md FIRST. Your VDs (ingest-health API + small conformance): VD-20b (return the now-nonzero health score on GET /qoe/ingest), VD-21 (compute + return ingest timeseries + drop_events per the OpenAPI IngestStream schema — the UI renders these), VD-23 (fix api.IngestTracker.Snapshot() return type to match ingest.HealthTracker; call SetIngestTracker from serve.go), VD-37 (set correct egress_method label when the bytes branch is taken), VD-38 (code comment documenting the peak_concurrency rollup caveat), VD-X3-A handler (add \`reachable\` to the source-test response per the amended spec), VD-X3-C handler (idempotent delete per INT-01's spec choice). Declare cmd/pulse edits.
Acceptance (BOUNDED): \`timeout 150 bash -c 'CGO_ENABLED=0 go build ./...'\` then \`timeout 200 go test -timeout 150s ./internal/api/...\` green incl. kin-openapi conformance; NEW test: GET /qoe/ingest returns health_score>0 with non-empty timeseries.
${ANTISTALL}${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3a-BE02A2-report.md. Return ONLY the StructuredOutput object.`,
      { label: 'BE-02-A2: ingest-health API', phase: 'Implement', schema: SCHEMA })

    return { be1, be2a1, be2a2 }
  },
  () => agent(
    `${head('SDK-01', 'beacon SDK', 'SDK-01-beacon-sdk.md')}
Your VDs: VD-09 (CRITICAL — transport.ts sends header 'X-Pulse-Token'; server + OpenAPI require 'X-Pulse-Ingest-Token'. Change it so the SDK sends the correct header. Add a test asserting the EXACT header string the SDK emits, so it can never silently drift again), VD-12 (HlsAdapter must emit rebuffer_end on FRAG_BUFFERED after a stall — currently only MediaElementAdapter does), VD-13 (HlsAdapter level-switch: populate from_kbps/to_kbps from hls.levels[].bitrate, not 0/0).
Acceptance (BOUNDED, each prefixed \`timeout 180\`): \`npm run build\` && \`npm run size\` (report measured gzip, must be <15 KB) && \`npm run lint\` && \`npm run test\` (= vitest run; ALL tests pass — do not leave failing tests). New tests for the header name + rebuffer_end.
${ANTISTALL}${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3a-SDK-report.md. Return ONLY the StructuredOutput object.`,
    { label: 'SDK-01: header + HLS', phase: 'Implement', schema: SCHEMA }),
])

const [chain, sdk] = impl
log(`V3a impl: BE-01=${chain && chain.be1 ? chain.be1.status : 'null'}, A1=${chain && chain.be2a1 ? chain.be2a1.status : 'null'}, A2=${chain && chain.be2a2 ? chain.be2a2.status : 'null'}, SDK=${sdk ? sdk.status : 'null'}`)

phase('Verify')
const qa = await agent(
  `You are QA-01 (verify, never fix product code), Pulse MVP V3a mini-gate. Read the four V3a-*-report.md files + triage rows VD-09/10/06/20/21/11. REBUILD bounded first: \`timeout 160 bash -c 'cd server && CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/'\` and \`timeout 180 bash -c 'cd sdk/beacon-js && npm run build'\`.

Verify with BOUNDED commands (favor the agents' Go tests over live servers):
1. VD-09 (no server needed): assert the SDK's outgoing header constant equals the server's expected header ('X-Pulse-Ingest-Token') — grep both and compare, and run the SDK's header test.
2. VD-10/06/20/21/11: run the BE agents' new tests: \`timeout 300 go test -timeout 250s -tags integration ./internal/api/... ./internal/query/...\` — confirm beacon→EventSink, geo/device non-empty, health_score>0 + timeseries, /qoe/summary real startup_p50 + correct bitrate field.
3. Regression (BOUNDED): \`timeout 300 go test -timeout 250s ./...\` (server) ; \`timeout 200 bash -c 'cd web && npm run build && npm run test'\` ; \`timeout 180 bash -c 'cd sdk/beacon-js && npm run test && npm run size'\`.
OPTIONAL live round-trip ONLY if bounded: start pulse backgrounded with \`(timeout 45 /tmp/pulse serve ... &)\`, poll briefly, POST a beacon with the SDK's header, then KILL it. Skip if it risks hanging — the Go tests are sufficient evidence.

${ANTISTALL}
Waivers D-002/D-007.5 only. Write agents/handoffs/validation/V3a-QA-report.md (PASS/FAIL per check + measured + repro + still-open defects). Commit qa/ artifacts (D-008). Return ONLY the StructuredOutput object.`,
  { label: 'QA-01: V3a mini-verify', phase: 'Verify', schema: GATE })

log(`V3a QA verdict: ${qa ? qa.verdict : 'null'}`)

return {
  be01: chain && chain.be1 ? { status: chain.be1.status, sha: chain.be1.commitSha || '', fixed: chain.be1.fixed || [], summary: chain.be1.summary } : null,
  be02a1: chain && chain.be2a1 ? { status: chain.be2a1.status, sha: chain.be2a1.commitSha || '', fixed: chain.be2a1.fixed || [], summary: chain.be2a1.summary } : null,
  be02a2: chain && chain.be2a2 ? { status: chain.be2a2.status, sha: chain.be2a2.commitSha || '', fixed: chain.be2a2.fixed || [], summary: chain.be2a2.summary } : null,
  sdk: sdk ? { status: sdk.status, sha: sdk.commitSha || '', fixed: sdk.fixed || [], summary: sdk.summary } : null,
  qa: qa ? { verdict: qa.verdict, checks: qa.checks || [], remainingDefects: qa.remainingDefects || [], summary: qa.summary } : null,
}
