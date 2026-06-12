# Pulse Web UI

Dark, ops-grade dashboard for Pulse — self-hosted analytics and QoE monitoring
for Ant Media Server.

## Tech stack

| Layer | Choice | Why |
|---|---|---|
| Framework | React 19 + React Router 7 | |
| Build | Vite 6 | |
| Types | TypeScript 5 strict | |
| Charts | recharts | Smallest learning curve, SVG fine at our cardinality |
| Virtualization | @tanstack/react-virtual | Satisfies 500-stream table budget |
| Tests | vitest + @testing-library/react | |
| Lint | ESLint 9 flat config | |

No external fonts or CDN imports — all assets are self-hosted (LAN self-hosting
rule per charter).

## Development

### Prerequisites

- Node 26 / npm 11

### Install

```sh
cd web
npm install
```

### Running against `pulse serve` (backend)

```sh
npm run dev
```

Vite proxies `/api` and `/live` (WebSocket) to `http://localhost:8090`, where
`pulse serve` runs. The dev server is at `http://localhost:5173`.

When `pulse serve` is not running, the UI shows error banners and falls back
to REST polling (WebSocket auto-reconnect with exponential backoff).

### Running against fixtures (no backend required)

The fixture dev story uses a minimal JSON fixture server. Create a directory
`dev-fixtures/` and serve it on port 8090:

```sh
# Install a simple static server (once)
npm install -g serve

# Serve fixtures from project root (adjust path as needed)
serve -p 8090 dev-fixtures/
```

Fixture file layout (mirrors the API):

```
dev-fixtures/
  api/v1/
    live/overview.json      # LiveOverview shape
    live/streams.json       # LiveStreamList shape
    alerts/rules.json       # AlertRuleList shape
    alerts/channels.json    # AlertChannelList shape
    alerts/history.json     # AlertHistoryList shape
    analytics/audience.json # AudienceResponse shape
    analytics/geo.json      # GeoResponse shape
    analytics/devices.json  # DeviceResponse shape
    admin/sources.json      # SourceList shape
    admin/tokens.json       # TokenList shape
    admin/license.json      # LicenseInfo shape
```

For live WebSocket development, use the MSW (Mock Service Worker) approach
or implement a tiny Node.js WebSocket server that sends the snapshot/delta/
heartbeat envelope format documented in the OpenAPI spec at `/live/ws`.

### Type generation

Types are generated from the contract (frozen after Wave 1 per D-004):

```sh
npm run gen:api
# or
npm run generate:api
```

This runs `openapi-typescript` against
`contracts/openapi/pulse-api.yaml` and writes `src/lib/api/schema.d.ts`.

**All API shapes must come from the generated `schema.d.ts`. Hand-rolled
shapes that duplicate the OpenAPI spec are a contract violation (see
`agents/definitions/FE-01-frontend.md`).**

To check compliance:
```sh
git grep -rn "interface.*Response\|interface.*Request" -- "src/" \
  | grep -v "schema.d.ts\|types.ts"
# should produce no output
```

## Available scripts

| Script | Purpose |
|---|---|
| `npm run dev` | Vite dev server (proxies to `pulse serve` on :8090) |
| `npm run build` | Production build (`tsc -b && vite build`) |
| `npm run lint` | ESLint flat config |
| `npm run test` | Vitest unit/component tests (run once) |
| `npm run test:ui` | Vitest with browser UI |
| `npm run gen:api` | Regenerate `src/lib/api/schema.d.ts` from OpenAPI spec |
| `npm run generate:api` | Alias for `gen:api` |

## Project structure

```
src/
  api/
    client.ts          # Typed fetch wrapper + LiveSocket class
  components/          # Shared components (AuthGate, Layout, Badge, Toast, etc.)
  features/
    live/              # F1 — live ops dashboard
    analytics/         # F2 — historical audience analytics
    alerts/            # F5 — alert rules, channels, history
    settings/          # Settings + onboarding wizard
    qoe/               # F3 — QoE (Wave 2 placeholder)
    ingest/            # F4 — ingest health (Wave 2 placeholder)
    reports/           # F6 — usage reports (Wave 2 placeholder)
    fleet/             # F7 — fleet view (Wave 2 placeholder)
  lib/
    api/
      schema.d.ts      # GENERATED — do not edit manually
      types.ts         # Re-exports + convenience aliases
  styles/
    global.css         # Dark theme; no external fonts
  test/
    setup.ts           # @testing-library/jest-dom setup
```

## Key architectural notes

- **Auth gate:** Token stored in `localStorage` under key `pulse_token`. A
  `401` from any API call surfaces the token entry screen. Use
  `Settings → API Tokens` to generate tokens.

- **Live dashboard (F1):** WebSocket at `/live/ws` delivers `snapshot` /
  `delta` / `heartbeat` envelopes. On disconnect the UI falls back to REST
  polling every 5 seconds. "New stream ≤ 10 s" requirement is satisfied by
  server push; the client does not debounce the WS stream list.

- **Streams table:** Virtualized with `@tanstack/react-virtual` — 500 rows
  render ≤ 20 DOM nodes at any time. Row height is 44 px.

- **Wave 2 routes** (`/qoe`, `/ingest`, `/reports`, `/fleet`) render
  structured "coming in this build" placeholders, not blank pages.

## Contract compliance

```sh
# Proof: no hand-rolled API shapes
git grep -rn "interface.*Response\|interface.*Request" -- "src/" \
  | grep -v "schema.d.ts\|types.ts"
# Expected output: empty
```

All API-typed values flow through `src/lib/api/types.ts` (re-exports from
`schema.d.ts`) or `components["schemas"][...]` direct references.
