# VAL-tenant INT-01 report — D-010 /admin/tenants contract amendment

**Agent:** INT-01  
**Date:** 2026-06-14  
**Status:** COMPLETE  

---

## Paths added

| Method | Path | OperationId | Response |
|--------|------|-------------|----------|
| GET | `/admin/tenants` | `listTenants` | `TenantList` (200) |
| POST | `/admin/tenants` | `createTenant` | `Tenant` (201) |
| GET | `/admin/tenants/{tenantId}` | `getTenant` | `Tenant` (200) |
| PUT | `/admin/tenants/{tenantId}` | `updateTenant` | `Tenant` (200) |
| DELETE | `/admin/tenants/{tenantId}` | `deleteTenant` | 204 |

All five paths use `tags: [admin]` and the existing error envelope (`$ref: "#/components/responses/..."`) per existing conventions.

---

## Parameter added

| Name | Location | Description |
|------|----------|-------------|
| `tenantId` | `components/parameters` (path) | Tenant UUID — `$ref`-ed by both `/admin/tenants/{tenantId}` paths |

---

## Schemas added

| Schema name | Description |
|-------------|-------------|
| `Tenant` | Read shape: `{id, name, stream_pattern?, meta_tag_key?, meta_tag_value?, created_at, updated_at}` — aligned exactly to `contracts/db/meta/0001_init.sql` `tenants` table |
| `TenantWrite` | Write shape: `{name (required), stream_pattern?, meta_tag_key?, meta_tag_value?}` |
| `TenantList` | Paginated list: `{items: [Tenant], meta: PaginatedMeta}` |

### Schema alignment to SQL DDL

| OpenAPI field | SQL column | Type match |
|---------------|------------|------------|
| `id` (string, required) | `id TEXT NOT NULL PRIMARY KEY` | TEXT/UUID |
| `name` (string, required) | `name TEXT NOT NULL UNIQUE` | TEXT |
| `stream_pattern` (string, nullable, optional) | `stream_pattern TEXT` | TEXT nullable |
| `meta_tag_key` (string, nullable, optional) | `meta_tag_key TEXT` | TEXT nullable |
| `meta_tag_value` (string, nullable, optional) | `meta_tag_value TEXT` | TEXT nullable |
| `created_at` (integer, required) | `created_at INTEGER NOT NULL` | Unix epoch ms |
| `updated_at` (integer, required) | `updated_at INTEGER NOT NULL` | Unix epoch ms |

`TenantWrite` omits `id`, `created_at`, `updated_at` (server-assigned) — correct.

---

## Shared component additions

Two new shared responses added to `components/responses` (used by tenant paths, safe reuse by future paths):

- `Conflict` — 409, duplicate name error
- `Forbidden` — 403, tier gate

---

## Lint result

Command:
```
npx --yes @redocly/cli lint --skip-rule=path-parameters-defined contracts/openapi/pulse-api.yaml
```

Result:
```
validating contracts/openapi/pulse-api.yaml...
contracts/openapi/pulse-api.yaml: validated in 48ms
Woohoo! Your API description is valid.
```

**0 errors, 0 warnings.**

---

## kin-openapi load verification

Command:
```
cd server && go test ./internal/api/... -v
```

Result: **PASS** — all 30+ API conformance tests pass (0.468s). Tests use `getkin/kin-openapi v0.140.0` to load the spec at test setup; a load failure would prevent all tests from running.

---

## Notes for BE-02 / FE-01

- **OperationIds** for codegen: `listTenants`, `createTenant`, `getTenant`, `updateTenant`, `deleteTenant`
- **Schema names** for codegen: `Tenant`, `TenantWrite`, `TenantList`
- BE-02: gate all five handlers to Business tier (D-010); return 409 on duplicate name (meta UNIQUE constraint); validate at least one matcher field is set (→ 422)
- FE-01: regenerate TS types (`npm run gen:api`) before building the tenant CRUD surface

---

## Files modified

- `contracts/openapi/pulse-api.yaml` — added paths, parameter, schemas, shared responses, updated admin tag description
