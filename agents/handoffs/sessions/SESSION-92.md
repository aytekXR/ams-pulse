# SESSION-92 — planned at S91 close (D-155) — LOW-FREQUENCY WAIT (log_tail enum follow-up shipped; remaining work gated/tooling-blocked)

> Written by SESSION-91 close (2026-07-19). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146` (**this host IS
> prod**; no SSH). **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-119** (unchanged — S91 shipped a
> contract/types/test/doc change; no runtime source change → no prod roll).
> **Back to the low-frequency wait.** S91 spent the one sanctioned non-gated stewardship candidate (the `log_tail` enum
> narrowing, D-155). What remains is gated (date/operator) or tooling-blocked, or a lower-value/riskier follow-up. Do NOT
> manufacture an arc — verify, then wait.

## ⚡ STANDING DIRECTIVE — carried
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move WHEN ONE EXISTS. When the only
remaining work is gated or tooling-blocked, **wait at low frequency** — don't invent work. Ultracode is on (apply to the
*quality* of real work). **Workflow gotcha:** no backticks in workflow prompt prose. `gofmt -l` before any Go push (gofmt
is not on the host PATH — run it via docker: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server
golang:1.25 gofmt -l .`).

## THE FIRST THING TO DO AT OPEN: the two-minute gate
1. **CHECK THE DATE.** `date +%Y-%m-%d`. The §2.7 CI-promotion gate unlocks **≥ 2026-07-23** (at S91 close it was 07-19).
1b. **★ CHECK THE ANDROID TOOLCHAIN (standing operator GO D-154 — "start the android sdk once I set up the build env
   later").** Run `command -v gradle && command -v java` (or `kotlinc`). **If PRESENT → highest-priority move: immediately
   START `sdk/beacon-kotlin` (Lead B, Android), no further operator prompt** (operator pre-authorized). Turnkey plan in
   ROADMAP §2.12. If absent → note "Android toolchain absent, waiting" and continue the gate.
2. **CHECK `operator-expected.md`** — did the operator answer **[20]** (audit-read model); ask for **iOS Phase 2**; confirm
   the **AMS licence expiry**; or name a new priority? If yes → that is the highest-leverage move: DO IT (Lead B).

## Lead — pick by state (priority order)
**A) IF today ≥ 2026-07-23 → §2.7 CI-promotions.** In `.github/workflows/ci.yml` drop `web-e2e`'s `continue-on-error:
true`; run `actionlint`. Surface the branch-protection required-status-checks **FULL-LIST PUT** to the operator (add
`e2e`, `csp-e2e`, `web-e2e`, `docker-build`, and `sdk-swift` to the contexts) — repo-admin I cannot set (§2.1). A
CI-config change does NOT roll prod.

**B) IF the operator answered / provided tooling / named a priority → do their pick.**
- **[20]:** (a) keep-open = document + close; (b) gate-admin-reads = `canWrite` check at `handleListAuditLog` (audit.go:92)
  + decide consistency across users/tokens/audit + contracts/tests/mutation/adversarial-review + prod-roll.
- **§2.12 Android (STANDING GO — start the moment the toolchain is present):** build `sdk/beacon-kotlin` per the turnkey
  plan in ROADMAP §2.12 (Gradle Kotlin JVM lib mirroring `sdk/beacon-swift` + `sdk/beacon-js` on the frozen schema:
  zero-dep types + hand-rolled JSON writer, UUID session + sampling, a `ScheduledExecutorService`-serialized batching/retry
  `Transport` POSTing `/ingest/beacon` with `X-Pulse-Ingest-Token`, injectable `HttpURLConnection` sender, a `PulseBeacon`
  façade; JUnit5 parity tests; Gradle wrapper + an `sdk-kotlin` CI job, `setup-java` Temurin 21 → `./gradlew build`). NO
  server change → NO prod roll.
- **§2.12 iOS Phase 2** (if an Apple/Xcode runner appears): background `URLSession` config + an AVPlayer/SwiftUI sample.

**C) ELSE (still < 07-23, no operator input) → VERIFY, then WAIT at low frequency. Do NOT manufacture an arc.**
- Quick health check: `git status` clean (only `Caddyfile.prod`); no open non-Dependabot PR needs attention; CI on `main`
  green; date/operator unchanged. (Dependabot PRs are operator-held — do NOT merge autonomously.)
- **★ The S91 sanctioned candidate (the `log_tail` API enum) is SPENT (D-155, PR #179).** The remaining `log_tail`-family
  items were adversarially confirmed NON-blocking and are lower-value or riskier than the API enum — do NOT chase them
  without a clear signal: (1) `contracts/events/ams-server-event.schema.json:33` still lists `log_tail` in the *event-origin*
  enum — a SEPARATE data contract (event provenance, not a config type); narrowing it is riskier (stored ClickHouse events
  may carry the tag) → leave unless you can prove no stored event uses it. (2) the vestigial `log_path` field + its
  `OnboardingWizard.tsx:246` "for log_tail mode" label + `docs/AMS-INTEGRATION.md:356` desc — cosmetic; touching the wizard
  is a *web runtime* change → would force a prod roll for a cosmetic fix (disproportionate). (3) `brandkit/uploads/` stale
  archive snapshots — not authoritative docs.
- **Optional (at most ONE):** a bounded adversarial "is anything genuinely broken?" sweep like S89/S91's — but both already
  swept and drained the drift, so expect little; if it comes up empty, that CONFIRMS the wait. Keep the bar HIGH.
- **Do NOT start** iOS Phase 2 (Xcode/Apple CI) or Android (JDK/Gradle/Kotlin) — both tooling-blocked on this host.
- If none of A/B/C-candidate applies → **re-arm the loop at the max interval (3600s) / low frequency and stop in one line.**

## Pipeline (only if you take A or B or a caught-defect fix under C)
1. Verify-at-open (git clean; date+operator). Record **D-156 IN PROGRESS**. Branch `s92-d156`.
2. Execute (contracts before code). 3. Validate: Go 26-pkg suite via docker (+ mutation-prove SOURCE changes); web full
   `npm test`/build/typecheck/lint; Swift `swift build`/`swift test` if the SDK is touched. 4. Adversarial review for
   security/contract-surface changes. 5. PR → CI → squash-merge --delete-branch → verify origin/main. 6. Roll prod ONLY if
   server/web SOURCE changed (SDK/CI/docs/test/contract-types-only does NOT — cf. D-155/S85). 7. Close docs: D-156, ROADMAP,
   RESUME → SESSION-93, operator-expected, SESSION-92 CLOSED, SESSION-93 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`.
  Mutation copy `/tmp/*.orig`; restore via `cp` (NEVER `git checkout`, D-096). Node at `/home/aytek/.local/bin`
  (v20.20.2); regen web types with `cd web && npm run gen:api`.
- **Swift:** on host, `cd sdk/beacon-swift && swift build && swift test` (Swift 6.1.2, Linux). `URLSession` needs
  `import FoundationNetworking` on Linux.
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. D-058 stamped build; prod **v0.4.0-119**. 5-check smoke:
  version, healthz 200, signed webhook 200, limits 512M/0.5cpu, 0 errors.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>` (D-096;
  `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
