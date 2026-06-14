export const meta = {
  name: 'pulse-val-2-adversarial',
  description: 'Validation V2: adversarial F1-F10 + cross-cutting verification vs PRD §7 / ARCHITECTURE §4, then triage',
  phases: [
    { title: 'Verify', detail: '10 feature verifiers + 4 cross-cutting critics, adversarial, verify-only' },
    { title: 'Triage', detail: 'dedup + severity-rank all findings into one report' },
  ],
}

const FINDINGS_SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['area', 'verdict', 'findings', 'summary'],
  properties: {
    area: { type: 'string' },
    verdict: { type: 'string', enum: ['MEETS', 'PARTIAL', 'FAILS'] },
    findings: { type: 'array', items: { type: 'object', additionalProperties: false,
      required: ['title', 'severity', 'confidence', 'evidence'],
      properties: {
        title: { type: 'string' },
        severity: { type: 'string', enum: ['critical', 'major', 'minor', 'cosmetic'] },
        confidence: { type: 'string', enum: ['high', 'medium', 'low'] },
        prdCriterion: { type: 'string' },
        evidence: { type: 'string' },
        repro: { type: 'string' },
        suggestedOwner: { type: 'string' },
      } } },
    budgetsChecked: { type: 'array', items: { type: 'object', additionalProperties: false,
      required: ['budget', 'status'],
      properties: { budget: { type: 'string' }, target: { type: 'string' }, measured: { type: 'string' }, status: { type: 'string', enum: ['PASS', 'FAIL', 'UNVERIFIED'] } } } },
    reportPath: { type: 'string' },
    summary: { type: 'string' },
  },
}

const TRIAGE_SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['totalFindings', 'confirmed', 'summary'],
  properties: {
    totalFindings: { type: 'integer' },
    confirmed: { type: 'array', items: { type: 'object', additionalProperties: false,
      required: ['id', 'title', 'severity', 'owner', 'area'],
      properties: { id: { type: 'string' }, title: { type: 'string' }, severity: { type: 'string', enum: ['critical', 'major', 'minor', 'cosmetic'] }, owner: { type: 'string' }, area: { type: 'string' }, fix: { type: 'string' }, mvpBlocking: { type: 'boolean' } } } },
    droppedLowConfidence: { type: 'array', items: { type: 'string' } },
    bySeverity: { type: 'object', additionalProperties: true },
    reportPath: { type: 'string' },
    summary: { type: 'string' },
  },
}

const RULES = `
You VERIFY ONLY — do NOT edit product code, do NOT commit, do NOT fix anything. Your job is to find what is wrong, missing, inconsistent, or untested. Be adversarial: assume the prior agents over-claimed. For each claim in the PRD acceptance criteria, check whether the implementation AND its tests genuinely satisfy it — a passing test that asserts the wrong thing, or a budget "met" only by a unit test that bypasses the real path (see D-W2-002), is a FINDING.

Method, in order of preference: (1) read the PRD §7 feature text + ARCHITECTURE §3-4 + the relevant contract + the implementation + its tests, and reason about gaps; (2) run EXISTING targeted tests (go test ./internal/<pkg>/..., npm test) to confirm; (3) only if a live check is essential, use an ISOLATED setup — pulse on a feature-unique port, a uniquely-named ClickHouse database, tear down after — never assume a shared running server. Binaries are freshly built at /tmp/pulse and /tmp/mock-ams. ClickHouse binary at /tmp/clickhouse (v26.6.1).

Environment: /Users/ae/repo/ant-marketplace, branch main, CGO_ENABLED=0, macOS arm64, NO Docker (D-002), no Kafka broker (D-007.5) — do not file those as defects (they are accepted waivers). Report MEASURED numbers where you check a budget. Rate each finding's confidence honestly; the triage step drops low-confidence noise.

Write your findings to agents/handoffs/validation/findings-<AREA>.md, then return ONLY the StructuredOutput object.
`

function verifier(area, focus) {
  return `You are an adversarial validation verifier (QA-class, verify-only) for the Pulse MVP. ORCH-00 dispatched you to validate **${area}** against the PRD and architecture. Read first: CLAUDE.md, prd-report.md §7 (find ${area}'s feature text + acceptance criterion), docs/ARCHITECTURE.md §3 (boundary rules) + §4 (numeric budgets), and the relevant contracts/ + server/ + web/ + sdk/ code and tests.

## Your focus
${focus}
${RULES}`
}

// ---------- Phase 1: Verify (parallel fan-out) ----------

phase('Verify')
log('V2 adversarial validation: 10 feature verifiers + 4 cross-cutting critics, then triage.')

const AREAS = [
  ['F1-live-dashboard', 'F1 real-time dashboard + collector. PRD acceptance: stream appears on dashboard ≤10s of going live; viewer counts within ±2%; dashboard renders <2s at 500 concurrent streams. Verify the collector→aggregator→WS/API→UI path is real, counts are accurate (not double-counted), the WS delta path works (wave-1 G7), and the budgets hold. Check F1 is not silently degraded by wave-2/3 changes.'],
  ['F2-audience-analytics', 'F2 audience analytics (geo/device/protocol, time-series). PRD: IP-derived geo, anonymizable; full breakdowns; 13-month query <3s; storage ~1-2 GB per million sessions. Verify geo enrichment (mmdb reader, anonymize-IP zeroing BEFORE storage), device/UA parsing, the 13-month rollup query budget (live), CSV export, and that GAP-2-001 (invalid test mmdb SKIP) does not hide a real lookup bug.'],
  ['F3-qoe-beacons', 'F3 QoE beacons (SDK + ingest + QoE surface). PRD: one-line init, CMCD-aligned events, SDK <15KB gzip, graceful no-op, startup/rebuffer/error metrics visible. Verify the real SDK→ingest→QoE round trip, the SDK size, schema conformance, and GAP-2-005 (/qoe/summary uses a live health-score proxy, NOT rollup_qoe_1h CH queries) — is the QoE summary actually backed by beacon data or a proxy? That is a likely real finding.'],
  ['F4-ingest-health', 'F4 ingest/publisher health. PRD: per-publisher bitrate/fps/keyframe/packet-loss/jitter; degradation visible ≤15s; reproducible health-score formula. Verify the metrics are really extracted, the health score matches its documented formula, degradation detection ≤15s, and GAP-2-003 (Kafka lag/parseErrors not surfaced in /healthz).'],
  ['F5-alerting', 'F5 alerting. PRD: rule types (threshold + the wave-2 additions), channels email/Slack/Telegram/PagerDuty/webhook(HMAC), maintenance windows, alert detect→notify <30s, enabled-vs-muted semantics. Verify all five channels really work (adapters + signatures), cert_expiry/cron windows, the <30s budget, and the TIER GATING of channels vs PRD §7.11 (PagerDuty/webhook should be BUSINESS tier — see decision D-014; the impl may mis-gate to enterprise).'],
  ['F6-reports-billing', 'F6 usage/billing reports. PRD: viewer-minutes/egress/recording per app/stream/tenant; egress method DISCLOSED per row; tenant mapping (glob+tag, precedence, unassigned); statements <60s & reconcile ±1%; CSV/PDF; schedules; S3 export; white-label header. Verify the LIVE billing path (D-W2-002 is fixed — confirm it stays fixed against rollup_usage_1d), per-tenant reconciliation, the new /admin/tenants CRUD (just landed), egress method disclosure, and TIER gating (reports/multi-tenant = BUSINESS per D-014, may be mis-gated to enterprise).'],
  ['F7-fleet', 'F7 multi-node fleet. PRD: node discovery ≤2min; origin/edge roles; viewer dedup (count at edge only when served via edges); node up/down → alerts. Verify discovery budget, and GAP-2-002 (edge/origin dedup IsEdgeStream() always false) — is fleet aggregation actually correct for multi-node, or does it double-count? Likely a real PARTIAL.'],
  ['F8-api-prometheus', 'F8 public API + Prometheus. PRD: read-only API over rollups; /metrics gauges/counters only, bounded cardinality (no stream/session labels); Grafana-friendly. Verify /metrics cardinality bounds under load, API token auth, that the web UI uses ONLY the public API (ARCH §3), and TIER gating (API tokens + Prometheus = BUSINESS per D-014).'],
  ['F9-anomaly', 'F9 anomaly detection. PRD: baseline-deviation flags on viewers/errors/rebuffering; simple stats, no ML; <1 false alarm per node-week at default sensitivity. Verify the baseline math (Welford), the false-alarm rate claim (0.2594/node-week — is the model sound or gamed by hysteresis?), GAP-3-004 (zero-stddev blind spot: a constant metric never flags ANY deviation — real robustness gap), and that anomaly = ENTERPRISE tier (correct per PRD).'],
  ['F10-probes', 'F10 synthetic probes. PRD: probes periodically play streams, report success/latency from outside; results visible alongside organic QoE with CLEAR labeling. Verify the probe runner (HLS real; webrtc/rtmp/dash honesty), probe_results round trip, the synthetic labeling in the UI, GAP-3-001 (HLS TTFB manifest-only) + GAP-3-003 (master-playlist bitrate=0), and tier gating (probes = PRO+).'],
  ['X1-architecture-boundaries', 'Cross-cutting: ARCHITECTURE §3 boundary rules. Verify: AMS wire formats appear ONLY in server/pkg/amsclient + server/internal/collector (grep for AMS-shaped parsing elsewhere); metrics live in ClickHouse and config in the meta store, never crossed; the web UI consumes ONLY the generated public-API types (no hand-rolled API shapes, no direct store access); CGO_ENABLED=0 single binary with serve|migrate|diag. Report any boundary violations.'],
  ['X2-tier-model', 'Cross-cutting: the tier model (decision D-014 — likely the biggest finding). PRD §7.11 defines FOUR tiers (Free/Pro/Business/Enterprise) with specific feature allocations; the impl License.tier enum is only free|pro|enterprise (Business MISSING). Produce the COMPLETE impact map: every gate call site in internal/license + handlers, every feature the PRD assigns to Business that is currently gated to enterprise (or pro, or ungated), the UI upsell copy mismatch, and the exact entitlement matrix the fix must implement. This drives the V3 fix.'],
  ['X3-contract-conformance', 'Cross-cutting: contract conformance. Verify EVERY path/operation in contracts/openapi/pulse-api.yaml has a real server implementation (list any 404/stub/unimplemented ops), responses match the schemas (kin-openapi), the error envelope is consistent, generated web types are current, and there are no contract drift points (impl shapes diverging from the frozen spec). Note any endpoints the UI calls that do not exist server-side, or vice versa.'],
  ['X4-security-hostile-input', 'Cross-cutting: security & hostile input (ARCH §3.5). Verify beacon ingest: token auth constant-time + hashed at rest, per-token rate limit, body size cap, strict schema validation, never echoes tokens, CORS sane; GAP-2-004 (beacon ingest NOT tier-gated on Free — fail-open, a real finding); secrets at rest (AES-GCM, never plaintext, never committed — check web/pulse_secret.key is gitignored); password hashing (bcrypt); /metrics + API token handling. Report exploitable gaps with severity.'],
]

const found = await parallel(AREAS.map(([area, focus]) =>
  () => agent(verifier(area, focus), { label: `verify:${area}`, phase: 'Verify', schema: FINDINGS_SCHEMA })
))

const ok = found.filter(Boolean)
const allFindings = ok.flatMap(r => (r.findings || []).map(f => ({ ...f, area: r.area })))
log(`Verify done: ${ok.length}/${AREAS.length} verifiers returned; ${allFindings.length} raw findings (verdicts: ${ok.map(r => r.area + '=' + r.verdict).join(', ')}).`)

// ---------- Phase 2: Triage (barrier is correct — needs ALL findings to dedup + rank) ----------

phase('Triage')
const triage = await agent(
  `You are the validation triage lead (verify-only, no code edits, no commits) for the Pulse MVP. ${ok.length} adversarial verifiers just finished. Read their finding files agents/handoffs/validation/findings-*.md and consolidate.

Here is the raw findings JSON from all verifiers (also in the files):
${JSON.stringify(allFindings, null, 1).slice(0, 60000)}

Tasks: (1) DEDUP findings that describe the same root issue across areas (e.g., the tier-model gap will appear under F5/F6/F8 and X2 — merge into one). (2) DROP low-confidence/noise and anything that is an accepted waiver (D-002 no-Docker, D-007.5 no-Kafka) or an already-known Phase-3 backlog item explicitly out of MVP scope (note these separately, do not list as defects). (3) Assign each surviving finding a stable id (VD-01…), an owner (BE-01/BE-02/FE-01/SDK-01/INFRA-01/INT-01/QA-01), a severity, and a concise concrete fix. (4) Mark mvpBlocking=true for anything that breaks a PRD acceptance criterion or a numeric budget or a documented feature flow; false for polish/Phase-3. (5) Rank for a V3 fix-loop.

Write the consolidated report to agents/handoffs/validation/V2-triage-report.md (table: id | area | severity | mvpBlocking | owner | finding | fix | evidence; plus a "deferred/out-of-scope" section and a "budgets re-checked" summary). Return ONLY the StructuredOutput object.`,
  { label: 'triage: consolidate findings', phase: 'Triage', schema: TRIAGE_SCHEMA })

log(`Triage: ${triage ? triage.totalFindings : '?'} findings; confirmed=${triage ? (triage.confirmed || []).length : '?'}.`)

return {
  verifiers: ok.map(r => ({ area: r.area, verdict: r.verdict, findings: (r.findings || []).length, report: r.reportPath || ('findings-' + r.area + '.md') })),
  triage: triage ? { total: triage.totalFindings, confirmed: triage.confirmed || [], bySeverity: triage.bySeverity || {}, report: triage.reportPath, summary: triage.summary } : null,
}
