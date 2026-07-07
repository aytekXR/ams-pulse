# Pulse e2e test plan — C-e2e + C-Playwright (Phase-2 testing tail, deferred D-045)

> Written 2026-07-07 during the `pulse-prod-harden` session. Every trigger mechanism below was
> verified against live code (file:line), not guessed. Execute as a `pulse-e2e-backfill` workflow:
> one disjoint-scope author per item → TDD → adversarial verify → ORCH gate/commit (D-047 pattern).

## Current baseline (verified)

`.github/workflows/e2e.yml` stands up `-p pulse-e2e` (base + `deploy/docker-compose.ci.yml`:
clickhouse, mock-ams from `qa/mock-ams` on 127.0.0.1:19090, pulse-migrate, pulse on
127.0.0.1:18090/18091), greps the bootstrap admin token (`plt_…`) from pulse logs, then asserts:
healthz 200, migrate exit 0, CH tables exist, `POST /control/publish` → `/api/v1/live/overview`
activity > 0. Teardown `down -v`, logs dumped on failure. That's ALL it asserts today.

## Part A — C-e2e: deepen `e2e.yml` (same job, reuse stack + token)

### A1. Alert fires → `/alerts/history` [S]
- Create rule via `POST /api/v1/alerts/rules` (handler `server.go:1129`, payload shape
  `alertRuleFromAPI` `server.go:1932`): `{"name":"e2e bitrate floor","metric":"ingest_bitrate_floor",
  "operator":"lt","threshold":99999,"window_s":0,"cooldown_s":1}`.
  mock-ams publishes at a HARDCODED 2000 kbps (`qa/mock-ams/main.go:107`) → fires deterministically.
  Evaluator tick = 5s (`evaluator.go:131` clamps), `window_s:0` → near-instant.
- Bounded poll (30s, 2s interval): `GET /api/v1/alerts/history?state=firing` → ≥1 entry with
  `metric=="ingest_bitrate_floor"`. Print the JSON body on failure.
- Cannot-false-green: threshold 99999 vs fixed 2000 — the only way this passes is the full
  restpoller→snapshot→evaluator→meta-store→API path working.
- Phase 2 (optional): attach a webhook channel at a dead URL → assert a `delivery_failure`
  history row (pins D-049 e2e). Keep out of round 1 — retry exhaustion adds ~8s wall time.

### A2. Beacon POST → `/qoe/summary` with a mock Pro+ license [M]
- License: Free tier 403s beacon ingest (`CheckBeaconIngest`, `beacon.go:306`) and `/qoe/summary`
  (`CheckDataAPI`). Test-license mechanism ALREADY EXISTS at unit level: generate an ed25519 pair,
  sign Pro claims, set `PULSE_LICENSE_PUBKEY` (overrides embedded dev key, `license.go:168`) +
  `PULSE_LICENSE_KEY` — pattern `makeTestProLicense` (`vd10_beacon_test.go:65-92`).
  → Add `qa/licensegen/` (tiny Go main reusing the claims format) and a CI step:
  `go run ./qa/licensegen -tier pro` → export both env vars into the compose env. NO committed keys.
- Ingest token: `POST /api/v1/admin/tokens` (`server.go:402`, `handleCreateToken:1664`) with
  `kind=ingest`; beacon auth header `X-Pulse-Ingest-Token` (`server.go:1790`).
- `POST 127.0.0.1:18091/ingest/beacon` minimal valid payload (`contracts/events/beacon-event.schema.json`):
  `{"version":1,"session_id":"<uuid>","stream_id":"e2e-stream-1","events":[{"type":"session_start","ts":<ms>}]}`
  → expect `202 {"accepted":1}`.
- `GET /api/v1/qoe/summary` reads **`rollup_qoe_1h`** (`query.go:733,742`) — NOT raw beacon_events.
  Bounded poll **120s** (the D-039 flake was exactly this rollup latency; 90s was the fix there).
  Assert `totals` present and session count ≥ 1.
- Cannot-false-green: a Free-tier stack 403s the very first POST — the assertion chain proves the
  license mock, token mint, hostile-input ingest, CH write, rollup, and gated read all work.

### A3. Ingest degrade → `health_score` drop [S]
- Mechanism: `ComputeHealthScore` (`collector/ingest/health.go:297-348`):
  `0.35*(kbps/target) + 0.25*(fps/30) + 0.20*keyframe + 0.12*loss + 0.08*jitter`;
  ≥0.80 Good→100, 0.50–0.79 Warning→50 (`query.go:164-173` enum mapping).
- Preferred: add `POST /control/set_bitrate {"stream_id":…,"bitrate":N}` to mock-ams
  (~15 lines next to `set_viewers`, `qa/mock-ams/main.go`) → publish at 2000 (health 100 asserted),
  drop to 400 (ratio 0.2 → score ≈0.665 → Warning) → assert `GET /api/v1/live/streams` shows
  `health_score` 100 → 50 **transition** on a dedicated stream `e2e-stream-degrade`.
  (Fallback config-only: `PULSE_INGEST_TARGET_BITRATE_KBPS=8000` on the pulse service → static 50,
  but that skews every other assertion's baseline — the control endpoint is cleaner.)
- Timing: poll ≤30s (restpoller interval + aggregator); do NOT unpublish first (15s
  `sourceGoneTimeout` would zero the score and false-pass a `<100` assertion → assert `==50`).

### CI wiring for Part A
- Same required `e2e` job; every wait bounded with explicit `for i in $(seq …)` loops; every
  assertion failure prints the fetched body (`jq .`); logs-on-failure step already exists.
- D-042 rule restated in the workflow file header: a timeout that never resolves with more waiting
  is a deterministic bug — read the code, never bump the timeout.
- Budget: adds ~3 min worst case to the job (rollup poll dominates). Keep job < 12 min.

## Part B — C-Playwright: `web/e2e/` skeleton (NON-required CI job to start)

**Grounded corrections to the D-047 sketch** (scouted 2026-07-07):
1. There is **no `/login` route**. `AuthGate` (`web/src/components/AuthGate.tsx`) renders an inline
   token panel in-place when `getToken()` is null, and clears the token on a `pulse:auth:401`
   CustomEvent. The test is "unauth → token gate visible", NOT a redirect assertion.
2. **CSP is set by Caddy** (`deploy/config/Caddyfile:71`), NOT the Go server. Playwright against
   `vite preview`/pulse:8090 can never see a CSP header. Round 1: assert the CSP header cheaply in
   `e2e.yml` via curl against a caddy-fronted stack (or defer to U5 manual check); a caddy-fronted
   Playwright job with `page.on('console')` zero-violation assertions is phase 2.
3. Virtualization target: `StreamsTable.tsx` uses `@tanstack/react-virtual` (ROW_HEIGHT 44,
   maxHeight 520, overscan 10) → 500 streams render ≈22–32 DOM rows; `aria-rowcount == 501`;
   footer text "500 streams".

**Skeleton (5 specs, chromium-only, API route-mocked via `page.route` — no backend needed):**
1. `auth-gate.spec.ts` — unauth → token panel; invalid token → error stays on gate; mocked-valid
   token → dashboard renders.
2. `dashboard-render.spec.ts` — route-mock overview/streams; nav + overview cards render; ZERO
   console errors/pageerrors (fail on any).
3. `streams-virtualization.spec.ts` — route-mock 500 streams: rowgroup children ≤ 35,
   `aria-rowcount` 501, scroll-to-bottom shows last stream, footer "500 streams".
4. `auth-401.spec.ts` — mock a 401 mid-session → token cleared → gate reappears.
5. `csp.spec.ts` — `test.skip` with a comment pointing at the caddy-fronted phase-2 job.

**Infra:** `web/e2e/` + `web/playwright.config.ts` (baseURL `http://127.0.0.1:4173`, webServer
`npm run preview` after `npm run build` — tests the PROD bundle, same artifact the Go binary
embeds); `npm run test:e2e`. CI: new `web-e2e` job in ci.yml, `continue-on-error: true` + trace
upload on failure; promote to required after 2 weeks green. Playwright browsers cached.

## Sequencing (one `pulse-e2e-backfill` workflow session)
1. A1 alert→history [S] — zero deps, highest signal (the §6.3 verified gap: firing→history had no e2e).
2. A3 health-score transition + mock-ams `set_bitrate` [S].
3. A2 licensegen + beacon→rollup→qoe [M] — also unblocks real-QoE work pre-U3.
4. B skeleton specs 1–4 [M] (spec 5 skipped).
5. Phase 2: caddy-fronted CSP job, delivery_failure e2e, promotion of web-e2e to required.

Acceptance for every item: trigger verified at file:line, bounded waits, body-printing diagnostics,
and an explicit cannot-false-green argument in the PR description.
