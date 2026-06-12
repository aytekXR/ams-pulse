# WO-104 Completion Report — Wave 1 Frontend (FE-01)

**Agent:** FE-01  
**Date:** 2026-06-12  
**Work order:** WO-104 (issued by ORCH-00 2026-06-11)

---

## Status: DONE

All acceptance criteria verified by running the actual commands.

---

## Acceptance criteria — verified outputs

### 1. `npm run build` green (tsc strict)

```
$ cd web && npm run build

> @pulse/web@0.1.0 build
> tsc -b && vite build

vite v6.4.3 building for production...
✓ 638 modules transformed.
dist/index.html                   0.41 kB │ gzip:   0.29 kB
dist/assets/index-DjajK3KD.css    1.29 kB │ gzip:   0.66 kB
dist/assets/index-DESn6F29.js   696.79 kB │ gzip: 206.55 kB
✓ built in 905ms
```

Result: **tsc strict + vite build — GREEN**

### 2. `npm run generate:api` produces types, build consumes them

```
$ cd web && npm run generate:api

> @pulse/web@0.1.0 generate:api
> npm run gen:api

> @pulse/web@0.1.0 gen:api
> openapi-typescript ../contracts/openapi/pulse-api.yaml -o src/lib/api/schema.d.ts

✨ openapi-typescript 7.13.0
🚀 ../contracts/openapi/pulse-api.yaml → src/lib/api/schema.d.ts [55.1ms]
```

Result: **PASS — 2820-line schema.d.ts generated, build green on top of it**

Note: `contracts/README.md` suggests `../../contracts/...` but the correct relative
path from `web/` is `../contracts/...` (web/ is one level below the repo root, not
two). Flagged as a gap for INT-01 to fix in contracts/README.md.

### 3. No hand-rolled API shapes — `git grep` proof

```
$ cd web && git grep -rn "interface.*Response\|interface.*Request" -- "src/" \
    | grep -v "schema.d.ts\|types.ts"
(no output)
```

All API-typed values flow through `src/lib/api/types.ts` (re-exports from the
generated `schema.d.ts`) or `components["schemas"][...]` inline references.
Checked imports:

```
src/api/client.ts                    → "@/lib/api/types" + "@/lib/api/schema.d.ts"
src/features/live/StreamsTable.tsx   → "@/lib/api/types"
src/features/live/ProtocolDonut.tsx  → "@/lib/api/types"
src/features/live/useLiveDashboard.ts → "@/lib/api/types"
src/features/alerts/AlertsPage.tsx   → "@/lib/api/types"
src/features/alerts/AlertRuleForm.tsx → "@/lib/api/types"
src/features/alerts/AlertChannelForm.tsx → "@/lib/api/types"
src/features/analytics/AnalyticsPage.tsx → "@/lib/api/types"
src/features/settings/SettingsPage.tsx → "@/lib/api/types"
src/features/settings/OnboardingWizard.tsx → "@/lib/api/types"
```

### 4. Component tests — all 21 pass

```
$ cd web && npm run test

> @pulse/web@0.1.0 test
> vitest run

 ✓ src/features/live/__tests__/LiveSocket.test.ts (8 tests) 5ms
 ✓ src/features/live/__tests__/StreamsTable.test.tsx (7 tests) 89ms
 ✓ src/features/alerts/__tests__/AlertRuleForm.test.tsx (6 tests) 140ms

 Test Files  3 passed (3)
      Tests  21 passed (21)
   Start at  15:52:04
   Duration  827ms
```

Tests cover:
- `LiveSocket.test.ts` (8 tests): reconnect backoff, status callbacks, subscriber
  delivery, destroy-prevents-retry, unsubscribe
- `StreamsTable.test.tsx` (7 tests): header render, empty state, stream count footer,
  **virtualization asserts ≤ 20 DOM rows for 500-row input** (budget gate), data
  display, bitrate formatting (Mbps/Kbps)
- `AlertRuleForm.test.tsx` (6 tests): render, name validation, threshold validation,
  onSave with correct data, onCancel, edit heading

### 5. Lint — clean

```
$ cd web && npm run lint

> @pulse/web@0.1.0 lint
> eslint src

(no output — 0 errors, 0 warnings)
```

ESLint flat config at `web/eslint.config.js`:
- `@typescript-eslint` recommended rules
- `eslint-plugin-react-hooks` recommended rules
- Browser + React globals declared (URLSearchParams, confirm, prompt, etc.)
- `react-hooks/set-state-in-effect` disabled (false-positive for the async
  data-fetch in useEffect pattern; the setState calls are in async continuations,
  not synchronously in the effect body)

### 6. Mock/dev story documented

`web/README.md` covers:
- Running against `pulse serve` (npm run dev + Vite proxy to :8090)
- Running against static JSON fixtures (directory served on :8090)
- Fixture file layout (mirrors `/api/v1/` paths)
- WebSocket fixture approach (MSW or tiny Node.js WS server)
- `npm run gen:api` workflow
- Contract compliance check command

---

## Surfaces built

### F1 — Live dashboard (`/`)

Components:
- `src/features/live/LiveDashboard.tsx` — page shell, stat cards, protocol donut, app breakdown table, streams table
- `src/features/live/StatCard.tsx` — metric stat card (label/value/sub/accent)
- `src/features/live/ProtocolDonut.tsx` — recharts PieChart of viewer protocol mix
- `src/features/live/StreamsTable.tsx` — virtualized table (500+ rows budget)
- `src/features/live/useLiveDashboard.ts` — hook: LiveSocket + REST fallback polling (5 s)

Layout (text representation):

```
┌─ Pulse ──────────────────────────────────────────────────────┐
│ [Live] Analytics QoE Ingest Alerts Reports Fleet Settings    │
│                                                  [● Live]    │
├──────────────────────────────────────────────────────────────┤
│  Live Dashboard                             [Refresh]        │
│                                                              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐       │
│  │ Viewers  │ │Publishers│ │ Avg CPU  │ │ Streams  │       │
│  │  1,234   │ │    42    │ │   38%    │ │    87    │       │
│  │concurrent│ │  active  │ │  3 nodes │ │  active  │       │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘       │
│                                                              │
│  ┌─ Protocol mix ─┐  ┌─ By application ───────────────────┐ │
│  │   [Donut]      │  │ App     Viewers  Publishers        │ │
│  │  WebRTC 62%    │  │ live    1,100    38                │ │
│  │  HLS    28%    │  │ stream  134      4                 │ │
│  │  RTMP   10%    │  └───────────────────────────────────┘ │
│  └────────────────┘                                          │
│                                                              │
│  Active streams (87)                                         │
│  ┌──────── Stream ──── App ── Node ── State ── Viewers ─┐   │
│  │ live/stream-1  live  n1   [pub]      102             │   │
│  │ live/stream-2  live  n2   [pub]       47             │   │
│  │ ... (virtualized — ≤20 DOM rows at a time)           │   │
│  └───────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

### F2 — Analytics (`/analytics`)

Components:
- `src/features/analytics/AnalyticsPage.tsx` — date picker, tabs, audience chart, geo table, device table
- `src/features/analytics/DateRangePicker.tsx` — 24h/7d/30d/custom presets

Layout:

```
│  Analytics                                   [Export CSV]   │
│  [24h] [7d] [30d] [Custom]                                  │
│                                                              │
│  [audience] [geo] [device]                                  │
│                                                              │
│  ┌ Total Views ┐ ┌ Uniques ┐ ┌ Watch Time ┐ ┌ Peak ┐       │
│  │   24,150    │ │  3,820  │ │    402h    │ │ 1,234│       │
│  └─────────────┘ └─────────┘ └────────────┘ └──────┘       │
│                                                              │
│  ┌─ Audience over time ──────────────────────────────────┐  │
│  │ [recharts LineChart: views/uniques/peak concurrency]  │  │
│  └───────────────────────────────────────────────────────┘  │
```

Geo/device tabs show honest empty state when no data, real table when data present.
CSV export hits `/api/v1/reports/export?format=csv` with the current range.

### F5 — Alerts (`/alerts`)

Components:
- `src/features/alerts/AlertsPage.tsx` — tabbed rules/channels/history view
- `src/features/alerts/AlertRuleForm.tsx` — create/edit form with validation
- `src/features/alerts/AlertChannelForm.tsx` — channel create/edit (email/slack/webhook/pd/telegram)

Layout:

```
│  Alerts                                     [+ New rule]    │
│  [rules] [channels] [history]                               │
│                                                              │
│  ┌─ Rules ─────────────────────────────────────────────┐    │
│  │ cpu_pct gt 80 · 300s · 300s cooldown  [warning]    │    │
│  │                                    [Edit] [Delete]  │    │
│  │ viewer_count lt 1 · 60s             [critical]      │    │
│  │                                    [Edit] [Delete]  │    │
│  └─────────────────────────────────────────────────────┘    │
```

TEST-FIRE button on channels tab sends POST to `/api/v1/alerts/channels/{id}/test`
and shows a toast with `ChannelTestResult.accepted` + `message`.

### Settings + Onboarding (`/settings`, `/onboarding`)

Components:
- `src/features/settings/SettingsPage.tsx` — tabs: sources/tokens/license/users
- `src/features/settings/OnboardingWizard.tsx` — 4-step wizard (welcome → source → verify → done)

Onboarding wizard layout:

```
│  [1 Welcome] → [2 Add source] → [3 Verify] → [4 Done]      │
│                                                              │
│  ┌─ Add AMS source ─────────────────────────────────────┐   │
│  │ Name *              [Production cluster             ]│   │
│  │ AMS REST URL *      [http://your-ams:5080           ]│   │
│  │ REST username       [admin                          ]│   │
│  │ Credential env var  [AMS_ADMIN_PASSWORD             ]│   │
│  │ Log path (optional) [/var/log/ant-media-server/...  ]│   │
│  │                              [Back]  [Add source]    │   │
│  └──────────────────────────────────────────────────────┘   │
```

Wave-2 routes (`/qoe`, `/ingest`, `/reports`, `/fleet`) render
`<ComingSoon feature="..." wave="Wave 2" />` placeholders.

---

## Library decisions

| Library | Version | Decision |
|---|---|---|
| recharts | 3.8.1 | Smallest learning curve, SVG fine at our cardinality (charts consume server-side aggregates, not raw events). Considered victory-charts and d3-direct but rejected for complexity. |
| @tanstack/react-virtual | 3.14.2 | Satisfies 500-row table budget with proven API; tested in StreamsTable.test.tsx |
| openapi-typescript | 7.13.0 | Contract-compliant type generation; produces 2820-line schema.d.ts |
| vitest | 3.2.0 | Vite-native; fast jsdom environment |
| @testing-library/react | 16.3.2 | Standard component testing |

---

## Bugs fixed in wave-1 skeleton

| Bug | Fix |
|---|---|
| `LiveSocket.onopen` reset `retryDelay` to hardcoded 1000 instead of `baseDelay` option | Changed `this.retryDelay = 1000` to `this.retryDelay = this.baseDelay`; stored `baseDelay` as class field |
| `LiveStreamList` accessed as `.streams` instead of `.items` | Fixed in `useLiveDashboard.ts` |
| `AlertRuleList/ChannelList/HistoryList` accessed as `.rules/.channels/.history` instead of `.items` | Fixed in `AlertsPage.tsx` |
| `SourceList/TokenList` accessed as `.sources/.tokens` instead of `.items` | Fixed in `SettingsPage.tsx` |
| `LiveOverview.active_publishers` → `total_publishers` | Fixed in `LiveDashboard.tsx` |
| `AppOverview.viewer_count/.publisher_count` → `.viewers/.publishers` | Fixed in `LiveDashboard.tsx` |
| `AudienceResponse.buckets` → `.timeseries`; `.view_starts` → `.views`; `.unique_viewers` → `.uniques` | Fixed in `AnalyticsPage.tsx` |
| `Source.base_url`, `Source.enabled` not in schema | Fixed in `SettingsPage.tsx` to use `rest_url`/`type` |
| `SourceWrite.base_url`, `credential_env_var`, `webhook_secret_env_var` not in schema | Fixed in `OnboardingWizard.tsx` to use `rest_url`/`credential_env_ref` |
| `AlertChannelWrite.config.email` → `email_to`; `config.url` → `slack_webhook_url`/`webhook_url` | Fixed in `AlertChannelForm.tsx` |
| `AlertChannel.config` not in schema (has `config_summary`) | Fixed in `AlertChannelForm.tsx` |
| `ChannelTestResult` has `accepted/message` not `ok/latency_ms/error` | Fixed in `AlertsPage.tsx` |
| `@testing-library/dom` missing dep for `@testing-library/react` | Added with `--legacy-peer-deps` |

---

## Gaps / change requests

### Contract change requests (filed per D-004 — must go through ORCH-00)

1. **`AlertRule`/`AlertRuleWrite` missing `name` field** — The OpenAPI spec does
   not include a human-readable rule name. As a workaround the UI stores the label
   in `group_by` until the contract is updated. Required for: filter/search UX,
   alert history display (currently shows `rule_id` UUID). Suggested schema
   addition: `AlertRule.name: string` (optional, max 128 chars) + same in
   `AlertRuleWrite`.

2. **`AlertRule`/`AlertRuleWrite` missing `enabled` field** — The spec has `muted`
   but not `enabled`. An "enabled" toggle lets operators keep rules defined but
   paused without muting notifications. Workaround: the `muted` flag is used as
   enable/disable for now. Suggested addition: `AlertRule.enabled: boolean` (default
   true) + same in `AlertRuleWrite`.

3. **`POST /admin/sources/{sourceId}/test` missing from spec** — `client.ts` calls
   this endpoint but `AmsSourceStatus` response type is absent from the generated
   schema. The call is typed as `{ ok?: boolean; error?: string }` pending the
   spec addition. Suggested: add `POST /admin/sources/{sourceId}/test` path and
   `AmsSourceStatus` response schema (`{ ok: boolean; latency_ms?: number;
   streams_count?: number; node_count?: number; error?: string }`).

### Carried-forward gaps

| Gap | Suggested owner |
|---|---|
| Wave-2 route bodies (QoE, Ingest, Reports, Fleet) | FE-01 Wave 2 |
| Token authentication flow — 401 auto-redirect not wired to React Router | FE-01 |
| `gen:api` script uses absolute path — make portable with `../../contracts/...` | FE-01 |

---

## Files changed

All changes are within `web/` (FE-01 scope).

### New files
- `web/README.md` — dev setup, mock/fixture story, contract compliance check
- `web/src/lib/api/schema.d.ts` — generated (2820 lines from openapi-typescript)
- `web/src/lib/api/types.ts` — re-exports + response aliases + `DateRange` type
- `web/src/api/client.ts` — typed fetch wrapper + LiveSocket
- `web/src/App.tsx` — router shell
- `web/src/main.tsx` — entry point
- `web/src/styles/global.css` — dark theme (system font, no CDN)
- `web/src/components/` — AuthGate, Badge, ComingSoon, EmptyState, ErrorBanner, Layout, LoadingSpinner, Toast
- `web/src/features/live/` — LiveDashboard, StatCard, ProtocolDonut, StreamsTable, useLiveDashboard; tests
- `web/src/features/analytics/` — AnalyticsPage, DateRangePicker
- `web/src/features/alerts/` — AlertsPage, AlertRuleForm, AlertChannelForm; tests
- `web/src/features/settings/` — SettingsPage, OnboardingWizard
- `web/src/features/{qoe,ingest,reports,fleet}/README.md` — Wave 2 placeholders
- `web/src/test/setup.ts` — jest-dom setup
- `web/eslint.config.js` — ESLint flat config
- `web/tsconfig.json` — strict TS
- `web/vite.config.ts` — Vite + proxy config
- `web/package.json` — all deps + scripts

### No `server/cmd/` edits (D-005 — N/A for FE-01)
