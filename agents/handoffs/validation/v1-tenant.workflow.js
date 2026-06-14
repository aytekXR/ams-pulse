export const meta = {
  name: 'pulse-val-1-tenant-crud',
  description: 'Validation V1: close the deferred F6 /admin/tenants CRUD (D-010 CR) — contract, server, UI, verify',
  phases: [
    { title: 'Contract', detail: 'INT-01 amends OpenAPI with /admin/tenants CRUD (approved CR D-010)' },
    { title: 'Implement', detail: 'parallel: BE-02 routes | FE-01 UI' },
    { title: 'Verify', detail: 'QA-01 tenant round-trip + per-tenant billing +/-1% + regression' },
  ],
}

const COMMIT = `
## Commit protocol (D-008 — binding)
1. VERIFY FIRST: commit only when your acceptance passes. Partial work is reported, not committed.
2. Stage by EXPLICIT path inside your scope only — NEVER 'git add -A', '-u', or '.' (D-011: an agent already broke this and swallowed another's files — list each path).
3. Message '<YOUR-AGENT-ID> VAL-tenant: <summary>' + verification evidence (commands + measured numbers). No push.
4. .git/index.lock busy ⇒ wait+retry (bounded); never delete the lock.
5. Report your commit SHA in your structured output.
`

const ENV = `
## Environment
- Working dir /Users/ae/repo/ant-marketplace, branch main (no new branches). macOS arm64; Go 1.26.4 CGO_ENABLED=0; Node v26. NO Docker (D-002). ClickHouse at /tmp/clickhouse (v26.6.1). web/pulse_secret.key gitignored — never commit.
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
    changeRequests: { type: 'array', items: { type: 'string' } },
    summary: { type: 'string' },
  },
}

// ---------- Phase 1: Contract (INT-01 — the ONLY agent allowed to touch the freeze, for this approved CR) ----------

phase('Contract')
log('Validation V1: close deferred F6 /admin/tenants CRUD (D-010). INT-01 contract -> BE-02 routes | FE-01 UI -> QA verify.')

const intRes = await agent(
  `You are INT-01, the integration/contracts agent. ORCH-00 dispatched you to apply an APPROVED contract change (decision D-010): add the F6 /admin/tenants CRUD surface that the D-004 freeze omitted. You ARE authorized to edit contracts/ for THIS amendment (you are the CR-applier channel, as in D-006 CR-1..4) — but ONLY the tenant additions below; touch nothing else under contracts/.

Read first: CLAUDE.md, your charter agents/definitions/INT-01-integration.md, the work order agents/handoffs/validation/wo-tenant-crud.md (the exact contract shape), decisions.md D-004 + D-010, and contracts/db/meta/0001_init.sql (the 'tenants' table — your schema MUST align to it).

Apply to contracts/openapi/pulse-api.yaml: GET/POST /admin/tenants, GET/PUT/DELETE /admin/tenants/{tenantId}, a tenantId path parameter, and schemas Tenant {id, name, stream_pattern?, meta_tag_key?, meta_tag_value?, created_at, updated_at}, TenantWrite {name (required), stream_pattern?, meta_tag_key?, meta_tag_value?}, TenantList {items:[Tenant], meta: PaginatedMeta}. Reuse the existing error envelope + tags conventions already in the file. Place them consistently with the existing admin paths/schemas.

Acceptance: redocly lint (or the repo's contract lint, e.g. 'npm run lint' in contracts/ or 'redocly lint') = 0 errors; kin-openapi still loads the spec. Report the operationIds + schema names you added so BE-02/FE-01 can codegen against them.
${ENV}${COMMIT}
Write agents/handoffs/validation/VAL-tenant-INT-report.md (paths/schemas/operationIds added, lint result). Return ONLY the StructuredOutput object.`,
  { label: 'INT-01: tenant CRUD contract', phase: 'Contract', schema: AGENT_SCHEMA })

log(`INT-01 contract: status=${intRes ? intRes.status : 'null'}, committed=${intRes ? intRes.committed : '?'}`)

// ---------- Phase 2: Implement (BE-02 routes || FE-01 UI; disjoint trees, both depend on INT-01) ----------

phase('Implement')
const impl = await parallel([
  () => agent(
    `You are BE-02, backend product-plane. ORCH-00 dispatched you to implement the F6 /admin/tenants CRUD routes now that INT-01 has amended the contract (D-010). Read first: CLAUDE.md, your charter agents/definitions/BE-02-backend-productplane.md, the work order agents/handoffs/validation/wo-tenant-crud.md, INT-01's agents/handoffs/validation/VAL-tenant-INT-report.md (the new operationIds/schemas), contracts/db/meta/0001_init.sql (tenants table), and the existing server/internal/reports/tenant.go matcher + server/internal/api route registration + server/internal/store/meta for patterns.

Implement the 5 handlers in internal/api over the meta tenants table (internal/store/meta), reusing tenant.go for validation. Business-tier-gated (§7.11 / D-010). Unique name → 409; unknown id → 404; require at least one of stream_pattern OR (meta_tag_key+meta_tag_value). Declare any cmd/pulse edits (D-005).

Acceptance: CGO_ENABLED=0 go build ./... && go vet ./... && go test ./... green; kin-openapi conformance extended to the 5 tenant ops; tests for CRUD happy paths, 409 duplicate, 404 unknown, tier gate (Free/Pro → 403), and that a created tenant's stream_pattern resolves a stream in usage accounting. Do NOT edit contracts/ (INT-01 did). Stay in BE-02 scope.
${ENV}${COMMIT}
Write agents/handoffs/validation/VAL-tenant-BE-report.md. Return ONLY the StructuredOutput object.`,
    { label: 'BE-02: tenant CRUD routes', phase: 'Implement', schema: AGENT_SCHEMA }),

  () => agent(
    `You are FE-01, frontend. ORCH-00 dispatched you to build the F6 tenant CRUD UI deferred in WO-205, now that INT-01 amended the contract (D-010). Read first: CLAUDE.md, your charter agents/definitions/FE-01-frontend.md, the work order agents/handoffs/validation/wo-tenant-crud.md, your wave-2 WO-205-report.md (where the tenant surface was stubbed), and the existing web/src/features/reports structure.

Regenerate API types FIRST (npm run generate:api) — GENERATED types ONLY (git-grep proof). Build: tenant list + create/edit form (name, stream_pattern, meta_tag_key/value) + delete confirm, ideally with a live preview of matched streams; Business-tier-gated with upsell (never a broken page); loading/error/empty states. Integrate into the existing reports/tenants area.

Acceptance: npm run build && npm run lint && npm run test green; component tests (form validation, tier-gated view, CRUD rendering); a screenshot against seeded fixtures in the report. Stay in web/ scope. Do NOT edit contracts/ or server/.
${ENV}${COMMIT}
Write agents/handoffs/validation/VAL-tenant-FE-report.md. Return ONLY the StructuredOutput object.`,
    { label: 'FE-01: tenant CRUD UI', phase: 'Implement', schema: AGENT_SCHEMA }),
])

const [beRes, feRes] = impl
log(`Implement: BE-02=${beRes ? beRes.status : 'null'}, FE-01=${feRes ? feRes.status : 'null'}`)

// ---------- Phase 3: Verify (QA-01 — barrier; verifies the integrated tree) ----------

phase('Verify')
const qaRes = await agent(
  `You are QA-01, QA & verification. ORCH-00 dispatched you to verify the newly-landed F6 /admin/tenants CRUD (D-010) end-to-end. VERIFY, NEVER FIX product code. Read first: CLAUDE.md, your charter agents/definitions/QA-01-qa.md, the work order agents/handoffs/validation/wo-tenant-crud.md, and the three reports VAL-tenant-{INT,BE,FE}-report.md.

REBUILD binaries FIRST (D-013 lesson — never test a stale binary): CGO_ENABLED=0 go build -o /tmp/pulse ./cmd/pulse/ and rebuild qa/mock-ams if your scenario needs it. Then on the live stack (/tmp/clickhouse + pulse + Business-tier dev license): create two tenants by stream_pattern via POST /admin/tenants; seed streams that match (and one that does not); generate a statement; assert per-tenant figures reconcile within ±1% of known truth and the unmatched stream falls to 'unassigned'. Exercise the full CRUD (create/list/get/update/delete, 409 duplicate, 404 unknown, Free/Pro tier 403). Then full regression: CGO_ENABLED=0 go test ./... + web build/lint/test; confirm the wave-1/2/3 gate scripts still pass.

Waivers limited to D-002/D-007.5. Write agents/handoffs/validation/VAL-tenant-QA-report.md with PASS/FAIL per check + measured numbers + repro + any defects (owner/severity/repro).
${ENV}${COMMIT}
Return ONLY the StructuredOutput object (status reflects PASS/PARTIAL/BLOCKED; put a clear verdict + any defects in summary/gaps).`,
  { label: 'QA-01: tenant round-trip verify', phase: 'Verify', schema: AGENT_SCHEMA })

log(`QA-01 verify: status=${qaRes ? qaRes.status : 'null'}`)

return {
  contract: intRes ? { status: intRes.status, committed: intRes.committed, sha: intRes.commitSha || '', summary: intRes.summary } : null,
  be: beRes ? { status: beRes.status, committed: beRes.committed, sha: beRes.commitSha || '', crs: beRes.changeRequests || [], summary: beRes.summary } : null,
  fe: feRes ? { status: feRes.status, committed: feRes.committed, sha: feRes.commitSha || '', crs: feRes.changeRequests || [], summary: feRes.summary } : null,
  qa: qaRes ? { status: qaRes.status, committed: qaRes.committed, gaps: qaRes.gaps || [], summary: qaRes.summary } : null,
}
