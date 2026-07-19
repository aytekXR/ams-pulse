# SESSION-94 — planned at S93 close (D-157) — LOW-FREQUENCY WAIT (stream_offline fix shipped; remaining work gated/tooling-blocked)

> Written by SESSION-93 close (2026-07-19). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146` (**this host IS
> prod**; no SSH). **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-124-g8eb3b57** (S93 prod-rolled the
> `stream_offline` alert fix; rollback tag `pre-d157`).
> **Back to the low-frequency wait.** The operator-requested arc (the D-156 `stream_offline` HIGH defect) is shipped
> (D-157). What remains is gated (date/operator) or tooling-blocked. Do NOT manufacture an arc — verify, then wait.

## ⚡ STANDING DIRECTIVE — carried
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move WHEN ONE EXISTS. When the only
remaining work is gated or tooling-blocked, **wait at low frequency** — don't invent work. Ultracode is on (apply to the
*quality* of real work). **Workflow gotcha:** no backticks in workflow prompt prose. `gofmt` is NOT on the host PATH — run
via docker (`docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server golang:1.25 gofmt -l .`).

## THE FIRST THING TO DO AT OPEN: the two-minute gate
1. **CHECK THE DATE.** `date +%Y-%m-%d`. §2.7 CI-promotion gate unlocks **≥ 2026-07-23** (at S93 close it was 07-19).
1b. **CHECK THE ANDROID TOOLCHAIN (standing GO D-154).** `command -v gradle && command -v java` (or `kotlinc`). If PRESENT →
   START `sdk/beacon-kotlin` (Lead B, ROADMAP §2.12), no operator prompt. If absent → "toolchain absent, waiting".
2. **CHECK `operator-expected.md`** — did the operator answer **[20]** (audit-read); ask for **iOS Phase 2**; or name a new
   priority? If yes → do their pick (Lead B).

## Lead — pick by state (priority order)
**A) IF today ≥ 2026-07-23 → §2.7 CI-promotions.** In `.github/workflows/ci.yml` drop `web-e2e`'s `continue-on-error:
true`; run `actionlint`. Surface the branch-protection required-status-checks **FULL-LIST PUT** to the operator (add `e2e`,
`csp-e2e`, `web-e2e`, `docker-build`, `sdk-swift` to the contexts) — repo-admin I cannot set (§2.1). A CI-config change does
NOT roll prod.

**B) IF the operator answered / provided tooling / named a priority → do their pick.**
- **§2.12 Android (STANDING GO):** build `sdk/beacon-kotlin` per ROADMAP §2.12 (Gradle Kotlin JVM lib mirroring
  `sdk/beacon-swift` + `sdk/beacon-js`; zero-dep; JUnit5; `sdk-kotlin` CI job). NO server change → NO prod roll.
- **[20]:** (a) keep-open = document + close; (b) gate-admin-reads at `handleListAuditLog` + contracts/tests/mutation/review
  + prod-roll.
- **§2.12 iOS Phase 2** (if an Apple/Xcode runner appears).

**C) ELSE (still < 07-23, no operator input) → VERIFY, then WAIT at low frequency. Do NOT manufacture an arc.**
- Quick health check: `git status` clean (only `Caddyfile.prod`); no open non-Dependabot PR needs attention; CI on `main`
  green; date/operator unchanged. (Dependabot PRs are operator-held — do NOT merge autonomously.)
- **★ Do NOT run another fresh "is anything broken?" sweep.** S89/S91/S92 have swept three times, the contract-drift class
  is drained, and S92's sweep already found + (D-157) fixed the one real HIGH defect it surfaced. A fourth sweep is
  manufacturing an arc. If you want to confirm health, a single read of CI/PR/date/operator suffices.
- **Do NOT start** iOS Phase 2 (Xcode/Apple CI) or Android (JDK/Gradle/Kotlin) — both tooling-blocked on this host.
- If none of A/B applies → **re-arm the loop at the max interval (3600s) / low frequency and stop in one line.**

## Pipeline (only if you take A or B)
1. Verify-at-open (git clean; date+operator). Record **D-158 IN PROGRESS**. Branch `s94-d158`.
2. Execute (contracts before code). 3. Validate: Go 26-pkg via docker (+ mutation-prove SOURCE); web full
   `npm test`/build/typecheck/lint; Swift/Kotlin build+test if an SDK is touched. 4. Adversarial review for
   security/contract-surface / state-machine changes. 5. PR → CI → squash-merge --delete-branch → verify origin/main.
   6. Roll prod ONLY if server/web SOURCE changed (SDK/CI/docs/test/contract-types-only does NOT). 7. Close docs: D-158,
   ROADMAP, RESUME → SESSION-95, operator-expected, SESSION-94 CLOSED, SESSION-95 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`.
  Mutation copy `/tmp/*.orig`; restore via `cp` (NEVER `git checkout`, D-096). Node at `/home/aytek/.local/bin` (v20.20.2).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. D-058 stamped build (build with `--build-arg VERSION=$(git
  describe --tags --always) COMMIT=... BUILD_DATE=...` FIRST, then `up -d pulse` WITHOUT `--build`). Tag a rollback image
  `pulse-prod-pulse:pre-dNNN` before rebuilding. prod **v0.4.0-124-g8eb3b57**. 5-check smoke: version, healthz 200, signed
  webhook 200 (unsigned 401), limits 512M/0.5cpu, 0 errors. Signed-webhook smoke: HMAC the body with `PULSE_WEBHOOK_SECRET`
  (openssl on host, secret captured via `docker exec … printenv`, not printed), POST an unknown-action payload (no state
  side-effect) from inside the container via busybox wget; exit 0 = 200.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>` (D-096;
  `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
