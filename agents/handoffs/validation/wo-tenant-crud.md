# VAL-TENANT — close the deferred F6 `/admin/tenants` CRUD (D-010 approved CR)

**Issued by:** ORCH-00 · 2026-06-14 · validation phase, step V1
**Approved CR:** D-010 (decisions.md). F6 required "tenants … CRUD per OpenAPI";
the D-004 freeze omitted the paths. The `tenants` meta table, `tenant` query
param, and `tenant.go` matcher all exist — only the management paths/UI are missing.

## The contract amendment (INT-01)

Add to `contracts/openapi/pulse-api.yaml` (this is an ORCH-00-approved amendment to
the freeze, applied by INT-01 — the same channel used for CR-1..4 in D-006):

- `GET /admin/tenants` → `TenantList` (paginated): list tenants.
- `POST /admin/tenants` (body `TenantWrite`) → `Tenant` (201): create.
- `GET /admin/tenants/{tenantId}` → `Tenant`.
- `PUT /admin/tenants/{tenantId}` (body `TenantWrite`) → `Tenant`.
- `DELETE /admin/tenants/{tenantId}` → 204.
- `parameters: tenantId` (path).
- Schemas, aligned EXACTLY to the meta `tenants` table
  (`contracts/db/meta/0001_init.sql`): `Tenant {id, name, stream_pattern?,
  meta_tag_key?, meta_tag_value?, created_at, updated_at}`; `TenantWrite
  {name (required), stream_pattern?, meta_tag_key?, meta_tag_value?}`;
  `TenantList {items: [Tenant], meta: PaginatedMeta}`. Reuse the standard error
  envelope. Keep redocly/kin-openapi lint at 0 issues.

## BE-02 — routes

Implement the five handlers in `internal/api` over the meta `tenants` table
(`internal/store/meta`), reusing the existing `tenant.go` matcher for validation.
Gate to **Business** tier (per §7.11 + D-010 — multi-tenant billing is Business).
Name is unique (meta constraint) → 409 on duplicate; 404 on unknown id; validate
that at least one of stream_pattern / (meta_tag_key+meta_tag_value) is set. Wire
into `cmd/pulse` only if new construction is needed (declare per D-005). Tests:
CRUD happy paths, duplicate-name 409, unknown-id 404, tier gate (Free/Pro → 403),
and that a created tenant's `stream_pattern` actually resolves a stream in usage
accounting (ties to the F6 reconcile path).

## FE-01 — UI

Build the tenant CRUD surface deferred in WO-205 (`web/`): list + create/edit form
(name, stream_pattern, meta_tag_key/value) + delete confirm, with a live preview of
matched streams where cheap; consume GENERATED types only (regenerate first).
Business-tier-gated with upsell, never a broken page. Component tests + a screenshot.

## QA-01 — mini-verify

Tenant round-trip on the live stack: create two tenants by `stream_pattern` via the
API → seed streams that match → generate a statement → assert per-tenant figures
reconcile within ±1% of known truth and unmatched streams fall to "unassigned".
Plus full build/lint/test regression (server/web). Append results to
`agents/handoffs/validation/VAL-tenant-report.md` (or a gate section). Waivers
D-002/D-007.5 only.

## Commit (D-008)

Each agent verifies then self-commits its own scope by EXPLICIT path (never
`git add -A`/`-u`/`.` — D-011), message `<AGENT-ID> VAL-tenant: <summary>` +
evidence, no push. INT-01 commits the contract amendment; BE-02 the server;
FE-01 the web; QA-01 its qa artifacts.

## Reports

`agents/handoffs/validation/VAL-tenant-{INT,BE,FE,QA}-report.md`.
