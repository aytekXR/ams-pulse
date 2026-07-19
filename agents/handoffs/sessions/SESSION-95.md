# SESSION-95 — planned at S94 close (D-158) — LOW-FREQUENCY WAIT (load lane + panel-revamp assessment shipped; remaining work gated/tooling-blocked)

> Written by SESSION-94 close (2026-07-19). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146` (**this host IS
> prod**; no SSH). **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-124-g8eb3b57** (unchanged — S94 was docs +
> QA-tooling only: the opt-in load lane + the Ant Media panel-revamp/G-27 assessment; no server/web source → no prod roll).
> **Back to the low-frequency wait.** The operator's two-part mid-session ask (panel assessment + load-testing lane) is
> delivered (D-158). What remains is gated (date/operator) or tooling-blocked. Do NOT manufacture an arc — verify, then wait.

## ⚡ STANDING DIRECTIVE — carried
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move WHEN ONE EXISTS. When the only
remaining work is gated or tooling-blocked, **wait at low frequency** — don't invent work. Ultracode is on (apply to the
*quality* of real work). **Workflow gotcha:** no backticks in workflow prompt prose. `gofmt`/`shellcheck` notes: `shellcheck`
IS on the host PATH (`/usr/bin/shellcheck`); `gofmt` is NOT — run via docker (`docker run --rm -v
/home/aytek/repo/ams-pulse:/repo -w /repo/server golang:1.25 gofmt -l .`).

## THE FIRST THING TO DO AT OPEN: the two-minute gate
1. **CHECK THE DATE.** `date +%Y-%m-%d`. §2.7 CI-promotion gate unlocks **≥ 2026-07-23** (at S94 close it was 07-19).
1b. **CHECK THE ANDROID TOOLCHAIN (standing GO D-154).** `command -v gradle && command -v java` (or `kotlinc`). If PRESENT →
   START `sdk/beacon-kotlin` (Lead B, ROADMAP §2.12), no operator prompt. If absent → "toolchain absent, waiting".
2. **CHECK `operator-expected.md`** — did the operator: answer **[20]** (audit-read); ask for **iOS Phase 2**; **run the load
   lane** and hand back a capacity number to fold into `docs/compatibility.md`; report back from the **panel developer
   meeting** (G-27 confirmations — may unlock an `amsclient` fix if REST re-versioning / cluster-pagination is confirmed); or
   name a new priority? If yes → do their pick (Lead B).

## Lead — pick by state (priority order)
**A) IF today ≥ 2026-07-23 → §2.7 CI-promotions.** In `.github/workflows/ci.yml` drop `web-e2e`'s `continue-on-error:
true`; run `actionlint`. Surface the branch-protection required-status-checks **FULL-LIST PUT** to the operator (add `e2e`,
`csp-e2e`, `web-e2e`, `docker-build`, `sdk-swift` to the contexts) — repo-admin I cannot set (§2.1). A CI-config change does
NOT roll prod.

**B) IF the operator answered / provided tooling / named a priority → do their pick.**
- **§2.12 Android (STANDING GO):** build `sdk/beacon-kotlin` per ROADMAP §2.12 (Gradle Kotlin JVM lib mirroring
  `sdk/beacon-swift` + `sdk/beacon-js`; zero-dep; JUnit5; `sdk-kotlin` CI job). NO server change → NO prod roll.
- **Load-lane capacity result:** if the operator ran `qa/realams/run-load-suite.sh` on a dedicated AMS and reported a
  `LOAD-REPORT.md` / capacity number → record the headline in `docs/compatibility.md` "Capacity and load validation" (the
  TBD row) and attach it to the marketplace-readiness notes. Docs-only.
- **G-27 panel confirmations:** if the meeting settled the 3 questions (REST v2 survives vs v2→v3; new auth flow; cluster
  `nodes` flat-array vs paginated / the G-21 claim) → act on the answer (e.g. a scoped `amsclient` + mock-ams pagination fix
  ONLY if a live cluster confirms G-21 — server SOURCE → prod roll; otherwise a docs update to the G-27 section).
- **[20]:** (a) keep-open = document + close; (b) gate-admin-reads at `handleListAuditLog` + contracts/tests/mutation/review
  + prod-roll.
- **§2.12 iOS Phase 2** (if an Apple/Xcode runner appears).

**C) ELSE (still < 07-23, no operator input) → VERIFY, then WAIT at low frequency. Do NOT manufacture an arc.**
- Quick health check: `git status` clean (only `Caddyfile.prod` expected modified); no open non-Dependabot PR needs
  attention; CI on `main` green; date/operator unchanged. (Dependabot PRs are operator-held — do NOT merge autonomously.)
- **★ Do NOT run another fresh "is anything broken?" sweep.** S89/S91/S92 have swept three times, the contract-drift class
  is drained, and S92's sweep already found + (D-157) fixed the one real HIGH defect it surfaced. A fresh sweep is
  manufacturing an arc. If you want to confirm health, a single read of CI/PR/date/operator suffices.
- **Do NOT start** iOS Phase 2 (Xcode/Apple CI) or Android (JDK/Gradle/Kotlin) — both tooling-blocked on this host. Do NOT
  RUN the load lane autonomously — it needs a dedicated PAYG AMS the operator provisions (and it hard-aborts on the shared
  VPS by design).
- If none of A/B applies → **re-arm the loop at the max interval (3600s) / low frequency and stop in one line.**

## Pipeline (only if you take A or B)
1. Verify-at-open (git clean; date+operator). Record **D-159 IN PROGRESS**. Branch `s95-d159`.
2. Execute (contracts before code). 3. Validate: Go 26-pkg via docker (+ mutation-prove SOURCE); web full
   `npm test`/build/typecheck/lint; Swift/Kotlin build+test if an SDK is touched; `bash -n` + `shellcheck` if QA scripts.
   4. Adversarial review for security/contract-surface / state-machine changes. 5. PR → CI → squash-merge --delete-branch →
   verify origin/main. 6. Roll prod ONLY if server/web SOURCE changed (SDK/CI/docs/test/contract-types-only does NOT).
   7. Close docs: D-159, ROADMAP, RESUME → SESSION-96, operator-expected, SESSION-95 CLOSED, SESSION-96 written. Re-arm the
   `/loop`.

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
- **Load lane (S94/D-158):** OPT-IN, operator-run on a DEDICATED instance ONLY. It sources `qa/realams/harness/load-env.sh`
  (gitignored; template `load-env.sh.example`), NEVER `env.sh`; hard-aborts on a forbidden host. Do NOT run it here (no
  dedicated AMS; the sandbox is egress-blocked). Phase 45 of `run-full-e2e.sh` SKIPs 77 when unconfigured.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>` (D-096;
  `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
