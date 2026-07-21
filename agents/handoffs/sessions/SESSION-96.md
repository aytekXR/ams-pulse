# SESSION-96 — planned at S95 close (D-159) — LOW-FREQUENCY WAIT (the D-157/D-158 delta is now swept; remaining work gated/tooling-blocked)

> Written by SESSION-95 close (2026-07-21). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146` (**this host IS
> prod**; no SSH). **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-129-g30717fc** (S95 prod-rolled the D-159
> alert-evaluator fix; rollback tag `pre-d159`). **Back to the low-frequency wait.** S95's arc — an independent adversarial
> re-verification of the never-independently-swept D-157 (stream_offline) + D-158 (load lane) delta — is delivered (6 of 7
> confirmed defects fixed, 1 deferred to §2.43). Do NOT manufacture an arc — verify at the gate, then wait or take §2.43.

## ⚡ STANDING DIRECTIVE — carried
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move WHEN ONE EXISTS. When the only
remaining work is gated or tooling-blocked, **wait at low frequency** — don't invent work. Ultracode is on (apply to the
*quality* of real work). **Workflow gotcha:** no backticks in workflow prompt prose. **Env:** `shellcheck` IS on the host
PATH (`/usr/bin/shellcheck`); `gofmt` is NOT — run via docker (`docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w
/repo/server golang:1.25 gofmt -l .`). Go tests run only in docker (incantation below). This host IS prod — direct `docker`
access works (no `sg docker` wrapper needed in this session; the runbook's `sg docker -c` form also works).

## THE FIRST THING TO DO AT OPEN: the two-minute gate
1. **CHECK THE DATE.** `date +%Y-%m-%d`. §2.7 CI-promotion gate unlocks **≥ 2026-07-23** (at S95 close it was 07-21 — so S96
   is **likely at/after the unlock**; if so, take Lead A).
1b. **CHECK THE ANDROID TOOLCHAIN (standing GO D-154).** `command -v gradle && command -v java` (or `kotlinc`). If PRESENT →
   START `sdk/beacon-kotlin` (Lead B, ROADMAP §2.12), no operator prompt. If absent → "toolchain absent, waiting".
2. **CHECK `operator-expected.md`** — did the operator: answer **[20]**; ask for **iOS Phase 2**; **run the load lane** and
   hand back a capacity number (→ `docs/compatibility.md`); report **panel-meeting G-27** answers (may unlock a scoped
   `amsclient` fix IF a live cluster confirms G-21); or name a new priority? If yes → do their pick (Lead B).

## Lead — pick by state (priority order)
**A) IF today ≥ 2026-07-23 → §2.7 CI-promotions.** In `.github/workflows/ci.yml` drop `web-e2e`'s `continue-on-error: true`;
run `actionlint`. Surface the branch-protection required-status-checks **FULL-LIST PUT** to the operator (add `e2e`,
`csp-e2e`, `web-e2e`, `docker-build`, `sdk-swift`) — repo-admin I cannot set (§2.1). A CI-config change does NOT roll prod.

**B) IF the operator answered / provided tooling / named a priority → do their pick.** (§2.12 Android STANDING GO;
load-lane capacity result → docs; G-27 panel confirmations; [20]; iOS Phase 2.)

**C) ELSE (still < 07-23, no operator input) → VERIFY, then WAIT — or take the one sanctioned non-gated arc.**
- Quick health check: `git status` clean; no open non-Dependabot PR needs attention; CI on `main` green; date/operator
  unchanged. (Dependabot PRs are operator-held — do NOT merge autonomously.)
- **★ Do NOT run another fresh "is anything broken?" sweep.** S95 swept the last un-swept delta (D-157/D-158) and drained
  it; S89/S91/S92 swept the rest 3×. A fresh sweep is manufacturing an arc.
- **The ONE sanctioned non-gated arc IF you want real work: ROADMAP §2.43 — the alert `e.states` unbounded-growth fix.**
  This is a genuine, pre-identified, non-gated internal defect (found S95/D-159 #5; deferred as too broad to bundle into a
  critical-alert PR). It touches the shared firing-state machine across ALL alert metrics, so it needs care: a correct fix
  must NOT drop `cooldownUntil` (evicting a "resolved" entry too early re-enables flapping alerts). Design a bounded sweep of
  terminal entries whose cooldown has expired (or a per-(rule,stream) TTL), mutation-prove it does NOT change fire/resolve/
  cooldown behavior, adversarial-review, server rebuild + prod roll (evaluator is server SOURCE). Scope it as a bounded
  increment. **If you take it, it is real high-value stewardship — not a manufactured arc.**
- **Do NOT start** iOS Phase 2 (Xcode/Apple CI) or Android (JDK/Gradle/Kotlin) — both tooling-blocked. Do NOT RUN the load
  lane autonomously — it needs a dedicated PAYG AMS the operator provisions (and it hard-aborts on the shared VPS by design,
  now also by raw IP — D-159).
- If none of A/B and you don't take §2.43 → **re-arm the loop at the max interval (3600s) / low frequency and stop in one
  line.**

## Pipeline (only if you take A / B / §2.43)
1. Verify-at-open (git clean; date+operator). Record **D-160 IN PROGRESS**. Branch `s96-d160`.
2. Execute (contracts before code). 3. Validate: Go 26-pkg via docker (+ mutation-prove SOURCE); web full
   `npm test`/build/typecheck/lint if web touched; `bash -n` + `shellcheck` if QA scripts. 4. Adversarial review for
   security/contract-surface / state-machine changes. 5. PR → CI → squash-merge --delete-branch → verify origin/main. 6. Roll
   prod ONLY if server/web SOURCE changed (§2.43 = evaluator SOURCE → roll; CI/docs-only does NOT). 7. Close docs: D-160,
   ROADMAP, RESUME → SESSION-97, operator-expected, SESSION-96 CLOSED, SESSION-97 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`.
  Mutation copy `/tmp/*.orig`; restore via `cp` (NEVER `git checkout`, D-096).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. D-058 stamped build (build with `--build-arg VERSION=$(git
  describe --tags --always) COMMIT=... BUILD_DATE=...` FIRST, then `up -d pulse` WITHOUT `--build`). Tag a rollback image
  `pulse-prod-pulse:pre-dNNN` before rebuilding. prod **v0.4.0-129-g30717fc**. 5-check smoke: version (startup log
  `"version":"vX"`), healthz 200, signed webhook 200 (unsigned 401 — `X-Ams-Signature: sha256=$(printf '%s' "$BODY" | openssl
  dgst -sha256 -hmac "$SECRET" -hex | sed 's/.* //')`, secret via `docker exec … printenv PULSE_WEBHOOK_SECRET`, via
  `https://beyondkaira.com/webhook/ams` with `--resolve beyondkaira.com:443:161.97.172.146`, unknown-action body = no side
  effect), limits 512M/0.5cpu (`docker inspect pulse-prod-pulse-1`), 0 errors/0 restarts.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>` (D-096;
  `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
