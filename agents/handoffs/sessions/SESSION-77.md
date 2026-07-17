# SESSION-77 — planned at S76 close (D-138)

> Written by SESSION-76 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S73-AUDIT-FINDINGS.md`** (8-finding ledger; 5 shipped, ALL HIGH done).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-78

Re-verify each finding against the code before building — take the verified CORE. Do NOT stop after one session — at
close, update all docs, regenerate this plan, record progress + operator-needs, continue until the roadmap is complete
or a human/operator is genuinely required. **Ultracode is on**. **Workflow-script gotcha:** no backticks in workflow
prompt prose. **This is a WEB-only cluster** — the toolchain is different from the Go loop (see below).

## Goal — [8] MEDIUM: web SettingsPage silent error handlers (web-only, quick — validates the web/vitest CI loop)

`S73-AUDIT-FINDINGS.md`: 3 MEDIUM remain ([5]/[7]/[8]). Lead = [8] (self-contained, web-only, low-risk — a good first
web fix before the bigger [7] and [5]):

### [8] MEDIUM — `deleteSource` / `deleteToken` (+ `createApiToken` / `createIngestToken`) silently discard API errors
- **loc:** `web/src/features/settings/SettingsPage.tsx:132` (deleteSource) + `:169` (deleteToken); call sites `:303/406/502`;
  also `createApiToken` :139 / `createIngestToken` :160 (the review flagged these two share the gap).
- **Re-verify at open:** confirm these async handlers still `await` the API call with NO try/catch and are called as
  `() => void handler(id)` (swallowing the rejection). Confirm the CORRECT pattern already in the same file
  (`saveLicense` ~:180 uses try/catch + `toast(..., 'error')`) and in `ProbesPage.tsx`.
- **Fix:** wrap each of the four handlers in `try/catch { toast(err instanceof ApiError ? err.message : 'Operation
  failed', 'error') }` (mirror `saveLicense` / ProbesPage). Keep the success `toast` + `loadAll()` in the try.
- **Test:** a vitest/RTL component (or unit) test that mocks the API call to reject (e.g. an `ApiError` 500) and asserts
  an error toast is shown and no unhandled rejection. Check the existing web test conventions first
  (`web/src/**/*.test.tsx` / `*.test.ts`; how `toast` is mocked/observed). Mutation: remove a catch → the test reddens.

### WEB toolchain (DIFFERENT from the Go loop — carried gotchas)
- Node at `/home/aytek/.local/bin/node` (npx). From `web/`: `npm install` (if needed), `npm test` (vitest), `npm run
  build` (tsc + vite — the `web` CI job runs typecheck + build + vitest; `web-e2e`/`csp-e2e` run Playwright).
- **NO Go change expected for [8]** → no docker Go suite needed, but DO run the web build + tests. If [8] somehow
  touches the API contract (it should NOT), regen `web/src/lib/api/schema.d.ts`.
- `gofmt` gate is Go-only; for web, the lint/format is via the web CI (eslint/prettier if configured — check
  `web/package.json` scripts; run `npm run lint` if present before pushing).
- Prod roll-forward: [8] is `web/` source → the pulse binary embeds the web build (verify: the Dockerfile builds web
  and embeds `web/dist`), so a web change DOES require a prod roll-forward + the 5-check smoke (the served SPA changes).

### Later clusters (remaining S73)
- **[7] MEDIUM (security, operator-flagged)** admin token in the Live-dashboard WS URL → proxy logs. Options to weigh at
  open: (a) a short-lived single-use ticket — `POST /auth/ws-ticket` server endpoint + a small ticket store (in-memory
  map with expiry) + web `LiveSocket.connect` fetches a ticket first; (b) pass the token as a WebSocket **subprotocol**
  (`new WebSocket(url, ['bearer', token])` → sent as `Sec-WebSocket-Protocol` header, NOT in the URL/logs; server reads
  the header + echoes the selected subprotocol) — no new endpoint/state, but charset/echo caveats; (c) first-WS-frame
  auth. Server + web (+ OpenAPI + schema.d.ts if a new endpoint). Take the verified CORE + adversarial-review (security).
  Avoid the do-not-commit Caddyfile. **Operator has been told this is pending** — closing it retires the log-exposure note.
- **[5] MEDIUM** `QoEForStream` cross-tenant QoE — ⚠ WIDER: `AlertScope`/`AlertRuleRow`/`LiveStream` have NO Tenant field;
  thread Tenant through the aggregator → those structs, THEN the `QoEReader` signature. Multi-tenant-only impact
  (primary single-tenant model unaffected). Scope carefully; may split into a "tenant-in-live-pipeline" prerequisite.

## Pipeline (adapted for web)
1. **Verify-at-open:** `git log --oneline -3`, HEAD == origin/main, `git status` shows ONLY Caddyfile.prod dirty. Record
   **D-139 IN PROGRESS** in `decisions.md`. Branch `s77-d139`. **CHECK THE DATE** (§2.7 gate ≥ 2026-07-23).
2. **Re-verify vs code**; take CORE.
3. **Fix → test** with `cd web && npm test` (vitest); mutation-check by reverting a catch. Web has no `/tmp/pulsemut` Go
   mutation dance — mutate in place + `npm test`, then restore.
4. **Web build/typecheck** (`npm run build`) + **Go suite** (unchanged, but run once to confirm no cross-impact).
5. **Adversarial review** OPTIONAL for [8] (mechanical UX fix) — self-review likely suffices; the reviewer has caught
   web issues before, so consider a 1-lens pass on error-handling completeness (are ALL silent-discard handlers fixed?).
6. **PR → CI poll** (the `web` + `web-e2e` + `csp-e2e` jobs are the relevant ones; Playwright may flake — re-run) →
   **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** (web source → SPA changes; the binary embeds web/dist): `config -q` → tag
   `pulse-prod-pulse:pre-d139` → backup rc0 → STAMPED build → assert stamp ≠ dev → `up -d pulse` → 5-check smoke.
8. **Close docs:** `decisions.md` D-139 SHIPPED, CHANGELOG, `S73-AUDIT-FINDINGS.md` [8] ✅ DONE, ROADMAP §2.32 count,
   RESUME rotation, `operator-expected.md`, SESSION-77 CLOSED, SESSION-78 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker** (25 pkgs); mutation copy `/tmp/pulsemut` (not `/mut`). **CodeQL** flags `InsecureSkipVerify` in
  PRODUCTION Go only. Any OpenAPI change → regen `web/src/lib/api/schema.d.ts` (committed; web CI drift check).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod at **v0.4.0-89-g300251d**. 5-check smoke: startup
  version stamp, healthz 200, signed webhook 200 (HMAC from PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
