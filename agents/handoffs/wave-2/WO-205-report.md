# WO-205 Completion Report — Wave 2 Frontend (FE-01)

**Agent:** FE-01
**Date:** 2026-06-14
**Work order:** WO-205 (issued by ORCH-00 2026-06-12)

---

## Status: DONE

All acceptance criteria verified by running the actual commands. Files committed in
commit `2d2910f` (note: SDK-01's commit also staged web/ files; tracked by diff below).

---

## Acceptance criteria — verified outputs

### 1. `npm run generate:api` — regenerate first

```
$ cd web && npm run generate:api

> @pulse/web@0.1.0 generate:api
> npm run gen:api

> @pulse/web@0.1.0 gen:api
> openapi-typescript ../contracts/openapi/pulse-api.yaml -o src/lib/api/schema.d.ts

✨ openapi-typescript 7.13.0
🚀 ../contracts/openapi/pulse-api.yaml → src/lib/api/schema.d.ts [56.3ms]
```

Result: **PASS** — schema.d.ts current with frozen spec.

### 2. `npm run build && npm run lint && npm run test` — all green

```
$ cd web && npm run build && npm run lint && npm run test

> @pulse/web@0.1.0 build
> tsc -b && vite build

vite v6.4.3 building for production...
✓ 641 modules transformed.
dist/index.html                   0.41 kB │ gzip:   0.29 kB
dist/assets/index-DjajK3KD.css    1.29 kB │ gzip:   0.66 kB
dist/assets/index-SpSrCHPQ.js   743.89 kB │ gzip: 215.33 kB
✓ built in 1.02s

> @pulse/web@0.1.0 lint
> eslint src
(0 errors, 0 warnings)

> @pulse/web@0.1.0 test
> vitest run

 ✓ src/features/live/__tests__/LiveSocket.test.ts (8 tests) 9ms
 ✓ src/features/live/__tests__/StreamsTable.test.tsx (7 tests) 202ms
 ✓ src/components/__tests__/AuthGate.test.tsx (5 tests) 223ms
 ✓ src/features/alerts/__tests__/AlertRuleForm.test.tsx (6 tests) 289ms
 ✓ src/features/reports/__tests__/ReportsPage.test.tsx (11 tests) 281ms
 ✓ src/features/qoe/__tests__/QoePage.test.tsx (11 tests) 179ms
 ✓ src/features/fleet/__tests__/FleetPage.test.tsx (10 tests) 336ms

 Test Files  7 passed (7)
      Tests  58 passed (58)
   Duration  1.51s
```

Result: **ALL GREEN** — build, lint, 58/58 tests.

### 3. No hand-rolled API shapes — git-grep proof

```
$ cd web && git grep -rn "interface.*Response\|interface.*Request" -- "src/" \
    | grep -v "schema.d.ts\|types.ts"
(no output — exit code 1 = no matches)
```

All API-typed values flow through `src/lib/api/types.ts` (re-exports from
the generated `schema.d.ts`). New types added:
- `QoeSummaryResponse`, `QoeTotals`, `BitrateBucket` (F3)
- `IngestHealthResponse`, `IngestStream`, `IngestBucket`, `DropEvent` (F4)
- `FleetNode`, `FleetNodeList` (F7)
- `UsageReportResponse`, `UsageRow`, `UsageTotals`, `ReportSchedule`,
  `ReportScheduleList`, `ReportScheduleWrite` (F6)

### 4. Component tests — required by WO-205

New test files (37 new tests, all pass):

| File | Tests | Coverage |
|---|---|---|
| `src/features/qoe/__tests__/QoePage.test.tsx` | 11 | Slice-state reducer (5 cases) + QoePage loading/empty/error/data states |
| `src/features/reports/__tests__/ReportsPage.test.tsx` | 11 | Schedule form validation (6 cases) + tier gate (5 cases: free→upsell, pro→usage, enterprise→schedules, loading) |
| `src/features/fleet/__tests__/FleetPage.test.tsx` | 10 | Node-state rendering (cards, table, empty, error) + health color logic |
| `src/components/__tests__/AuthGate.test.tsx` | 5 | 401 redirect: clearToken called, session-expired message shown |

Pre-existing tests (21 tests): all still pass.

---

## Surfaces built

### F3 — Viewer QoE (`/qoe`)

**File:** `web/src/features/qoe/QoePage.tsx`

Components:
- Summary cards: startup p50/p95, rebuffer ratio (yellow >5%), error rate (red >1%)
- Bitrate timeline: recharts LineChart with p50 (blue) + p95 (yellow dashed)
- Slice controls: stream ID filter, app filter, date range picker
- Honest empty state: "No QoE data yet" with link to SDK Setup Docs
- Loading/error/empty states on all code paths

```
┌─ Viewer QoE ──────────────────────────────────────────────────────────────┐
│  [24h] [7d] [30d] [Custom]    [Stream filter]  [App filter]              │
│                                                                            │
│  ┌─ Startup p50 ─┐ ┌─ Startup p95 ─┐ ┌─ Rebuffer % ─┐ ┌─ Error % ─┐   │
│  │     350 ms    │ │    1200 ms    │ │     2.0%     │ │   0.10%  │   │
│  └───────────────┘ └───────────────┘ └──────────────┘ └──────────┘   │
│                                                                            │
│  ┌─ Bitrate Timeline (Kbps) ────────────────────────────────────────────┐ │
│  │  ── p50   ---- p95                                                   │ │
│  │  [recharts LineChart: dual-line bitrate over time]                  │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────────────┘
```

### F4 — Ingest Health (`/ingest`)

**File:** `web/src/features/ingest/IngestPage.tsx`

Components:
- Per-publisher list: stream_id, app, node, health progress bar + label badge
- Drop events count (red if present)
- Detail panel (collapsible): bitrate+FPS timeline, packet loss + jitter timelines
- Drop event markers as ReferenceLine on bitrate chart
- Threshold indicator: 1% packet loss ReferenceLine
- Auto-refresh every 15 s (degradation budget ≤15 s)
- Loading/error/empty states

```
┌─ Ingest Health ──────────── ● Live (15s) ──────────── [Refresh] ───────┐
│  Stream         App    Node   Health                   Drops            │
│  live/stream-1  live   n1    [████████░░] Healthy      —               │
│  live/stream-2  live   n2    [████░░░░░░] Degraded    2 drops  [Details]│
│                                                                          │
│  ┌─ live/stream-2 detail ─────────────────────────────────────────────┐ │
│  │  Drop Events (2)  [12:03:10 — bitrate_drop]  [12:04:22 — fps_drop]│ │
│  │  [Bitrate & FPS chart with drop markers]                           │ │
│  │  [Packet Loss % + Jitter ms chart with 1% threshold line]          │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
```

### F6 — Usage Reports (`/reports`)

**File:** `web/src/features/reports/ReportsPage.tsx`

Components:
- Tier gate: `TierUpsell` renders when `license.tier === "free"` — never a broken
  page; shows upgrade prompt and link to `/settings#license`
- Usage tab: date range picker, Export CSV + PDF buttons, egress method notice
  (always shown per spec), totals grid, per-row table (app/stream/tenant/viewer-min/
  peak/egress/recording)
- Schedules tab: CRUD with cron presets (monthly/weekly/daily + custom field),
  format (CSV/PDF), app/tenant scope
- ScheduleForm validation: required cron, 5-field validation

Tier gate layout:
```
┌─ Reports ─────────────────────────────────────────────────────────────────┐
│                                                              [free]       │
│                                                                           │
│  ┌─ Usage Reports requires Business tier ──────────────────────────────┐ │
│  │  [icon]                                                              │ │
│  │  You are currently on the free plan. Upgrade to Business to         │ │
│  │  unlock usage reports, scheduled exports, and tenant mapping.       │ │
│  │                                    [Upgrade License]                │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────────────┘
```

Entitled layout:
```
│  [usage] [schedules]                                                      │
│  [24h][7d][30d][Custom]           [Export CSV] [Export PDF]              │
│  Egress method: bitrate_x_watch_time                                      │
│  ┌── Viewer-Min ──┐ ┌── Peak ──┐ ┌── Egress GB ──┐ ┌── Recording GB ──┐ │
│  │    24,150      │ │  1,234   │ │     88.40      │ │     12.30        │ │
│  ╞═════════════════════════════════════════════════════════════════════╡ │
│  │ App   Stream   Tenant  Viewer-Min  Peak  Egress GB*  Recording GB  │ │
│  │ live  stream-1  —       12,075     892     44.20       6.15        │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
```

### F7 — Fleet View (`/fleet`)

**File:** `web/src/features/fleet/FleetPage.tsx`

Components:
- Cards view (default): NodeCard with role badge (origin=info/edge=muted),
  status badge (up=green/degraded=yellow/down=red), last_seen, version,
  CPU/memory load bars (color-coded at 60%/80% thresholds), network Mbps
- Table view: all columns in compact form, toggle cards/table
- Aggregate header: total, up, degraded, down, origins, edges counts
- Auto-refresh every 30 s (node discovery ≤2 min)
- Dedup note in prose
- Loading/empty/error states

```
┌─ Fleet ──────────────────── ● Auto-refresh (30s) ─── [Refresh] ─── [cards|table] ─┐
│  Total: 3  Up: 2  Degraded: 1  Down: 0  Origins: 1  Edges: 2                       │
│                                                                                       │
│  ┌─ node-origin-1 ─── [origin] [up] ─┐ ┌─ node-edge-1 ─── [edge] [degraded] ─┐    │
│  │ v2.9.1 · 5s ago                   │ │ v2.9.0 · 30s ago                      │    │
│  │ CPU  [████████░░░] 45%            │ │ CPU  [████████████████░░] 85%          │    │
│  │ Mem  [████████████░] 62%          │ │ Mem  [████████████████████] 91%        │    │
│  │ ↓12.5 Mbps ↑88.3 Mbps            │ │                                        │    │
│  └────────────────────────────────────┘ └────────────────────────────────────────┘    │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### Settings additions

**File:** `web/src/features/settings/SettingsPage.tsx`

New tabs added:

**Ingest Tokens tab:**
- Lists ingest-kind tokens only (filtered from `/admin/tokens`)
- Create ingest token → shows raw value once with copy-paste SDK snippet:
  ```js
  Pulse.init({
    token: 'plt_xyz...',
    endpoint: window.location.origin + '/ingest/beacon',
  });
  ```
- Revoke button on each token
- Ingest endpoint URL shown at bottom

**Integrations tab:**
- Prometheus info panel: scrape URL (`/metrics`), example `scrape_configs` YAML block
- S3 export form: bucket, region, access key env-ref name, secret key env-ref name
  (never raw credential values; env-ref names only per security rule)
  Note: S3 server-side storage is wave-3 server implementation; UI saves config with
  a toast noting server-side TBD.

### 401 redirect (carried fix from wave-1)

**Files:** `web/src/api/client.ts`, `web/src/components/AuthGate.tsx`

Implementation:
1. `apiFetch()` in `client.ts` dispatches `window.dispatchEvent(new Event("pulse:auth:401"))`
   on HTTP 401 response.
2. `AuthGate` installs an event listener on `"pulse:auth:401"` which calls `clearToken()`
   and resets state to show the login form with message "Session expired or token
   revoked. Please enter your token again."
3. No full page reload needed; React Router navigation is preserved.

Test coverage: `src/components/__tests__/AuthGate.test.tsx` — 5 tests covering:
- Authenticated → shows children
- Unauthenticated → shows login form
- Empty token submit → validation error
- Successful login → setToken called
- `pulse:auth:401` event → clearToken called, session-expired message shown

---

## Library decisions

No new libraries added. All wave-2 UI built with:
- recharts (already installed) — bitrate timelines, QoE chart
- @tanstack/react-virtual (already installed) — not needed for new surfaces (lists are
  shorter than stream table)
- react-router-dom (already installed) — routing unchanged
- openapi-typescript (already installed) — type generation

---

## Files changed

All changes within `web/` (FE-01 scope).

### New files
- `web/src/features/qoe/QoePage.tsx` — F3 viewer QoE dashboard
- `web/src/features/qoe/__tests__/QoePage.test.tsx` — 11 tests
- `web/src/features/ingest/IngestPage.tsx` — F4 ingest health
- `web/src/features/reports/ReportsPage.tsx` — F6 usage reports + schedules
- `web/src/features/reports/__tests__/ReportsPage.test.tsx` — 11 tests
- `web/src/features/fleet/FleetPage.tsx` — F7 fleet view
- `web/src/features/fleet/__tests__/FleetPage.test.tsx` — 10 tests
- `web/src/components/__tests__/AuthGate.test.tsx` — 5 tests

### Modified files
- `web/src/App.tsx` — replaced ComingSoon placeholders with real page components
- `web/src/api/client.ts` — added qoeApi, fleetApi, reportsApi; 401→event dispatch
- `web/src/lib/api/types.ts` — added F3/F4/F6/F7 type aliases from generated schema
- `web/src/components/AuthGate.tsx` — wave-2 401 redirect via custom event
- `web/src/components/Layout.tsx` — removed W2 badges from now-implemented routes
- `web/src/features/settings/SettingsPage.tsx` — ingest tokens + integrations tabs
- `web/eslint.config.js` — added `navigator` to browser globals

---

## Numeric budget verification

| Budget | Status | Evidence |
|---|---|---|
| Dashboard < 2s at 500 streams | PASS (Wave 1 verified) | StreamsTable virtualization, ≤20 DOM rows for 500-row input |
| New stream visible ≤10 s | PASS (Wave 1 verified) | REST poller 5s + aggregator |
| Ingest degradation visible ≤15 s | IN SPEC | IngestPage auto-refreshes every 15 s |
| Fleet node discovery ≤2 min | IN SPEC | FleetPage auto-refreshes every 30 s; discovery is server-side |
| SDK < 15 KB gzip | N/A (SDK-01 owns) | SDK-01 report: 3.44 KB gzip |

---

## Gaps / change requests

### Contract change requests

None — all F3/F4/F6/F7 endpoints were already in the frozen spec (`/qoe/summary`,
`/qoe/ingest`, `/fleet/nodes`, `/reports/usage`, `/reports/schedules`). No contract
changes needed.

### Carried-forward gaps

1. **Analytics page geo/device CSV export** — the CSV export button on AnalyticsPage
   hits `/api/v1/reports/export?format=csv` which is a generic endpoint, not
   `/analytics/geo` or `/analytics/devices`. The server-side export needs to be
   implemented by BE-02 to include geo/device breakdown. Current behavior:
   exports the full audience/usage report, not the geo or device slice.

2. **S3 export server-side storage** — the S3 form in Settings/Integrations collects
   bucket + region + env-ref names (wave-2 UI complete). The server-side config
   persistence and actual S3 upload are wave-3 BE-02 work.

3. **Tenant CRUD (pattern/tag mapping)** — WO-205 item 5 mentions "tenants CRUD
   (pattern/tag mapping with live preview of matched streams)". The `/admin/tenants`
   endpoint is NOT in the frozen OpenAPI spec (`contracts/openapi/pulse-api.yaml`).
   A CR is filed below.

### CR-1: Tenant mapping CRUD endpoint missing from spec

**Description:** The reports feature (F6) requires tenant mapping configuration:
pattern/tag rules that map stream names to tenant identifiers. The current spec
has `tenant_mapping?: string` on `ReportSchedule` (a reference string) but no
CRUD endpoint for managing tenant mapping rules themselves.

**Suggested addition:**
- `GET /admin/tenants` → list tenant mapping rules
- `POST /admin/tenants` → create rule (pattern, tag, tenant_id)
- `PUT /admin/tenants/{tenantId}` → update rule
- `DELETE /admin/tenants/{tenantId}` → delete rule

Until this CR is approved and the spec amended (D-004 freeze), the tenant
CRUD surface is deferred. The reports table already renders `tenant` values
from the server response when present.

### CR-2: White-label PDF header config endpoint

**Description:** WO-205 item 5 mentions "white-label header config". The
`ReportSchedule` schema has `whitelabel_header?: object` but there is no
dedicated endpoint to set the white-label config globally (for all reports,
not just per-schedule). Suggest adding `GET/PUT /admin/whitelabel` endpoint
for the global brand config (logo URL, company name, color). Deferred to
wave-3 per WO-205 "Phase 3" annotation.
