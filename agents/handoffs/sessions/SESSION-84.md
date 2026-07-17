# SESSION-84 — planned at S83 close (D-145) — the §2.7 date-gate approaches; else quiet

> Written by SESSION-83 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-98-g641b4e2** (hardened: read-only rootfs, cap_drop,
> report-artifact retention). S82 was an operator checkpoint; S83 was a test-only coverage arc — the autonomous
> backlog is thin and the next headline move (§2.7) is date-gated to **2026-07-23**.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-85

Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and **choose the next-highest-leverage move.** Verify candidate status AND
product-viability against the code before committing. Take the verified CORE. Do NOT stop after one session — at close,
update all docs, record progress + operator-needs, continue until the roadmap is complete or a human/operator is
genuinely required. **Ultracode is on.** **Workflow-script gotcha:** no backticks in workflow prompt prose. `gofmt -l`
before every push; web gotchas (`vi.hoisted` for `vi.mock`; full `npm test` for the coverage gate; `ApiError(status,
{code, message})`; binary embeds `web/dist`; `created_at`/`last_used_at` are epoch-ms **numbers**, not ISO strings).

## THE FIRST THING TO DO AT OPEN: check the date + check for operator response (a two-minute gate)

1. **CHECK THE DATE.** `date +%Y-%m-%d`. The §2.7 CI-promotion gate unlocks **≥ 2026-07-23**.
2. **CHECK `operator-expected.md`** — has the operator responded to the S82 checkpoint (F6, §2.6, §2.1, §2.18 GHCR/licence,
   §2.19, §2.12)? If they picked something, DO THAT (operator-scoped → highest leverage). If not, continue below.

## Lead — pick by the date/operator state

**A) IF today ≥ 2026-07-23 → §2.7 CI-promotions (the primary autonomous move).** Bounded, autonomous, high-signal.
- Read §2.7's spec (ROADMAP-V2 line ~168). Flip the soft/advisory CI jobs (web-e2e, csp-e2e, e2e, docker-build — confirm
  the exact set in the spec) from advisory to **required** so they gate merges (workflow gating and/or the
  branch-protection required-status-checks list).
- **CAVEAT:** if the promotion needs GitHub **branch-protection** repo-admin settings (which I cannot change), that half
  is operator-gated (§2.1) — do the workflow-side changes I CAN make, and surface the exact branch-protection settings to
  the operator in `operator-expected.md`. Don't claim it's done if the enforcing half needs the operator.
- Validate a test PR still gates correctly. Docs-close as usual. (A CI-config change does NOT roll prod.)

**B) IF the operator answered the checkpoint → do their pick.** Now scoped by them → the highest-leverage move. Verify
status/viability against the code first (as always), then take the verified core.

**C) IF today < 07-23 AND no operator answer → this is the 2nd→3rd consecutive quiet arc; be honest.** Prefer ONE more
small, unobjectionable arc over idling, then scale back:
- **`docs/assessment/documentation-gaps.md` completeness pass** — close concrete documented doc gaps (operator runbooks,
  API docs). Autonomous, GA-relevant. **Verify each gap is still open before writing** (several may already be closed).
- **Or** another small web/server test-coverage arc if a genuinely low-covered, meaningful file remains (S83 already
  took the two lowest web files; check the coverage report before assuming there's a worthwhile target).
- **Then scale the loop to a low-frequency wait** for the 07-23 gate / operator input (loop guidance: after ~3 no-op
  ticks, reduce frequency rather than manufacturing work). Do NOT start a large operator-unscoped work-stream
  (NOT F6, NOT §2.19, NOT §2.12).

## Pipeline (the standard loop)
1. **Verify-at-open:** git state clean (only Caddyfile). Check the date + operator-expected. Record **D-146 IN PROGRESS**
   in `decisions.md` (only if you take a code/docs arc). Branch `s84-d146`.
2. **Execute** the chosen lead. Contracts before code.
3. **Validate:** Go → mutation-prove + full 25-pkg suite; web → full `npm test` + build + typecheck + lint.
4. **Adversarial review** for any security-relevant change (a CI-gate, docs, or test-only change is low-risk; scale down).
5. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main. (Two-PR cadence: code/arc PR, then docs-close PR.)
6. **Roll prod forward** ONLY if server/web SOURCE changed (a CI-config, test-only, or docs change does NOT). Stamped
   rebuild + 5-check smoke if it does.
7. **Close docs:** D-146, CHANGELOG (if user-facing), ROADMAP, RESUME rotation (→ SESSION-85), `operator-expected.md`,
   SESSION-84 CLOSED, SESSION-85 written. Re-arm the `/loop` (longer interval as the backlog is quiet).

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` NOT on host PATH. Mutation copy `/tmp/pulsemut`; restore via `cp` (NEVER `git checkout`,
  D-096). Node at `/home/aytek/.local/bin/node`; CI installs web with `npm ci --legacy-peer-deps`.
- **Web tests:** from `web/`, `npm test` (full suite for the coverage gate; thresholds ~59% lines / 45% funcs / 54%
  branch global). `PATH="/home/aytek/.local/bin:$PATH" npx vitest run <files>` for a scoped run;
  `--coverage.include='src/features/xxx/**'` to measure a file. `vi.hoisted` when a `vi.mock` factory references a mock.
  Replace `@/api/client` via `async (orig) => ({ ...await orig(), adminApi: {...} })` to keep the real `ApiError`.
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
