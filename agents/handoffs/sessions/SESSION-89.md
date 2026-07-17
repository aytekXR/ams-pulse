# SESSION-89 — planned at S88 close (D-150) — LOW-FREQUENCY WAIT (F6 code complete; remaining work is gated)

> Written by SESSION-88 close (2026-07-18). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`
> (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-114-ge295795** (F6 Phase 1+2 live).
> **This is the low-frequency-wait phase.** The operator's "start F6" is dispositioned: BUG-009 ✅ (D-148), [5] ✅ (D-149),
> [20] = operator product call (D-150). The safe, bounded, operator-unscoped autonomous backlog is drained again. Do NOT
> manufacture an arc — verify, then wait.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — still in force
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move WHEN ONE EXISTS. When the only
remaining work is gated (date/operator) or a large operator-unscoped work-stream, the correct move is to **wait at low
frequency**, not invent work. **Ultracode is on** (apply to the *quality* of real work, not to justify manufacturing
arcs). **Workflow gotcha:** no backticks in workflow prompt prose. `gofmt -l` before every push.

## THE FIRST THING TO DO AT OPEN: the two-minute gate
1. **CHECK THE DATE.** `date +%Y-%m-%d`. The §2.7 CI-promotion gate unlocks **≥ 2026-07-23** (at S88 close it was 07-18).
2. **CHECK `operator-expected.md`** — has the operator answered **[20] the audit-read model** ((a) keep reads open vs (b)
   gate the admin-read surface) or any other checkpoint item, or named a new priority? If yes → that is now the
   highest-leverage move: DO IT (Lead B).

## Lead — pick by state (priority order)
**A) IF today ≥ 2026-07-23 → §2.7 CI-promotions (THE primary autonomous move; finally unlocked).**
Read §2.7's spec (ROADMAP-V2 §2.7). In `.github/workflows/ci.yml` the ONLY advisory job is `web-e2e`
(`continue-on-error: true`; `csp.spec.ts` runs inside it — there is no separate `csp-e2e` job in the file). Drop the flag,
run `actionlint`. **CAVEAT:** the enforcing half — the GitHub branch-protection required-status-checks FULL-LIST PUT —
needs repo-admin I cannot set (§2.1); do the workflow-side edit I CAN make and surface the exact PUT payload to the
operator. Don't claim §2.7 done if the enforcing half needs the operator. (A CI-config change does NOT roll prod.)

**B) IF the operator answered / named a priority → do their pick.** For **[20]**: (a) keep-open = document + close (no
code); (b) gate-admin-reads = add the `canWrite` check at the top of `handleListAuditLog` (audit.go:92) AND decide whether
to gate the whole admin-read surface (users/tokens/audit) consistently — contracts/tests/mutation/adversarial-review +
prod-roll. Verify status + viability against the code first.

**C) ELSE (still < 07-23, no operator answer) → VERIFY, then WAIT at low frequency. Do NOT manufacture an arc.**
- Quick health check: `git status` clean (only `Caddyfile.prod`); no open non-Dependabot PR needs attention; CI on `main`
  green; date/operator unchanged. (Dependabot PRs #69/#70/#153/etc. are operator-held — do NOT merge autonomously,
  esp. the eslint 9→10 major which conflicts with the pinned `@eslint/js`.)
- **Optional (at most ONE):** a bounded adversarial "is anything genuinely broken?" sweep (roadmap-status / stewardship /
  contract-drift — like S85's). Its job: surface a **real, non-gated defect** (broken link, contract drift, build
  breakage, a bug) → fix it (stewardship, one-off), or confirm nothing is broken → **wait.** Keep the bar high:
  web-coverage nudges / doc-completeness on already-complete docs = busywork → dismiss.
- Do NOT start the deeper F6 expansion (tenant-scoped AUTH — `APIToken` has no tenant field; a tenant-management web UI —
  §2.19 territory) autonomously; both are large + operator-scoped + demand-driven.
- If neither A/B nor a genuine defect → **re-arm the loop at the max interval (3600s) / low frequency and stop in one
  line.** No manufactured work.

## Pipeline (only if you take A or B or a caught-defect fix under C)
1. Verify-at-open (git clean; date+operator). Record **D-151 IN PROGRESS**. Branch `s89-d151`.
2. Execute (contracts before code). 3. Validate: Go full 25-pkg suite via docker (+ mutation-prove SOURCE changes); web
   full `npm test`/build/typecheck/lint. 4. Adversarial review for security-relevant changes. 5. PR → CI →
   squash-merge --delete-branch → verify origin/main. 6. Roll prod ONLY if server/web SOURCE changed (CI-config/docs/
   test-only does NOT). 7. Close docs: D-151, ROADMAP, RESUME → SESSION-90, operator-expected, SESSION-89 CLOSED,
   SESSION-90 written. Re-arm the `/loop`.

## Environment gotchas (carried — unchanged)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`.
  `gofmt`/`go` NOT on host PATH. Mutation copy `/tmp/pulsemut`; restore via `cp` (NEVER `git checkout`, D-096).
  Node at `/home/aytek/.local/bin/node`. Reusable: `internal/tenant` (Matcher + CachedResolver).
- **Web:** from `web/`, `npm test`; `npm run gen:api` regenerates `src/lib/api/schema.d.ts`. A new query param needs a
  `param_conformance_test.go` registry entry + floor bumps (`minSpecParams`, `minProbes` — D-147 pattern).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. **D-058 stamped build:** `docker compose $DC build
  --build-arg VERSION=$(git describe --tags --always) --build-arg COMMIT=$(git rev-parse --short HEAD) --build-arg
  BUILD_DATE=$(date -u ...) pulse` THEN `docker compose $DC up -d pulse` (never mix `--build` into `up -d`). Prod
  **v0.4.0-114-ge295795**; rollback tags `pulse-prod-pulse:pre-d148[-fix]` / `:pre-d149`. Read-only rootfs — new writes →
  `/var/lib/pulse` or `/tmp`. 5-check smoke: version stamp, healthz 200, signed webhook 200 (HMAC from
  PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines. Admin token (side-effect-free GET only, never commit):
  gitignored `oguz-testing.md`; API base `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
