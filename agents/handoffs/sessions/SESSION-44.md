# SESSION-44 — planned at S43 close (D-105)

> Written by SESSION-43 close (2026-07-15). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5 roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-45

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this plan if a
higher-leverage move exists.** This file is a starting point, not a contract. Record any revision in the
D-106 open block. **Verify candidate status AND product-viability against the code before committing** —
S38 and S43 both overturned their own plans; S39–S42 confirmed theirs. The clause cuts both ways.

## ★★ READ FIRST — the clean-autonomous backlog is thinning (S43 finding)

After S43's two verify-at-open overturns, most **high-leverage** remaining items now need an **operator
ruling** or a **future date**, not more code:
- **Audit-read access model** (operator ruling — operator-expected ⚑ A). Do NOT unilaterally gate the audit
  read: it follows the deliberate "all reads open / only writes admin-gated" model (`requireWriteScope`),
  and tightening it (or the whole admin-read surface) is a product choice that would also 403 the S41
  AuditLogPage for viewer SSO users.
- **BE-02 config skeleton** (operator ruling — operator-expected ⚑ B). `config.Load` (the YAML+env system,
  incl. `PULSE_LICENSE_OFFLINE_FILE`) is entirely unwired. Wire it (large) or delete it (a call on a
  documented skeleton). Don't do either without the ruling.
- **Default `license_expiry` alert rule** (operator ruling — threshold + channel).
- **Team-management UI** (blocked, operator-expected item 10).
- **§2.7 CI job promotions** — date-gated **≥ 2026-07-23**. **CHECK THE DATE AT OPEN.** If eligible, this is
  a real clean win (promote `web-e2e`/`csp-e2e` off `continue-on-error`; note both have been green the last
  three rounds, so the flake may have settled — confirm across a few runs first).

So SESSION-44's honest job: **re-verify the date** (if ≥ 07-23, do the CI promotion); otherwise pick a
bounded, low-risk **hygiene** candidate below, and **surface clearly** that the biggest moves now wait on the
operator (don't invent scope to look busy — "no silent scope caps", inverted).

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.**

- `git log --oneline origin/main -4` — S43 (D-105, PR #83) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-25-g6a0226d`** (S42; S43 was test-only, no roll-forward):
  `sg docker -c "docker exec pulse-prod-pulse-1 /usr/local/bin/pulse version"`.
- `/healthz` should still report `ams_env_configured: true` (all components ok).
- Re-check the operator queue **live**: GHCR anon pull → 401; **AMS licence expiry — still the inconsistent
  07-12 vs 07-27 doc discrepancy, operator-only to resolve.**

## 🔧 Environment gotchas — read BEFORE running any gate

- **Go cannot run bare-metal.** Repo-root mount + `-w /repo/server`:
  ```sh
  sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo \
    -v pulse-gomod:/go/pkg/mod -v pulse-gocache2:/root/.cache/go-build \
    -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./..."   # census: 24/24 packages
  ```
- **Contract change? Regenerate `schema.d.ts`** (`cd web && npm run gen:api`) — even a description-only edit
  (S42 lesson) changes the generated file and drifts the `contracts` CI check.
- **New meta migration? Update FIVE things** (S40) — migration **0004 (audit_log) is the last shipped**; a
  new one is 0005. Miss the `meta_pg_integration_test.go` `sqliteSchemaVersions` entry → `TestPG_MigrationParity`
  RED in CI only.
- **Playwright:** local host lacks browser libs. Run in the official image AFTER `npm run build`:
  `sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse/web:/work -w /work \
  mcr.microsoft.com/playwright:v1.61.1-jammy npx playwright test <spec...>"` (S43-proven). **Mutation-prove
  every e2e** (S43: remove the behavior → the test must go RED; the project's #1 e2e failure is vacuous tests).
- **Do NOT overlap gate runs with heavy jobs on this box** (S42/S43: a single vitest/e2e flake appeared only
  under concurrent load; passed in isolation). Run gates serially; re-run a lone failure isolated before believing it.
- **`sleep` in the foreground is BLOCKED.** Pin an ABSOLUTE repo path in `-v`.
- **Prod deploy is LOCAL** — `deploy/runbooks/upgrade-rollback.md`: validate → tag `pre-dNNN` → backup (exit 0)
  → STAMPED build → assert stamp ≠ dev/unknown → `up -d` WITHOUT `--build` → evidence smoke. Rollback tags are
  **docker image tags** (`pulse-prod-pulse:pre-dNNN`). If S44 is test/docs-only, **do NOT roll prod forward**
  (S43 correctly skipped it — a byte-identical bundle).

## Mission

> ### Confirm the AMS licence expiry — the documentation is inconsistent (07-12 vs 07-27).
> Still the top operator item. If the true expiry is 2026-07-12 it has ALREADY lapsed and the next `antmedia`
> restart = total ingest death. A session cannot resolve it (AMS creds operator-only; enforced only on
> restart). GHCR is still private (anon → 401).

## S44 candidates — pick by leverage (verify date + product-viability first)

1. **§2.7 CI job promotions** [S] — **do this IF the date is ≥ 2026-07-23.** Promote `web-e2e`/`csp-e2e` off
   `continue-on-error` in the workflow. Both have been GREEN the last three rounds (S41–S43); confirm stability
   across a few runs, then promote. A real, clean, high-value win once eligible. (If < 07-23, skip and note it.)
2. **e2e/unit coverage hardening** [S, test-only] — bounded, no ruling, no prod deploy. Candidates a session
   can verify are genuinely under-driven: the AuditLogPage error/empty-state paths in a browser (S41 added
   vitest + 3 e2e; check for gaps), or other flows the vitest suite covers but e2e does not. **Mutation-prove
   each** or don't ship it.
3. **Doc reconciliation** [XS–S] — reconcile any doc drift found at open (the AMS-expiry runbook line is
   operator-blocked, but other docs may have code-provable drift a session CAN fix — RUN the doc, don't read it).
4. **Operator-ruling items** — do NOT build without the ruling: audit-read model, BE-02 config, default
   `license_expiry` rule. If the operator has answered any in `operator-expected`/a commit since S43, THEN build it.

### BLOCKED — do not start without the operator

- **Audit-read gating / BE-02 config / default license_expiry rule** — product rulings (operator-expected ⚑ A/B + item).
- **Team-management UI** — BLOCKED on the operator product ruling (item 10).
- **§2.19 uipro Wave 2+** — OPERATOR-DIRECTED; `brandkit/` is the operator's (D-071, G7 not approved).

**Not candidates (DONE):** §2.20–§2.28 (S35–S43). Audit trail is feature-complete across every user-creation
path (S40/S42) and surfaced in the UI (S41).

## ⚠️ Binding lessons — carry into every wave

1. **★ Verify product-viability AND candidate-status before building** — S38 and S43 both overturned their leads.
2. **★ A gate with no test is not a gate; a green suite is not a working feature.** Mutation-prove every guard
   AND every e2e (S43). Pin wiring seams (D-062/D-101). Audit on the committed-write path, before any re-fetch (S40).
3. **★ Run an independent adversarial review before merge for non-trivial code** (S40 found a real defect; S42
   confirmed clean). For a test-only change already mutation-proven, the mutation proof is sufficient (S43).
4. **★ A new meta migration touches FIVE places**; **contract changes need `schema.d.ts` regenerated** (even
   description-only). **Audit the whole handler family** when you touch one.
5. **RUN the doc/path; do not read it.** Adversarially verify findings. **No silent scope caps** — and its
   inverse: **do not invent scope to look busy.** When the highest-leverage work is operator-gated, say so.
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
2. `decisions.md` **D-106** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-45; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-45.md` (carry the standing-directive header).
6. **Roll prod forward** ONLY if server/web *source* changed, per `deploy/runbooks/upgrade-rollback.md` —
   STAMPED build (`--build-arg` on `build`, then `up -d` WITHOUT `--build`); smoke with **evidence**. Skip for
   test/docs-only sessions (say so).
