# SESSION-86 — planned at S85 close (D-147) — STILL THE LOW-FREQUENCY WAIT

> Written by SESSION-85 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-98-g641b4e2** (hardened; report-artifact retention).
> **This is the low-frequency-wait phase.** The bounded, operator-unscoped autonomous backlog is exhausted. S85's fix
> (D-147, OpenAPI `/reports/export` drift) was a *caught defect*, not a new work-stream — do NOT read it as "there's more
> to do." Do NOT manufacture an arc — verify, then wait.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — still in force

Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and **choose the next-highest-leverage move** when one exists. Verify
candidate status AND product-viability against the code before committing. Do NOT stop the loop — but when the only
remaining work is gated (date/operator) or is a large operator-unscoped work-stream, the correct move is to **wait at
low frequency**, not to invent work. **Ultracode is on** (apply it to the *quality* of real work, not to justify
manufacturing arcs). **Workflow gotcha:** no backticks in workflow prompt prose. `gofmt -l` before every push.

## THE FIRST THING TO DO AT OPEN: the two-minute gate

1. **CHECK THE DATE.** `date +%Y-%m-%d`. The §2.7 CI-promotion gate unlocks **≥ 2026-07-23**.
2. **CHECK `operator-expected.md`** — has the operator answered the checkpoint (F6, §2.6, §2.1, §2.18 GHCR/licence,
   §2.19, §2.12) or named any new priority? If yes → that is now the highest-leverage, operator-scoped move: DO IT.

## Lead — pick by state (in priority order)

**A) IF today ≥ 2026-07-23 → §2.7 CI-promotions (THE primary autonomous move; finally unlocked).**
- Read §2.7's spec (ROADMAP-V2 §2.7, line ~168). Flip the soft/advisory CI jobs from advisory to **required** so they
  gate merges. **Confirmed detail from S85:** in `.github/workflows/ci.yml` the ONLY advisory job is `web-e2e`
  (`continue-on-error: true` at ~line 330); `csp.spec.ts` runs *inside* `web-e2e` (there is no separate `csp-e2e` job in
  the file — the `csp-e2e` PR check comes from a different workflow). Drop `continue-on-error`, run `actionlint`.
- **CAVEAT:** the enforcing half — the GitHub **branch-protection** required-status-checks FULL-LIST PUT (a partial list
  silently de-requires the rest: contracts, server, web, sdk, docker-build, helm, compose + web-e2e + csp-e2e) — needs
  repo-admin I cannot set (§2.1). Do the workflow-side edit I CAN make, and surface the exact PUT payload to the operator
  in `operator-expected.md`. Don't claim §2.7 complete if the enforcing half still needs the operator.
- Validate a test PR still gates correctly. Docs-close as usual. (A CI-config change does NOT roll prod.)

**B) IF the operator answered / named a priority → do their pick.** Verify status + viability against the code first,
then take the verified core. This is the highest-leverage path whenever it's available.

**C) ELSE (still < 07-23, no operator answer) → VERIFY, then WAIT at low frequency. Do NOT manufacture an arc.**
- Quick health check: `git status` clean (only `Caddyfile.prod`); no open non-Dependabot PR needs attention; CI on
  `main` green; date/operator unchanged. (Dependabot PRs #69/#70/#153/etc. are deliberately operator-held — do NOT merge
  autonomously, esp. the eslint 9→10 major which conflicts with the pinned `@eslint/js`.)
- **Optional (at most ONE):** a bounded adversarial "is anything genuinely broken?" sweep like S85's (roadmap-status /
  stewardship / contract-drift scouts + a strict judge). This is *verification*, not arc-manufacturing — its job is to
  either (a) surface a **real, non-gated defect** (broken link, contract drift, build breakage, a bug) → fix that
  (stewardship, one-off), or (b) confirm nothing is broken → **wait.** Keep the bar high: web-coverage nudges,
  doc-completeness on already-complete docs, and cosmetic churn are BUSYWORK — dismiss them (the judge did this at S85).
- **Known low-priority, non-gated stewardship candidates a sweep may re-surface** (S85 deferred these deliberately — take
  them ONLY if you judge them worth a clean arc, else leave them): the CHANGELOG `[0.4.0]` gap (faithful reconstruction
  of the 0.3.0→0.4.0 change set — judgment-heavy) and the `VERSION` file `0.1.0` staleness (cosmetic; nothing reads it —
  the build uses `git describe`). Neither is a correctness/behavior defect.
- If neither A/B nor a genuine defect exists → **re-arm the loop at the max interval (3600s) / low frequency and stop in
  one line.** No manufactured work.

## Pipeline (only if you take A, B, or a caught-defect fix under C)
1. **Verify-at-open:** git clean (only Caddyfile). Date + operator check. Record **D-148 IN PROGRESS** in `decisions.md`
   (or at close, single-flow). Branch `s86-d148`.
2. **Execute** the chosen lead. Contracts before code.
3. **Validate:** Go → full 25-pkg suite via docker (+ mutation-prove any SOURCE change); web → full `npm test`/`build`/
   `typecheck`/`lint`. A contract/test/types-only change (like D-147) needs no adversarial review; scale to risk.
4. **Adversarial review** for any security-relevant SOURCE change.
5. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main. (Two-PR cadence: arc PR, then docs-close.)
6. **Roll prod forward** ONLY if server/web SOURCE changed (CI-config / docs / contract / test / generated-types does
   NOT). Stamped rebuild + 5-check smoke if it does.
7. **Close docs:** D-148, CHANGELOG (if user-facing), ROADMAP, RESUME rotation (→ SESSION-87), `operator-expected.md`,
   SESSION-86 CLOSED, SESSION-87 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` NOT on host PATH. Mutation copy `/tmp/pulsemut`; restore via `cp` (NEVER `git checkout`,
  D-096). Node at `/home/aytek/.local/bin/node`; CI installs web with `npm ci --legacy-peer-deps`.
- **Web tests:** from `web/`, `npm test` (full suite for the coverage gate); `npm run typecheck && npm run lint &&
  npm run build`; **`npm run gen:api`** regenerates `src/lib/api/schema.d.ts` from the OpenAPI spec (openapi-typescript).
  `PATH="/home/aytek/.local/bin:$PATH"` needed for node.
- **Contract/conformance:** `contracts/openapi/pulse-api.yaml` is validated by `openapi_conformance_test.go`
  (`doc.Validate` on load — a malformed edit fatally fails the api pkg). Adding a query param REQUIRES a
  `param_conformance_test.go` registry entry keyed `"METHOD /path ?name"` (probe/exempt/known-violation) AND bumping the
  `minSpecParams` / `minProbes` non-vacuity floors — see D-147 for the pattern.
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod **v0.4.0-98-g641b4e2**; rollback `pulse-prod-pulse:pre-d143`.
  Container runs read-only rootfs — new server-side writes must target `/var/lib/pulse` or `/tmp`. 5-check smoke:
  version stamp, healthz 200, signed webhook 200 (HMAC from PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
