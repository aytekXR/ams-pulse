# SESSION-46 — planned at S45 close (D-107)

> **✅ CLOSED 2026-07-16 (D-108, PR #89).** Both candidates shipped: (1) probe-runner entitlement gate
> (`prober.Config.EntitlementGate` → `lic.CheckProbes`); (2) live-WS auth — finding 2 was subtler than ranked
> below (a route/middleware mismatch, not just a handler bug), so the route was **moved** to
> `downloadAuthMiddleware` and the handler reads `ctxTokenKey`. Full evidence + the verify-before-build note:
> `decisions.md` D-108. S47 candidates (findings 3–6) carried to `sessions/SESSION-47.md`.

> Written by SESSION-45 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the S44 audit backlog (D-106) before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-47

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this plan if a
higher-leverage move exists.** Verify candidate status AND product-viability against the code before committing
(S38/S43 overturned their leads; S39–S42/S44/S45 confirmed theirs). **Re-verify each audit finding against the
code before building** — the D-106 audit is a strong signal, not a licence to skip verification.

## ★★ Context — working through the S44 audit backlog (13 confirmed bugs)

The S44 adversarial audit found 13 confirmed defects. Shipped so far: **S44** the security cluster (D-106,
PR #85); **S45** the reports-scheduler cluster (D-107 — the `PUT` BLOCKER + cron day-of-month). **S46 = the
entitlement + WS-auth cluster; S47 = audit-integrity + hardening.** Full file:line + scenarios: D-106.

## S46 candidates — entitlement + WS auth [M]

1. **MAJOR — probe runner executes probes after a Pro→Free downgrade** (`server/internal/prober/prober.go:~101`).
   The HTTP CRUD handlers gate `CheckProbes()` (403 on Free), but the background `Runner.Run()` refresh tick
   (`source.ListEnabled(ctx)` → run all enabled probes) does NOT check the live entitlement — so a tenant that
   downgrades keeps probing (the S37 "enforced, not decorative" class). **Verify**: read prober.go's Run loop +
   how it accesses the license (does it hold a `*license.Manager`? if not, thread one in). **Fix**: skip the
   tick (or clear the probe set) when `CheckProbes()` fails. Mutation-prove: downgrade → tick does not probe.
   ⚠ Scope note: confirm the runner has access to the license manager; if not, this needs a wiring seam
   (pin it, D-101 style).
2. **MAJOR — `handleLiveWS` ignores cookie auth already validated by middleware** (`server/internal/api/server.go:~1091`).
   `bearerAuthMiddleware` validates the `pulse_session` cookie and stashes the token in `ctxTokenKey`, but
   `handleLiveWS` re-extracts from the `Authorization` header / `?token=` only, so an OIDC cookie-session user
   (no header) is rejected from `GET /api/v1/live/ws`. **Fix**: read the validated token from context (it's
   already there). **Verify**: confirm the middleware runs on the WS route and that the handler currently
   ignores `ctxTokenKey`. Mutation-prove with a cookie-only request → 101/authorized.

## S47 candidates — audit integrity + hardening [S, several XS]

3. **MAJOR ×2 — `handleDeleteUser` / `handleRevokeToken` emit a false audit entry + 204 for a non-existent id**
   (`server.go:~2180` / `~2045`). `DeleteUser`/`DeleteToken` ignore `RowsAffected`, so deleting a made-up id
   audits a fabricated `user.delete`/`token.revoke` and returns 204 instead of 404 — corrupting the compliance
   trail (S38 missing-id class). **⚠ finding 6 (revoke-token) had a SPLIT verdict — re-verify first**: check
   whether `DeleteToken` surfaces rows-affected. **Fix**: check rows-affected → 404 + skip the audit when
   nothing was deleted.
4. **MINOR — `handleCreateUser` / `handleCreateToken` audit AFTER the re-fetch guard** (`server.go:~2115` /
   `~2031`): a committed create can go unrecorded if the re-fetch nils (S40 class, fixed for *update* but
   missed for *create*). **Fix**: pre-assign the UUID and audit before the re-fetch. (`uuid` is already in
   go.mod.)
5. **MINOR — `handleCreateToken` accepts an arbitrary `kind`** (`server.go:~2010`): no allowlist → a
   `kind:"superadmin"` token is stored but authenticates nowhere (dead row). **Fix**: allowlist `api`/`ingest`
   → 422 otherwise (positive-allowlist, D-098).
6. **MINOR — anomaly boundary `>` vs `>=`** (`alert/wave3.go:~250` eval vs `anomaly.go:~532` detect): a z
   exactly at the sigma threshold is flagged on the tick/detect path (`>=`) but not the eval path (`>`).
   **Fix**: unify on the detect-path `>=` semantics.

## ⛔ At open — verify, do not assume (D-095 standing rule)

- `git log --oneline origin/main -4` — S45 (D-107, PR #87 `2787dcd`) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-31-g2787dcd`** (S45 roll-forward; rollback tag `pre-d107`). `/healthz` all-ok,
  `ams_env_configured: true`.
- Operator queue **live**: GHCR anon → 401; **AMS trial-expiry doc discrepancy (07-12 vs 07-27)** — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If eligible, promote `web-e2e`/`csp-e2e` off
  `continue-on-error` (green the last several rounds); a clean high-value win.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gomod:/go/pkg/mod -v pulse-gocache2:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./..."` (24/24).
- **Mutation-prove on a COPY**: `cp -a /repo/server /tmp/mut && cp -a /repo/contracts /tmp/contracts` (tests resolve the meta DDL at `../contracts`); mutate `/tmp/mut`; test there. **Target the mutation precisely** — identical text in sibling handlers (e.g. create vs update) can over-match and cause a build-fail instead of a clean RED (S45 lesson: use a perl range anchored on a unique line). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` (unstage only) is fine.
- **Contract change? `cd web && npm run gen:api`.** **New migration? FIVE places** (0004 audit_log last shipped → next 0005). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: tag `pre-dNNN` → backup (rc 0) → STAMPED build (`--build-arg`) → assert stamp ≠ dev → `up -d` WITHOUT `--build` → smoke. Build takes >2 min → run it with a longer Bash timeout or in the background. Roll forward ONLY if server/web *source* changed (S46 probe-runner + WS DO change source).

## Binding lessons (carry into every wave)

1. Verify product-viability AND candidate-status before building (S38/S43).
2. A gate with no test is not a gate; mutation-prove every guard/e2e; audit on the committed-write path before any re-fetch (S40). Pin wiring seams (D-101) — relevant to the prober license-manager seam.
3. Independent adversarial review before merge for non-trivial code (S40/S44/S45).
4. Positive allowlists over blocklists for authz (D-098) — findings 5.
5. No silent scope caps; don't invent scope. The S44 audit proved real autonomous work exists — keep working it.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-108** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-47; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-47.md` (carry the standing-directive header + the remaining findings).
6. **Roll prod forward** if server/web *source* changed.
