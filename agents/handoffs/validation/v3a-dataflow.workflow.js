export const meta = {
  name: 'pulse-val-3a',
  description: 'V3a fix-loop: make the data flow — contract, data-plane enrichment/health, ingest pipelines, SDK header',
  phases: [
    { title: 'Contract', detail: 'INT-01: business tier enum + contract conformance fixes' },
    { title: 'Implement', detail: 'parallel: [BE-01 data-plane -> BE-02-A pipelines] | SDK-01' },
    { title: 'Verify', detail: 'QA-01 mini: real-SDK round-trip, geo/device, health>0, timeseries' },
  ],
}

const HARD = `
## Hard rules (CLAUDE.md + ARCHITECTURE §3-4)
- Stay STRICTLY in your scope (agents/manifest.yaml). Do NOT edit contracts/ unless you are INT-01 (the only CR-applier). Metrics in ClickHouse, config in meta — never crossed. AMS wire formats only in amsclient+collector. Web UI uses only generated public-API types. CGO_ENABLED=0 always.
- For EACH fix you make, add or fix a test that would have CAUGHT the bug (these defects survived prior gates precisely because tests asserted the wrong thing or bypassed the real path — D-W2-002 lesson). A fix without a guarding test is incomplete.
`
const ENV = `
## Environment
- /Users/ae/repo/ant-marketplace, branch main (no new branches). CGO_ENABLED=0; Go 1.26.4; Node v26; macOS arm64; NO Docker (D-002); no Kafka broker (D-007.5). Fresh binaries at /tmp/pulse, /tmp/mock-ams; ClickHouse at /tmp/clickhouse (v26.6.1). web/pulse_secret.key + *.db* are gitignored — never commit.
`
const COMMIT = `
## Commit protocol (D-008)
Verify your acceptance (build + your new tests pass), THEN commit by EXPLICIT path inside your scope only (NEVER 'git add -A'/'-u'/'.' — D-011). Message '<YOUR-AGENT-ID> V3a <VD-ids>: <summary>' + evidence. No push. index.lock busy ⇒ wait+retry, never delete. Report your commit SHA.
`
const SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['status', 'committed', 'fixed', 'summary'],
  properties: {
    status: { type: 'string', enum: ['COMPLETE', 'PARTIAL', 'BLOCKED'] },
    fixed: { type: 'array', items: { type: 'object', additionalProperties: false, required: ['vd', 'verdict'], properties: { vd: { type: 'string' }, verdict: { type: 'string', enum: ['FIXED', 'PARTIAL', 'DEFERRED'] }, test: { type: 'string' }, note: { type: 'string' } } } },
    committed: { type: 'boolean' }, commitSha: { type: 'string' },
    reportPath: { type: 'string' }, newCRs: { type: 'array', items: { type: 'string' } }, summary: { type: 'string' },
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
  return `You are ${id}, the ${role} agent, in the Pulse MVP V3a fix-loop. ORCH-00 dispatched you to FIX specific validation defects. Read first: CLAUDE.md, your charter agents/definitions/${charter}, and the triage report agents/handoffs/validation/V2-triage-report.md — each VD row has the exact file:line evidence + prescribed fix. Apply ONLY your assigned VDs (below); leave others for their owners.`
}

phase('Contract')
log('V3a: make the data flow. INT-01 contract -> [BE-01 -> BE-02-A] | SDK-01 -> QA mini.')

const intRes = await agent(
  `${head('INT-01', 'integration/contracts', 'INT-01-integration.md')}
You ARE authorized to edit contracts/ for these approved fixes (D-014 + V2 conformance findings) — touch ONLY what these VDs require:
- VD-01: add \`business\` to the License.tier enum (order: free, pro, business, enterprise) everywhere it appears in pulse-api.yaml; add a short entitlement-matrix description per PRD §7.11 to the License schema docs.
- VD-X3-A: AmsSourceStatus must include \`reachable: boolean\` (required).
- VD-X3-D: add a 403 response to GET /anomalies.
- VD-X3-C: document delete endpoints as idempotent (204 always) — adjust the spec to NOT require 404 on DELETE /admin/tokens|users (or document the chosen semantics); pick idempotent-204 and make the spec consistent.
- VD-S4: set the beacon ingest body-size cap in the spec to 64 KB (the hardened handler is authoritative).
Acceptance: redocly lint 0 errors; kin-openapi still loads (go test ./internal/api/... compiles). Regenerate NOTHING (FE/BE do). Report the exact schema/enum/response changes.
${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3a-INT-report.md. Return ONLY the StructuredOutput object.`,
  { label: 'INT-01: contract fixes', phase: 'Contract', schema: SCHEMA })

phase('Implement')
const impl = await parallel([
  async () => {
    const be1 = await agent(
      `${head('BE-01', 'backend data-plane', 'BE-01-backend-dataplane.md')}
Your VDs (read each row for file:line + fix): VD-07 (pass geoResolver+uaParser into restpoller.Config at serve.go), VD-08 (in beacon batchToDomain, extract client IP X-Forwarded-For/RemoteAddr + User-Agent, call geoResolver+uaParser, populate Enrichment before stitching), VD-03 (implement IsEdgeStream() from NodeInfo.Role + dedup edge viewer counts in aggregator), VD-20a (bridge HealthTracker.PublisherState → aggregator: call UpdateIngestHealth() in onIngestStats so LiveStream.HealthScore is non-zero), VD-22 (emit EventIngestStats from REST NormalizeBroadcast: map BroadcastDTO CurrentFPS etc.), VD-40 (add Version to ClusterNodeDTO→NodeInfo→FleetNode), VD-17 (make BuildTestMMDB produce a valid mmdb so TestGeo_MMDBFixture runs, not skips), VD-16+VD-25 (doc-accuracy: REST-no-viewer-IP note; keyframe threshold comment/dead const).
NOTE the BE-01/BE-02 seam: BE-02-A runs AFTER you and depends on EventIngestStats flowing + IsEdgeStream + the health bridge — document any new/changed signatures in your report. Acceptance: CGO_ENABLED=0 go build/vet/test ./... green + new regression tests (esp. a test asserting LiveStream.HealthScore>0 after ingest stats, and geo/UA enrichment populated on the beacon path).
${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3a-BE01-report.md. Return ONLY the StructuredOutput object.`,
      { label: 'BE-01: data-plane fixes', phase: 'Implement', schema: SCHEMA })

    const be2a = await agent(
      `${head('BE-02', 'backend product-plane', 'BE-02-backend-productplane.md')}
Read BE-01's agents/handoffs/validation/V3a-BE01-report.md FIRST (it just landed the health bridge + EventIngestStats + IsEdgeStream you build on). Your VDs: VD-10 (main-port /ingest/beacon MUST write to EventSink — wire an EventSink into api.Server OR set PULSE_INGEST_LISTEN_ADDR routing through the hardened handler; align body cap to 64 KB; the default deployment must actually persist beacons), VD-06 (implement geo + device breakdown queries in internal/query GROUP BY geo_country/client_device; wire into handleGeoAnalytics/handleDeviceAnalytics — currently stubs returning []), VD-11 (implement /qoe/summary from rollup_qoe_1h in internal/query incl. startup_p50_ms; fix bitrate timeline field name to bitrate_kbps_p50), VD-20b (return the now-nonzero health score on GET /qoe/ingest), VD-21 (compute + return ingest timeseries + drop_events per the OpenAPI IngestStream schema — UI renders these), VD-23 (fix api.IngestTracker.Snapshot() return type to match ingest.HealthTracker; call SetIngestTracker from serve.go), VD-37/VD-38 (egress_method label when bytes-branch taken; note peak_concurrency rollup caveat), VD-X3-A handler (add reachable to source-test response), VD-X3-C handler (idempotent delete per INT-01's spec choice).
Declare cmd/pulse edits (D-005). Acceptance: CGO_ENABLED=0 go build/vet/test ./... green; kin-openapi conformance; NEW tests that seed data and assert geo/device endpoints return NON-EMPTY rows, /qoe/ingest returns health_score>0 with timeseries, and a real beacon POST to the DEFAULT main port lands in a test EventSink.
${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3a-BE02A-report.md. Return ONLY the StructuredOutput object.`,
      { label: 'BE-02-A: pipeline fixes', phase: 'Implement', schema: SCHEMA })

    return { be1, be2a }
  },
  () => agent(
    `${head('SDK-01', 'beacon SDK', 'SDK-01-beacon-sdk.md')}
Your VDs: VD-09 (CRITICAL — transport.ts sends header 'X-Pulse-Token' but the server + OpenAPI require 'X-Pulse-Ingest-Token'; every real browser beacon currently 401s. Change the header name; ensure the sendBeacon path also carries auth or document why it can't and fall back to fetch+keepalive when a token is required. Add a test asserting the exact header name the SDK emits), VD-12 (HlsAdapter must emit rebuffer_end on FRAG_BUFFERED after a stall — currently only MediaElementAdapter does, so attachHls() users accumulate unbounded open stalls), VD-13 (HlsAdapter._onLevelSwitched: populate from_kbps/to_kbps from hls.levels[].bitrate instead of 0/0).
Acceptance: npm run build && npm run size (still <15 KB — report measured) && npm run lint && npm run test green; new tests for the header name + rebuffer_end emission. Re-confirm the gzip size.
${HARD}${ENV}${COMMIT}
Write agents/handoffs/validation/V3a-SDK-report.md. Return ONLY the StructuredOutput object.`,
    { label: 'SDK-01: header + HLS fixes', phase: 'Implement', schema: SCHEMA }),
])

const [chain, sdk] = impl
log(`V3a implement: BE-01=${chain && chain.be1 ? chain.be1.status : 'null'}, BE-02-A=${chain && chain.be2a ? chain.be2a.status : 'null'}, SDK-01=${sdk ? sdk.status : 'null'}`)

phase('Verify')
const qa = await agent(
  `You are QA-01 (verify, never fix product code). ORCH-00 dispatched you to verify the V3a data-flow fixes on the LIVE stack. REBUILD /tmp/pulse + the SDK FIRST (D-013 — never test a stale binary): CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/ ; cd sdk/beacon-js && npm run build. Read the four V3a-*-report.md files + the triage rows for VD-09/10/06/20/21/11.

Verify on a live stack (/tmp/clickhouse + pulse, default ports): (1) VD-09/VD-10 — drive the REAL built SDK (its actual header) against the DEFAULT main-port /ingest/beacon with a valid ingest token → 202 AND the event is persisted (visible via the QoE/analytics API or a direct ClickHouse query); a tampered token → 401. This is the round trip that was broken. (2) VD-06 — seed viewer_sessions with geo/device dims, assert GET geo + device analytics return NON-EMPTY rows. (3) VD-20/VD-21 — drive ingest stats, assert GET /qoe/ingest health_score>0 and timeseries non-empty. (4) VD-11 — assert /qoe/summary returns a real startup_p50_ms (not always 0) and the bitrate field name matches the spec. Then full regression: CGO_ENABLED=0 go test ./... ; web build/lint/test ; sdk size. Waivers D-002/D-007.5 only.

Write agents/handoffs/validation/V3a-QA-report.md (PASS/FAIL per check + measured + repro + any still-open defects). Commit your qa/ artifacts (D-008). Return ONLY the StructuredOutput object.`,
  { label: 'QA-01: V3a mini-verify', phase: 'Verify', schema: GATE })

log(`V3a QA verdict: ${qa ? qa.verdict : 'null'}`)

return {
  contract: intRes ? { status: intRes.status, sha: intRes.commitSha || '', fixed: intRes.fixed || [], summary: intRes.summary } : null,
  be01: chain && chain.be1 ? { status: chain.be1.status, sha: chain.be1.commitSha || '', fixed: chain.be1.fixed || [], summary: chain.be1.summary } : null,
  be02a: chain && chain.be2a ? { status: chain.be2a.status, sha: chain.be2a.commitSha || '', fixed: chain.be2a.fixed || [], summary: chain.be2a.summary } : null,
  sdk: sdk ? { status: sdk.status, sha: sdk.commitSha || '', fixed: sdk.fixed || [], summary: sdk.summary } : null,
  qa: qa ? { verdict: qa.verdict, checks: qa.checks || [], remainingDefects: qa.remainingDefects || [], summary: qa.summary } : null,
}
