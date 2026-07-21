# SESSION-97 — planned at S96 close (D-160) — LOW-FREQUENCY WAIT (non-gated backlog drained again; §2.7 unlocks 2026-07-23)

> Written by SESSION-96 close (2026-07-21). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146` (**this host IS
> prod**; no SSH). **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-131-g6b5bd38** (S96 prod-rolled the D-160
> `e.states` eviction; rollback tag `pre-d160`). **The low-frequency wait.** S95 swept the un-swept D-157/D-158 delta; S96
> closed §2.43 (the `e.states` growth fix). The concrete non-gated backlog is DRAINED AGAIN — what remains is gated
> (date/tooling/operator) or the `[FO-1]` firing-orphan product call. Do NOT manufacture an arc — verify at the gate, then wait.

## ⚡ STANDING DIRECTIVE — carried
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move WHEN ONE EXISTS. When the only
remaining work is gated / tooling-blocked / a product call, **wait at low frequency** — don't invent work. Ultracode is on
(apply to the *quality* of real work). **Workflow gotcha:** no backticks in workflow prompt prose. `shellcheck` IS on PATH;
`gofmt` NOT (docker: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server golang:1.25 gofmt -l .`). Go tests
only in docker. This host IS prod — direct `docker` access works.

## THE FIRST THING TO DO AT OPEN: the two-minute gate
1. **CHECK THE DATE.** `date +%Y-%m-%d`. §2.7 CI-promotion gate unlocks **≥ 2026-07-23** (at S96 close it was 07-21 — so S97
   is **likely at/after the unlock**; if so, take Lead A).
1b. **CHECK THE ANDROID TOOLCHAIN (standing GO D-154).** `command -v gradle && command -v java` (or `kotlinc`). If PRESENT →
   START `sdk/beacon-kotlin` (Lead B, ROADMAP §2.12), no operator prompt. If absent → "toolchain absent, waiting".
2. **CHECK `operator-expected.md`** — did the operator: answer **[FO-1]** (firing-orphan resolution: auto-resolve-after-grace
   / stay-firing / leave-as-is); answer **[20]**; ask for **iOS Phase 2**; **run the load lane** + hand back a capacity number;
   report **panel-meeting G-27** answers; or name a new priority? If yes → do their pick (Lead B).

## Lead — pick by state (priority order)
**A) IF today ≥ 2026-07-23 → §2.7 CI-promotions.** In `.github/workflows/ci.yml` drop `web-e2e`'s `continue-on-error: true`;
run `actionlint`. Surface the branch-protection required-status-checks **FULL-LIST PUT** to the operator (add `e2e`,
`csp-e2e`, `web-e2e`, `docker-build`, `sdk-swift`). CI-config change does NOT roll prod.

**B) IF the operator answered / provided tooling / named a priority → do their pick.**
- **[FO-1] firing-orphan (if answered):** build the chosen resolution. If auto-resolve-after-grace: a per-metric stale-firing
  sweep that resolves a firing alert whose groupKey has been ABSENT for a grace window — but it MUST NOT touch `stream_offline`
  (absence IS its alert; D-157/D-159 own that path). Contract-check (does a resolve notification for a vanished source need a
  value?), mutation-prove fire/resolve/no-false-resolve, adversarial-review (a wrong sweep re-introduces the D-156 stuck/flap
  class), server rebuild + prod roll. This is a real feature with a semantics dimension — scope as a bounded increment.
- §2.12 Android (STANDING GO); load-lane capacity result → docs; G-27 panel confirmations; [20].

**C) ELSE (still < 07-23, no operator input) → VERIFY, then WAIT. Do NOT manufacture an arc.**
- Quick health check: `git status` clean; no open non-Dependabot PR needs attention; CI on `main` green; date/operator
  unchanged. (Dependabot PRs are operator-held — do NOT merge autonomously.)
- **★ Do NOT run another fresh "is anything broken?" sweep.** S89/S91/S92 swept 3×; S95 swept the last un-swept delta
  (D-157/D-158); S96 drained §2.43. The non-gated backlog is empty and the only remaining internal item (`[FO-1]`) is a
  product call. A fresh sweep is manufacturing an arc.
- **Do NOT start** iOS Phase 2 (Xcode/Apple CI) or Android (JDK/Gradle/Kotlin) — tooling-blocked. Do NOT RUN the load lane
  autonomously (needs a dedicated PAYG AMS). Do NOT force-build `[FO-1]` (needs the operator's firing-semantics call).
- If none of A/B applies → **re-arm the loop at the max interval (3600s) / low frequency and stop in one line.**

## Pipeline (only if you take A / B)
1. Verify-at-open (git clean; date+operator). Record **D-161 IN PROGRESS**. Branch `s97-d161`.
2. Execute (contracts before code). 3. Validate: Go 26-pkg via docker (+ mutation-prove SOURCE + `-race` for state-machine
   changes); web full suite if web touched; `bash -n` + `shellcheck` if QA scripts. 4. Adversarial review for
   security/contract-surface / state-machine changes (MANDATORY for `[FO-1]` — it is a live critical-alert path). 5. PR → CI →
   squash-merge --delete-branch → verify origin/main. 6. Roll prod ONLY if server/web SOURCE changed. 7. Close docs: D-161,
   ROADMAP, RESUME → SESSION-98, operator-expected, SESSION-97 CLOSED, SESSION-98 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`. Add
  `-race` for state-machine changes. Mutation copy `/tmp/*.orig`; restore via `cp` (NEVER `git checkout`, D-096).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. D-058 stamped build (build `--build-arg VERSION=$(git describe
  --tags --always) COMMIT=... BUILD_DATE=...` FIRST, then `up -d pulse` WITHOUT `--build`). Tag `pulse-prod-pulse:pre-dNNN`
  before rebuilding. prod **v0.4.0-131-g6b5bd38**. 5-check smoke: version (startup log `"version":"vX"`), healthz 200, signed
  webhook 200 / unsigned 401 (`X-Ams-Signature: sha256=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | sed
  's/.* //')`, secret via `docker exec … printenv PULSE_WEBHOOK_SECRET`, POST to `https://beyondkaira.com/webhook/ams` with
  `--resolve beyondkaira.com:443:161.97.172.146`, unknown-action body), limits 512M/0.5cpu, 0 errors/0 restarts.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>` (D-096;
  `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
