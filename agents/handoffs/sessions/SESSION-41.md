# SESSION-41 — planned at S40 close (D-102)

> Written by SESSION-40 close (2026-07-15). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5 roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-42

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this plan if a
higher-leverage move exists.** This file is a starting point, not a contract. Record any revision in the
D-103 open block. **Verify candidate status AND product-viability against the code before committing** —
S38 overturned its plan, S39 & S40 confirmed theirs; the clause cuts both ways.

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.**

- `git log --oneline origin/main -4` — S40 (D-102, PR #77) + its docs PR should be on `origin/main`.
- Prod should print the **S40 build `v0.4.0-21-g0b7decc`** (rolled forward at S40 close):
  `sg docker -c "docker exec pulse-prod-pulse-1 /usr/local/bin/pulse version"`.
- `/healthz` should still report `ams_env_configured: true`.
- **Prove the S40 feature is live** (side-effect-free): `audit_log` table exists — WAL-aware copy per
  `deploy/runbooks/upgrade-rollback.md` "SQLite WAL gotcha", or GET `/api/v1/admin/audit-log` with the admin
  token (in `oguz-testing.md`) → expect **200** (empty items until an admin change is made; a **500** means
  the migration did not run). Do not create throwaway prod resources just to populate it.
- Re-check the operator queue **live**: GHCR anon pull → 401; **AMS licence expiry — SEE THE NEW ITEM
  BELOW: the documented value is inconsistent (07-12 vs 07-27) and must be operator-confirmed.**

## 🔧 Environment gotchas — read BEFORE running any gate

- **Go cannot run bare-metal.** Repo-root mount + `-w /repo/server`:
  ```sh
  sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo \
    -v pulse-gomod:/go/pkg/mod -v pulse-gocache2:/root/.cache/go-build \
    -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./..."   # S40 census: 24/24
  ```
- **Contract change? Regenerate `schema.d.ts`** (`cd web && npm run gen:api`) or the `contracts` CI check drifts.
- **New meta migration? Update FOUR things** (S40 lesson): the SQLite contracts file, the PG contracts file,
  the PG embed copy (`sql/postgres_000N_*.sql` + `embed_pg.go`), the idempotent SQLite `applySchemaUpgrades`
  block — AND `meta_pg_integration_test.go`'s `sqliteSchemaVersions` combined list, or `TestPG_MigrationParity`
  goes RED in CI (it runs against a real postgres:16; the SQLite side is skipped locally).
- **Playwright needs built `dist/`** — `npm run build` first; skip only if zero web files changed (say so).
- **`sleep` in the foreground is BLOCKED.** Pin an ABSOLUTE repo path in `-v`.
- **Prod deploy is LOCAL** — `deploy/runbooks/upgrade-rollback.md`: validate → tag `pre-dNNN` → backup (exit 0)
  → STAMPED build → assert stamp ≠ dev/unknown → `up -d` WITHOUT `--build` → evidence smoke. Rollback tag from
  S40: `pre-d102` = `v0.4.0-19-g38111c9`.

## Mission

> ### Confirm the AMS licence expiry — the documentation is inconsistent (07-12 vs 07-27).
> This is now the top operator item (see operator-expected). If the true expiry is 2026-07-12 it has ALREADY
> lapsed and the next `antmedia` restart = total ingest death. GHCR is still private (anon → 401).

## S41 candidates — pick by leverage (verify against the ledger AND product-viability first)

1. **Audit trail Phase 2 — complete the S40 feature** [S–M] — **strongest continuation.** S40 shipped the
   backend + read API but left two documented gaps: (a) **an audit-log web UI** — `GET /admin/audit-log`
   has no page; add an `AuditLogPage` (the typed `AuditEntry`/`AuditLogPage` schema already exists in
   `schema.d.ts`) so operators can actually SEE the trail; (b) **audit OIDC auto-provisioning** — `oidc.go`
   CreateUser creates users on first SSO login outside `handleCreateUser`, so those creations aren't
   recorded; design the actor model (SSO subject/groups) and add the capture. Either half is a self-contained
   session; together they make the audit trail a fully-realized feature.
2. **The two S34 e2e gaps** [S] — Reports **Schedules** tab never activated by a test; Probes **create**
   happy-path never driven. Bounded confidence win.
3. **Reconcile the dead `PULSE_LICENSE_OFFLINE_FILE` path** [XS] — read by `internal/config/config.go` but
   `config.Load()` never runs. Wire or delete the ghost.
4. **Seed a default `license_expiry` rule** [XS] — S39 shipped the metric but left rule-creation to the
   operator. Product ruling first (threshold + channel).
5. **§2.7 CI job promotions** [S] — date-gated **≥ 2026-07-23**. Check the date at open; promote
   `web-e2e`/`csp-e2e` off `continue-on-error` once eligible. (This would also stop the recurring `csp-e2e`
   flake from muddying merges — though note it flakes on the `Live Dashboard` render, which may need a fix
   first.)

### BLOCKED — do not start without the operator

- **Team-management UI** — BLOCKED on the operator product ruling (operator-expected item 10).
- **§2.19 uipro Wave 2+** — OPERATOR-DIRECTED; `brandkit/` is the operator's (D-071, G7 not approved).

**Not candidates (DONE):** §2.20 (S35), §2.21 (S36), §2.22 (S37), §2.23 (S38), §2.24 (S39), §2.25 (S40).

## ⚠️ Binding lessons — carry into every wave

1. **★ Verify product-viability AND candidate-status before building** — the clause cuts both ways.
2. **★ A gate with no test is not a gate; a green suite is not a working feature.** Mutation-prove every
   guard; **pin wiring seams** (D-062/D-101); **and audit on the committed-write path, before any re-fetch**
   (S40 review lesson — a re-fetch guard can drop the audit for a mutation that already happened).
3. **★ A new meta migration touches FIVE places** (see gotchas) — the PG parity test will catch a miss, but
   only in CI. Run the whole `./...` before pushing.
4. **★ Contract changes need `schema.d.ts` regenerated**; **audit the whole family** when you touch one handler.
5. **RUN the doc/path; do not read it.** Adversarially verify findings. **No silent scope caps** — document
   what a feature does NOT cover (S40 documented 4 out-of-scope paths rather than pretend full coverage).
6. **Positive allowlists over blocklists** for authz.

## Gates (before any commit)

- Web: lint + `tsc --noEmit` + build + `npx vitest run` + **Playwright in docker AFTER `npm run build`**
  (skip only if zero web files changed — say so). Contract drift: `npm run gen:api` → check `schema.d.ts`.
- Any Go change: full suite in `golang:1.25` docker (24/24), `vet`, `gofmt`.
- **`brandkit/` byte-untouched unless the operator ruled (D-071).** `docs/marketplace/` DRAFT-INTERNAL (D-081).
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** — the only expected dirty file; a CLEAN `git status`
  is a FAILURE signal.
- **NEVER** `git reset --hard` / `git checkout -- .` / `git stash` / `git clean` / `git restore` (D-096).
- **Never restart or "fix" AMS** — observe-only.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — and VERIFY the merge landed on `origin/main`.**
2. `decisions.md` **D-103** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-42; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-42.md` (carry the standing-directive header).
6. **Roll prod forward** if server/web code changed, per `deploy/runbooks/upgrade-rollback.md` — STAMPED
   build (`--build-arg` on `build`, then `up -d` WITHOUT `--build`); smoke with **evidence**.

---

## ✅ RESULT — S41 DONE (D-103, PR #79, 2026-07-15)

**Chose candidate 1 (audit-log web UI) — the self-contained half of "audit trail Phase 2."** Added
`AuditLogPage.tsx` (read-only table + cursor "Load more", mirroring `AnomaliesPage`, no tier gate),
`adminApi.listAuditLog`, the `AuditEntry`/`AuditLogPage` type re-exports, and the router + left-nav wiring.
Web-only — no Go/contract change (the endpoint + schema shipped in S40).

**Gates:** `tsc` · **650 vitest** (incl. 10 new — states, actor fallback, load-more append + cursor param,
design-token pins) · `build`. **3 Playwright e2e** proven green in the official
`mcr.microsoft.com/playwright:v1.61.1-jammy` image (the local host lacks browser libs — the dockerised
image is the correct local runner; the `--with-deps` CI path needs root). CI all required checks green
(web-e2e, csp-e2e, e2e) — no flake this round.

**Prod rolled forward:** rollback `pre-d103` = `v0.4.0-21-g0b7decc`; backup exit 0; STAMPED build
**`v0.4.0-23-ga44691b`** deployed; evidence smoke green (healthz all-ok, stamp `-23-ga44691b`, limits
`512M/0.5cpu`, logs clean). **New UI proven served** — the live JS bundle contains the AuditLogPage strings.

**Operator action: NONE.** The carried AMS-expiry-confirmation item persists (runbook 07-12 vs ledger 07-27).
GHCR anon → 401. Full evidence: `decisions.md` D-103.
