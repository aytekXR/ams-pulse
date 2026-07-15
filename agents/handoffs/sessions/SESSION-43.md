# SESSION-43 — planned at S42 close (D-104)

> Written by SESSION-42 close (2026-07-15). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5 roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-44

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this plan if a
higher-leverage move exists.** This file is a starting point, not a contract. Record any revision in the
D-105 open block. **Verify candidate status AND product-viability against the code before committing** —
S38 overturned its plan, S39/S40/S41/S42 confirmed theirs; the clause cuts both ways.

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.**

- `git log --oneline origin/main -4` — S42 (D-104, PR #81) + its docs PR should be on `origin/main`.
- Prod should print the **S42 build `v0.4.0-25-g6a0226d`** (rolled forward at S42 close):
  `sg docker -c "docker exec pulse-prod-pulse-1 /usr/local/bin/pulse version"`.
- `/healthz` should still report `ams_env_configured: true` (all components ok).
- **Prove the S42 feature is present** (side-effect-free): the provisioning-audit path is dormant unless OIDC
  is configured (off in prod), so DO NOT try to manufacture a `user.provision` entry. The version stamp
  `-25-g6a0226d` IS the proof the code is deployed. If you want code-level assurance, `grep auditProvision`
  in `server/internal/api/oidc.go` on `origin/main`.
- Re-check the operator queue **live**: GHCR anon pull → 401; **AMS licence expiry — still the inconsistent
  07-12 vs 07-27 doc discrepancy, operator-only to resolve (SEE MISSION NOTE).**

## 🔧 Environment gotchas — read BEFORE running any gate

- **Go cannot run bare-metal.** Repo-root mount + `-w /repo/server`:
  ```sh
  sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo \
    -v pulse-gomod:/go/pkg/mod -v pulse-gocache2:/root/.cache/go-build \
    -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./..."   # census: 24/24 packages
  ```
- **Contract change? Regenerate `schema.d.ts`** (`cd web && npm run gen:api`) or the `contracts` CI check
  drifts — this bites even for a description-only OpenAPI edit (S42 lesson: the JSDoc comment changes).
- **New meta migration? Update FIVE things** (S40 lesson): SQLite contracts file, PG contracts file, PG embed
  copy (`sql/postgres_000N_*.sql` + `embed_pg.go`), the idempotent SQLite `applySchemaUpgrades` block — AND
  `meta_pg_integration_test.go`'s `sqliteSchemaVersions` combined list, or `TestPG_MigrationParity` goes RED
  in CI only. **Migration 0004 (S40 audit_log) is the last one shipped** — a new one is 0005.
- **Playwright:** the local host lacks browser libs (`libatk-1.0.so.0`). Run e2e in the official image —
  `sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse/web:/work -w /work \
  mcr.microsoft.com/playwright:v1.61.1-jammy npx playwright test"` — AFTER `npm run build`. Skip only if zero
  web files changed (say so). CI uses `npx playwright install chromium --with-deps` (root).
- **Do NOT overlap gate runs with heavy jobs on this box.** S42 saw a single `AlertsPage` vitest failure only
  because the Go suite + build ran concurrently (environment time 248 s); it passed 18/18 in isolation. Run
  gates serially, or re-run a lone failure in isolation before believing it.
- **`sleep` in the foreground is BLOCKED.** Pin an ABSOLUTE repo path in `-v`.
- **Prod deploy is LOCAL** — `deploy/runbooks/upgrade-rollback.md`: validate → tag `pre-dNNN` → backup (exit 0)
  → STAMPED build → assert stamp ≠ dev/unknown → `up -d` WITHOUT `--build` → evidence smoke. Rollback tag from
  S42: `pre-d104` = `v0.4.0-23-ga44691b`. Rollback tags are **docker image tags** (`pulse-prod-pulse:pre-dNNN`),
  not git tags.

## Mission

> ### Confirm the AMS licence expiry — the documentation is inconsistent (07-12 vs 07-27).
> Still the top operator item (see operator-expected). If the true expiry is 2026-07-12 it has ALREADY
> lapsed and the next `antmedia` restart = total ingest death. A session cannot resolve it (AMS creds
> operator-only; AMS enforces the licence only on restart). GHCR is still private (anon → 401).

## S43 candidates — pick by leverage (verify against the ledger AND product-viability first)

1. **Admin-scope gating of the audit-log read** [S] — **strongest continuation of the audit arc.**
   `GET /admin/audit-log` is auth-gated (any valid token) but **not admin-scope-gated**. S36/D-098 established
   the positive-allowlist scope model — writes require an `admin` scope. A read-only (`viewer`) token can
   currently read the entire audit trail (who changed what, from which IP), which is a mild information-
   disclosure gap for a compliance feature. **Product ruling first**: is the audit log admin-only, or
   readable by any authenticated operator? If admin-only, mirror the write-path scope check + mutation-prove
   it (delete the guard → one test RED). Small, self-contained, closes the audit feature cleanly. **Verify at
   open** how writes are gated today (find the `admin` scope check) so the read gate matches exactly.
2. **The two S34 e2e gaps** [S] — Reports **Schedules** tab never activated by a test; Probes **create**
   happy-path never driven. Bounded confidence win; needs Playwright in the docker image.
3. **Reconcile the dead `PULSE_LICENSE_OFFLINE_FILE` path** [XS] — read by `internal/config/config.go` but
   `config.Load()` never runs. Wire it (env → license offline file) or delete the ghost. Verify at open which
   is correct — do not assume it's dead without confirming no caller.
4. **Seed a default `license_expiry` rule** [XS] — S39 shipped the metric but left rule-creation to the
   operator. Product ruling first (threshold + channel); may be operator-gated.
5. **§2.7 CI job promotions** [S] — date-gated **≥ 2026-07-23**. Check the date at open; promote
   `web-e2e`/`csp-e2e` off `continue-on-error` once eligible. Note `csp-e2e` has been GREEN the last two
   rounds (S41, S42) — the flake may have settled, but confirm across a few runs before promoting.

### BLOCKED — do not start without the operator

- **Team-management UI** — BLOCKED on the operator product ruling (operator-expected item 10).
- **§2.19 uipro Wave 2+** — OPERATOR-DIRECTED; `brandkit/` is the operator's (D-071, G7 not approved).

**Not candidates (DONE):** §2.20 (S35), §2.21 (S36), §2.22 (S37), §2.23 (S38), §2.24 (S39), §2.25 (S40),
§2.26 (S41 audit-log UI), §2.27 (S42 OIDC-provision audit).

## ⚠️ Binding lessons — carry into every wave

1. **★ Verify product-viability AND candidate-status before building** — the clause cuts both ways.
2. **★ A gate with no test is not a gate; a green suite is not a working feature.** Mutation-prove every
   guard; **pin wiring seams** (D-062/D-101); **audit on the committed-write path, before any re-fetch** (S40).
   For a NEW authored path, mutation-prove the capture (S42: delete the `auditProvision` call → test RED).
3. **★ Run an independent adversarial review before merge** — it has repeatedly cut both ways (S40 found a
   real re-fetch-ordering defect; S42 confirmed the change was clean). Ask it to REFUTE, not to praise.
4. **★ A new meta migration touches FIVE places** (see gotchas) — the PG parity test catches a miss, in CI only.
5. **★ Contract changes need `schema.d.ts` regenerated** — even a description-only edit (S42). **Audit the
   whole family** when you touch one handler.
6. **RUN the doc/path; do not read it.** Adversarially verify findings. **No silent scope caps** — document
   what a feature does NOT cover (S40 documented out-of-scope paths; S42 then closed one of them).
7. **Positive allowlists over blocklists** for authz.

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
2. `decisions.md` **D-105** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-44; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-44.md` (carry the standing-directive header).
6. **Roll prod forward** if server/web code changed, per `deploy/runbooks/upgrade-rollback.md` — STAMPED
   build (`--build-arg` on `build`, then `up -d` WITHOUT `--build`); smoke with **evidence**.

---

## ✅ RESULT — S43 DONE (D-105, PR #83, 2026-07-15)

**★ Overturned the lead candidate at verify-at-open (the clause cut both ways).** Candidate 1 (admin-gate the
audit read) was refuted against the code: `requireWriteScope` (server.go:690) deliberately lets ALL reads
through and gates only writes on the `admin` scope — so the audit read follows the uniform "reads open" model
(same as `GET /admin/users`, `/admin/tokens`). Gating just it is inconsistent; gating the whole read surface
is a product ruling (and would 403 the S41 AuditLogPage for viewer SSO users). → deferred to operator.
Candidate 3 (`PULSE_LICENSE_OFFLINE_FILE`) was also overturned — the whole `config.Load` is an unwired
`HOOK(BE-02)` skeleton, so it's not XS. **Built candidate 2 instead.**

**Built: the two S34 e2e gaps** [test-only]. `probes.spec.ts` create happy-path (valid submit → POST →
appended + form closed); `reports.spec.ts` Schedules tab activation (click → GET schedules → row renders, not
empty state). **16/16** in the Playwright docker image. **Mutation-proven non-vacuous**: removing the
probe-append and the schedules fetch-on-activate turns EXACTLY these two RED, 14 others green. `tsc`+`eslint`
clean; CI all required green.

**Prod: NOT rolled forward — test-only.** Only `web/e2e/*.spec.ts` changed (not part of the served bundle),
so prod correctly stays **`v0.4.0-25-g6a0226d`** (S42). No adversarial-review agent this round — test-only +
mutation proof is the strongest non-vacuousness evidence.

**Operator action: NONE for the build.** Two NEW soft (non-blocking) operator/ruling items recorded: audit-read
access model (ruling); the BE-02 config skeleton (wire or delete). AMS-expiry-confirmation item persists.
GHCR anon → 401. Full evidence: `decisions.md` D-105.
