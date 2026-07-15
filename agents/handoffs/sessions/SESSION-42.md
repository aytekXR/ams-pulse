# SESSION-42 — planned at S41 close (D-103)

> Written by SESSION-41 close (2026-07-15). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5 roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-43

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this plan if a
higher-leverage move exists.** This file is a starting point, not a contract. Record any revision in the
D-104 open block. **Verify candidate status AND product-viability against the code before committing** —
S38 overturned its plan, S39/S40/S41 confirmed theirs; the clause cuts both ways.

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.**

- `git log --oneline origin/main -4` — S41 (D-103, PR #79) + its docs PR should be on `origin/main`.
- Prod should print the **S41 build `v0.4.0-23-ga44691b`** (rolled forward at S41 close):
  `sg docker -c "docker exec pulse-prod-pulse-1 /usr/local/bin/pulse version"`.
- `/healthz` should still report `ams_env_configured: true`.
- **Prove the S41 feature is live** (side-effect-free): the served JS bundle contains the `AuditLogPage`
  strings — `sg docker -c "docker exec pulse-prod-pulse-1 sh -c 'grep -l AuditLogPage /app/web/assets/*.js'"`
  (or curl the SPA and grep the bundle). The `/audit-log` route renders the table; no throwaway prod resource
  needed to prove render.
- Re-check the operator queue **live**: GHCR anon pull → 401; **AMS licence expiry — the documented value is
  still inconsistent (07-12 vs 07-27) and must be operator-confirmed. SEE THE MISSION NOTE BELOW.**

## 🔧 Environment gotchas — read BEFORE running any gate

- **Go cannot run bare-metal.** Repo-root mount + `-w /repo/server`:
  ```sh
  sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo \
    -v pulse-gomod:/go/pkg/mod -v pulse-gocache2:/root/.cache/go-build \
    -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./..."   # S40 census: 24/24
  ```
- **Contract change? Regenerate `schema.d.ts`** (`cd web && npm run gen:api`) or the `contracts` CI check drifts.
- **New meta migration? Update FIVE things** (S40 lesson): the SQLite contracts file, the PG contracts file,
  the PG embed copy (`sql/postgres_000N_*.sql` + `embed_pg.go`), the idempotent SQLite `applySchemaUpgrades`
  block — AND `meta_pg_integration_test.go`'s `sqliteSchemaVersions` combined list, or `TestPG_MigrationParity`
  goes RED in CI only (it runs against a real postgres:16; the SQLite side is skipped locally). **Migration
  0004 is the last one shipped** (S40 audit_log) — a new one is 0005.
- **Playwright:** the local host lacks browser libs (`libatk-1.0.so.0`). Run e2e in the official image —
  `sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse/web:/work -w /work \
  mcr.microsoft.com/playwright:v1.61.1-jammy npx playwright test"` (S41-proven) — AFTER `npm run build`.
  Skip only if zero web files changed (say so). CI uses `npx playwright install chromium --with-deps` (root).
- **`sleep` in the foreground is BLOCKED.** Pin an ABSOLUTE repo path in `-v`.
- **Prod deploy is LOCAL** — `deploy/runbooks/upgrade-rollback.md`: validate → tag `pre-dNNN` → backup (exit 0)
  → STAMPED build → assert stamp ≠ dev/unknown → `up -d` WITHOUT `--build` → evidence smoke. Rollback tag from
  S41: `pre-d103` = `v0.4.0-21-g0b7decc`.

## Mission

> ### Confirm the AMS licence expiry — the documentation is inconsistent (07-12 vs 07-27).
> Still the top operator item (see operator-expected). If the true expiry is 2026-07-12 it has ALREADY
> lapsed and the next `antmedia` restart = total ingest death. A session cannot resolve it (AMS creds
> operator-only; AMS enforces the licence only on restart, so live-ingesting doesn't disambiguate). GHCR is
> still private (anon → 401).

## S42 candidates — pick by leverage (verify against the ledger AND product-viability first)

1. **Audit OIDC auto-provisioning — close the audit-trail Phase-2 tail** [S–M] — **strongest continuation.**
   S40 documented it as explicitly out-of-scope: `oidc.go`'s `CreateUser` provisions a user on first SSO
   login **outside** `handleCreateUser`, so those account creations are the one mutating path the audit trail
   does NOT record. Design the actor model for a self-provisioning event (there is no bearer token — the actor
   IS the SSO subject/email/groups from the IdP claims, not `ctxTokenKey`), then capture a `user.provision`
   (or `user.create` with an `sso` actor kind) audit entry on that path. **Verify first** that the OIDC login
   flow actually reaches a create (S37 Enterprise-gated `/auth/oidc/*`; S38 found OIDC re-maps role every
   login) — the create only fires for a genuinely new subject. Self-contained; makes the trail complete.
2. **Admin-only gating of the audit read** [XS–S] — `GET /admin/audit-log` is auth-gated but not
   *admin-scope*-gated; S36/D-098 established the positive-allowlist scope model (`admin` scope for writes).
   A read-only token can currently read the whole trail. Product ruling first: is the audit log admin-only, or
   readable by any authenticated operator? If admin-only, add the scope check (mirror the write-path gate) +
   a mutation-proven test. Pairs naturally with candidate 1.
3. **The two S34 e2e gaps** [S] — Reports **Schedules** tab never activated by a test; Probes **create**
   happy-path never driven. Bounded confidence win.
4. **Reconcile the dead `PULSE_LICENSE_OFFLINE_FILE` path** [XS] — read by `internal/config/config.go` but
   `config.Load()` never runs. Wire or delete the ghost.
5. **Seed a default `license_expiry` rule** [XS] — S39 shipped the metric but left rule-creation to the
   operator. Product ruling first (threshold + channel).
6. **§2.7 CI job promotions** [S] — date-gated **≥ 2026-07-23**. Check the date at open; promote
   `web-e2e`/`csp-e2e` off `continue-on-error` once eligible. (Would also stop the recurring `csp-e2e` flake
   from muddying merges — though note it flakes on the Caddy-fronted `Live Dashboard` render, which may need a
   fix first.)

### BLOCKED — do not start without the operator

- **Team-management UI** — BLOCKED on the operator product ruling (operator-expected item 10).
- **§2.19 uipro Wave 2+** — OPERATOR-DIRECTED; `brandkit/` is the operator's (D-071, G7 not approved).

**Not candidates (DONE):** §2.20 (S35), §2.21 (S36), §2.22 (S37), §2.23 (S38), §2.24 (S39), §2.25 (S40),
§2.26 (S41 audit-log UI).

## ⚠️ Binding lessons — carry into every wave

1. **★ Verify product-viability AND candidate-status before building** — the clause cuts both ways.
2. **★ A gate with no test is not a gate; a green suite is not a working feature.** Mutation-prove every
   guard; **pin wiring seams** (D-062/D-101); **and audit on the committed-write path, before any re-fetch**
   (S40 review lesson — a re-fetch guard can drop the audit for a mutation that already happened). For a NEW
   audited path (candidate 1), mutation-prove the capture: delete the `s.audit(...)` and prove one test goes RED.
3. **★ A new meta migration touches FIVE places** (see gotchas) — the PG parity test catches a miss, in CI only.
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
2. `decisions.md` **D-104** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-43; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-43.md` (carry the standing-directive header).
6. **Roll prod forward** if server/web code changed, per `deploy/runbooks/upgrade-rollback.md` — STAMPED
   build (`--build-arg` on `build`, then `up -d` WITHOUT `--build`); smoke with **evidence**.
