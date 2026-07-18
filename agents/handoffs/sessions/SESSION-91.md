# SESSION-91 — planned at S90 close (D-153) — LOW-FREQUENCY WAIT (iOS SDK Phase 1 shipped; remaining work gated/tooling-blocked)

> Written by SESSION-90 close (2026-07-18). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146` (**this host IS
> prod**; no SSH). **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-119** (unchanged — S90 shipped an SDK, a
> client library; no server change, no prod roll).
> **Back to the low-frequency wait.** The operator's decision menu is resolved (D-152) and the one buildable green-lit item
> — the iOS Swift beacon SDK Phase 1 — is shipped (D-153). What remains is gated (date/operator) or tooling-blocked. Do
> NOT manufacture an arc — verify, then wait.

## ⚡ STANDING DIRECTIVE — carried
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move WHEN ONE EXISTS. When the only
remaining work is gated or tooling-blocked, **wait at low frequency** — don't invent work. Ultracode is on (apply to the
*quality* of real work). **Workflow gotcha:** no backticks in workflow prompt prose. `gofmt -l` before any Go push.

## THE FIRST THING TO DO AT OPEN: the two-minute gate
1. **CHECK THE DATE.** `date +%Y-%m-%d`. The §2.7 CI-promotion gate unlocks **≥ 2026-07-23** (at S90 close it was 07-18).
2. **CHECK `operator-expected.md`** — did the operator: answer **[20]** (audit-read model); provide an **Android build
   environment** (JDK+Gradle+Kotlin, or an Android CI job) to unblock §2.12 Android; ask for **iOS Phase 2**; confirm the
   **AMS licence expiry**; or name a new priority? If yes → that is the highest-leverage move: DO IT (Lead B).

## Lead — pick by state (priority order)
**A) IF today ≥ 2026-07-23 → §2.7 CI-promotions.** In `.github/workflows/ci.yml` drop `web-e2e`'s `continue-on-error:
true`; run `actionlint`. Surface the branch-protection required-status-checks **FULL-LIST PUT** to the operator (add
`e2e`, `csp-e2e`, `web-e2e`, `docker-build`, and now `sdk-swift` to the contexts) — repo-admin I cannot set (§2.1). A
CI-config change does NOT roll prod.

**B) IF the operator answered / provided tooling / named a priority → do their pick.**
- **[20]:** (a) keep-open = document + close; (b) gate-admin-reads = `canWrite` check at `handleListAuditLog` (audit.go:92)
  + decide consistency across users/tokens/audit + contracts/tests/mutation/adversarial-review + prod-roll.
- **§2.12 Android** (if a JVM+Gradle+Kotlin env appears): build `sdk/beacon-kotlin` mirroring `sdk/beacon-swift` +
  `sdk/beacon-js` (same frozen schema); Gradle build + JUnit; add a CI job.
- **§2.12 iOS Phase 2** (if an Apple/Xcode runner appears): background `URLSession` config + an AVPlayer/SwiftUI sample.

**C) ELSE (still < 07-23, no operator input) → VERIFY, then WAIT at low frequency. Do NOT manufacture an arc.**
- Quick health check: `git status` clean (only `Caddyfile.prod`); no open non-Dependabot PR needs attention; CI on `main`
  green; date/operator unchanged. (Dependabot PRs are operator-held — do NOT merge autonomously.)
- **One concrete stewardship candidate (my call, non-gated):** the deferred **`log_tail` enum cleanup** — the OpenAPI
  `SourceWrite`/`Source` `type` enums (`contracts/openapi/pulse-api.yaml:3051,3088`) still list the dead `log_tail` type.
  Removing it is a contract-**narrowing** change (a stored `log_tail` source would fail conformance), so treat it
  contract-first: change the yaml + regen `schema.d.ts` + verify the source-create handler's behavior for the removed type
  + a conformance test. Small but real; take it as a bounded arc if you want a concrete move over idling, else leave noted.
- **Optional (at most ONE):** a bounded adversarial "is anything genuinely broken?" sweep like S89's — but S89 already
  swept and drained the drift, so expect little; if it comes up empty, that CONFIRMS the wait. Keep the bar HIGH.
- **Do NOT start** iOS Phase 2 (Xcode/Apple CI) or Android (JDK/Gradle/Kotlin) — both tooling-blocked on this host.
- If none of A/B/C-candidate applies → **re-arm the loop at the max interval (3600s) / low frequency and stop in one line.**

## Pipeline (only if you take A or B or a caught-defect fix under C)
1. Verify-at-open (git clean; date+operator). Record **D-154 IN PROGRESS**. Branch `s91-d154`.
2. Execute (contracts before code). 3. Validate: Go 26-pkg suite via docker (+ mutation-prove SOURCE changes); web full
   `npm test`/build/typecheck/lint; Swift `swift build`/`swift test` if the SDK is touched. 4. Adversarial review for
   security/contract-surface changes. 5. PR → CI → squash-merge --delete-branch → verify origin/main. 6. Roll prod ONLY if
   server/web SOURCE changed (SDK/CI/docs/test-only does NOT). 7. Close docs: D-154, ROADMAP, RESUME → SESSION-92,
   operator-expected, SESSION-91 CLOSED, SESSION-92 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`.
  Mutation copy `/tmp/*.orig`; restore via `cp` (NEVER `git checkout`, D-096). Node at `/home/aytek/.local/bin`.
- **Swift:** on host, `cd sdk/beacon-swift && swift build && swift test` (Swift 6.1.2, Linux). SDK is encode-only, mirrors
  the frozen beacon schema; `URLSession` needs `import FoundationNetworking` on Linux.
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. D-058 stamped build; prod **v0.4.0-119**; rollback tags
  `pulse-prod-pulse:pre-d151` etc. 5-check smoke: version, healthz 200, signed webhook 200, limits 512M/0.5cpu, 0 errors.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>` (D-096;
  `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
