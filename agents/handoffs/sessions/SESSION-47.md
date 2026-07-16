# SESSION-47 — planned at S46 close (D-108)

> **✅ CLOSED 2026-07-16 (D-109, PR #91).** All 5 findings shipped + a CodeQL-surfaced password CWE-916.
> Verify-before-build overturned 1a/1b's ranked premise: the OpenAPI contract deliberately documents idempotent
> 204-on-missing, so the fix keeps 204 and only suppresses the phantom audit entry. 8 mutations RED; review clean
> (1 medium test-accuracy finding accepted + corrected). **The S44 13-bug backlog is now fully closed.** Full
> evidence: `decisions.md` D-109. Next: `sessions/SESSION-48.md` (re-scan mandate — no queued findings).

> Written by SESSION-46 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the S44 audit backlog (D-106) before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-48

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this plan if a
higher-leverage move exists.** Verify candidate status AND product-viability against the code before committing
(S38/S43 overturned their leads; S39–S42/S44/S45/S46 confirmed theirs). **Re-verify each audit finding against
the code before building** — the D-106 audit is a strong signal, not a licence to skip verification. S46 proved
this again: finding 2 (WS auth) was subtler than the audit stated (a route/middleware mismatch, not just a
handler bug), and building the literal claim would have been wrong.

## ★★ Context — closing out the S44 audit backlog (13 confirmed bugs)

The S44 adversarial audit found 13 confirmed defects. Shipped: **S44** security cluster (D-106, PR #85);
**S45** reports-scheduler cluster (D-107, PR #87); **S46** entitlement + WS-auth cluster (D-108, PR #89 — probe
runner ignored `CheckProbes()` on the tick; `handleLiveWS` rejected browser cookie/`?token=` auth). **S47 = the
final cluster: audit integrity + hardening (6 findings).** Full file:line + scenarios: D-106.

## S47 candidates — audit integrity + hardening [S, several XS]

1. **MAJOR ×2 — `handleDeleteUser` / `handleRevokeToken` emit a false audit entry + 204 for a non-existent id**
   (`server.go` — the admin delete-user and revoke-token handlers). `DeleteUser`/`DeleteToken` ignore
   `RowsAffected`, so deleting a made-up id audits a fabricated `user.delete`/`token.revoke` and returns 204
   instead of 404 — corrupting the compliance trail (S38 missing-id class). **⚠ finding 6 (revoke-token) had a
   SPLIT verdict in the S44 audit — RE-VERIFY FIRST**: read whether `DeleteToken` (meta store) surfaces
   rows-affected and whether the handler already 404s. **Fix**: check rows-affected → 404 + skip the audit when
   nothing was deleted. **Mutation-prove**: delete a non-existent id → the test asserts 404 AND no audit row
   (revert → false 204 + phantom audit → RED).

2. **MINOR — `handleCreateUser` / `handleCreateToken` audit AFTER the re-fetch guard** (`server.go`): a committed
   create can go unrecorded if the re-fetch nils (S40 class — fixed for *update* in S40, missed for *create*).
   **Fix**: pre-assign the UUID and audit **before** the re-fetch, on the committed-write path (`uuid` is already
   in go.mod; the S40 update-path fix is the template). **Mutation-prove**: force the re-fetch to nil → the
   create is still audited (revert → unrecorded → RED).

3. **MINOR — `handleCreateToken` accepts an arbitrary `kind`** (`server.go`): no allowlist → a
   `kind:"superadmin"` token is stored but authenticates nowhere (a dead row that looks valid). **Fix**:
   positive-allowlist `api`/`ingest` → 422 otherwise (D-098 — allowlist over blocklist). **Mutation-prove**:
   POST `kind:"bogus"` → 422 (revert → 201 with a dead row → RED).

4. **MINOR — anomaly boundary `>` vs `>=`** (`alert/wave3.go` eval path vs `anomaly.go` detect path): a z-score
   exactly at the sigma threshold is flagged on the detect/tick path (`>=`) but not the eval path (`>`) — a
   silent inconsistency between "what fired" and "what a re-eval says fired". **Fix**: unify on the detect-path
   `>=` semantics. **Verify BOTH call sites first** (confirm the operators are actually `>=` vs `>` and that they
   gate the same decision); **mutation-prove** with a z exactly at threshold → both paths agree.

> Ordering: do **1** first (highest severity, compliance-integrity, and it needs the split-verdict re-verify),
> then **2** (same audit-integrity theme, S40 template), then the two XS hardening items **3**/**4**. Each is a
> separate concern — commit per scope or bundle audit-integrity (1+2) and hardening (3+4) into two PRs if that
> reviews cleaner. **After S47 the S44 13-bug backlog is fully closed** — at S47 close, re-scan ROADMAP-V2 §2 /
> assessment §5 for the next-highest-leverage track (per the standing directive).

## ⛔ At open — verify, do not assume (D-095 standing rule)

- `git log --oneline origin/main -4` — S46 (D-108, PR #89) + its docs PR should be on `origin/main`.
- Prod should print the **S46 roll-forward stamp** (recorded in D-108; rollback tag `pre-d108`). `/healthz`
  all-ok, `ams_env_configured: true`.
- Operator queue **live**: GHCR anon → 401; **AMS trial-expiry doc discrepancy (07-12 vs 07-27)** — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If eligible, promote `web-e2e`/`csp-e2e` off
  `continue-on-error` (green the last several rounds); a clean high-value win.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` is required** (mounted repo → dubious git ownership). **Go test caching does NOT track the runtime-read OpenAPI spec** — after any `contracts/` edit, re-run the api package with `-count=1` or the result is stale-cached.
- **Mutation-prove on a COPY**: reference tree mounted read-only at `/repo`; `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts` inside the container (tests resolve the meta DDL at `../../../contracts`); mutate `/mut`; test there. **Target the mutation precisely** — identical text in sibling handlers (create vs update) over-matches (S45); a replacement string ending in `{` unbalances perl `{}` delimiters → use `#`-delimiters (S46). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` (unstage only) is fine.
- **Contract change? `cd web && npm run gen:api`** (→ `web/src/lib/api/schema.d.ts`; openapi-typescript 7 does NOT emit security schemes as types, so a security-scheme edit only touches JSDoc). **New migration? FIVE places** (0004 audit_log last shipped → next 0005). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: tag `pre-dNNN` → backup (rc 0) → STAMPED build (`--build-arg`) → assert stamp ≠ dev → `up -d` WITHOUT `--build` → smoke. Build takes >2 min → run it with a longer Bash timeout or in the background. Roll forward ONLY if server/web *source* changed. **S47 findings 1–4 all change server source → roll forward.**

## Binding lessons (carry into every wave)

1. Verify product-viability AND candidate-status before building (S38/S43); verify the audit's *mechanism*, not
   just its conclusion (S46 — the WS finding was a route mismatch, not the stated handler bug).
2. A gate with no test is not a gate; **mutation-prove every guard/e2e**; audit on the committed-write path
   before any re-fetch (S40 — directly relevant to finding 2). Drive tests through the REAL code path with a
   positive control so the harness can't be vacuous (S46 prober test).
3. Independent adversarial review before merge for non-trivial code (S40/S44/S45/S46) — and **fix the should-fixes
   it surfaces** (S46: the OpenAPI cookie-auth doc gap).
4. Positive allowlists over blocklists for authz (D-098) — directly relevant to finding 3.
5. No silent scope caps; don't invent scope. The S44 audit proved real autonomous work exists — keep working it.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-109** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-48; ROADMAP-V2 ledgers (mark the 13-bug backlog CLOSED at §2.29).
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-48.md` (carry the standing-directive header; at S47 close the audit backlog is done,
   so SESSION-48 re-scans for the next-highest-leverage track).
6. **Roll prod forward** if server/web *source* changed.
