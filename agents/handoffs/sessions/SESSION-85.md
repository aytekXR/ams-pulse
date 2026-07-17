# SESSION-85 — planned at S84 close (D-146) — SCALE BACK: the bounded backlog is exhausted

> Written by SESSION-84 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-98-g641b4e2** (hardened; report-artifact retention).
> **This is the low-frequency-wait phase.** Three consecutive quiet arcs (S82 checkpoint → S83 web coverage → S84
> doc-gaps) have drained the safe, bounded, operator-unscoped autonomous work. Do NOT manufacture a new arc — wait.

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
- Read §2.7's spec (ROADMAP-V2 §2.7, line ~168). Flip the soft/advisory CI jobs (web-e2e, csp-e2e, e2e, docker-build —
  confirm the exact set in the spec) from advisory to **required** so they gate merges — via workflow gating and/or the
  branch-protection required-status-checks list.
- **CAVEAT:** if the enforcing half needs GitHub **branch-protection** repo-admin settings (I cannot set these), that
  part is operator-gated (§2.1) — do the workflow-side changes I CAN make and surface the exact branch-protection
  settings to the operator in `operator-expected.md`. Don't claim it's done if the enforcing half needs the operator.
- Validate a test PR still gates correctly. Docs-close as usual. (A CI-config change does NOT roll prod.)

**B) IF the operator answered / named a priority → do their pick.** Verify status + viability against the code first,
then take the verified core. This is the highest-leverage path whenever it's available.

**C) ELSE (still < 07-23, no operator answer) → WAIT at low frequency. Do NOT manufacture an arc.**
- The safe bounded backlog is exhausted: §2.7 is date-gated; the 6 checkpoint decisions are operator-gated; assessment
  bugs are all fixed except BUG-009-tenant (needs operator-gated F6); the two lowest-covered web files (S83) and all 18
  documentation gaps (S84) are done. Web coverage on the *next* files down and server-side coverage exist but are
  diminishing-returns; a NEW arc here would be manufacturing work against the loop guidance.
- **Do a quick health check only** — `git status` clean (only Caddyfile); no open non-Dependabot PR needs attention;
  CI on `main` is green; date/operator unchanged — then **re-arm the loop at the max interval (3600s) and stop in one
  line.** (Dependabot PRs #69/#70/#153/etc. are deliberately operator-held — do NOT merge them autonomously, esp. the
  eslint 9→10 major which conflicts with the pinned `@eslint/js`.)
- Only if you spot a genuine *regression or newly-broken thing* (failing CI on main, a broken link, a real bug) → fix
  that (it's stewardship, not invention). Otherwise wait.

## Pipeline (only if you take A or B)
1. **Verify-at-open:** git clean (only Caddyfile). Date + operator check. Record **D-147 IN PROGRESS** in `decisions.md`.
   Branch `s85-d147`.
2. **Execute** the chosen lead. Contracts before code.
3. **Validate:** Go → mutation-prove + full 25-pkg suite; web → full `npm test` + build + typecheck + lint.
4. **Adversarial review** for any security-relevant change (a CI-gate change is low-risk; scale down).
5. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main. (Two-PR cadence: arc PR, then docs-close.)
6. **Roll prod forward** ONLY if server/web SOURCE changed (CI-config / docs / test-only does NOT). Stamped rebuild +
   5-check smoke if it does.
7. **Close docs:** D-147, CHANGELOG (if user-facing), ROADMAP, RESUME rotation (→ SESSION-86), `operator-expected.md`,
   SESSION-85 CLOSED, SESSION-86 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` NOT on host PATH. Mutation copy `/tmp/pulsemut`; restore via `cp` (NEVER `git checkout`,
  D-096). Node at `/home/aytek/.local/bin/node`; CI installs web with `npm ci --legacy-peer-deps`.
- **Web tests:** from `web/`, `npm test` (full suite for the coverage gate). `PATH="/home/aytek/.local/bin:$PATH" npx
  vitest run <files>` for scoped runs; `--coverage.include='src/features/xxx/**'` to measure a file. `vi.hoisted` when a
  `vi.mock` factory references a mock; replace `@/api/client` via `async (orig) => ({ ...await orig(), adminApi:{...} })`
  to keep the real `ApiError`. Token `created_at`/`last_used_at` are epoch-ms numbers.
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
