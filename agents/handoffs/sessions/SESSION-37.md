# SESSION-37 — planned at S36 close (D-098)

> Written by SESSION-36 close (2026-07-15). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146`. Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5
> roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-38

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this
plan if a higher-leverage move exists.** This file is a starting point, not a contract. Record any
revision in the D-099 open block.

**S35 and S36 both exercised that clause and both were right** — S35 discarded its plan for a
ship-readiness audit; S36 discarded "§2.16 early-warning" for the user-intake audit after the
operator's question exposed three post-login blockers. Do the same if the evidence points elsewhere.

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.**

- `git log --oneline origin/main -3` — S36 (`D-098`, PR #53) should be on `origin/main`.
- Prod should print the S36 build:
  ```sh
  sg docker -c "docker exec pulse-prod-pulse-1 /usr/local/bin/pulse version"
  ```
- **Prove the S36 fix is live** — the role gate is the cheapest real check:
  ```sh
  # A read-scoped token must be denied a write. Mint one first via the UI or:
  #   curl -X POST .../api/v1/admin/tokens -H "Authorization: Bearer <admin>" \
  #        -d '{"kind":"api","name":"probe","scopes":["read"]}'
  # then, with that read token:
  curl -s -o /dev/null -w "%{http_code}\n" -X POST \
    --resolve beyondkaira.com:443:161.97.172.146 \
    -H "Authorization: Bearer <read-token>" \
    https://beyondkaira.com/api/v1/alerts/rules -d '{}'
  # 403 = the scope gate is live (correct). 4xx-other/2xx = investigate.
  ```
- Re-check the operator queue **live** (do not trust the doc): GHCR anonymous pull, licence expiry.

## 🔧 Environment gotchas — read BEFORE running any gate

- **Playwright tests the built `dist/`, not source (D-098 lesson).** After ANY web change,
  `npm run build` **before** running Playwright, or you will debug a bug you already fixed.
- **Playwright cannot run bare-metal here:**
  ```sh
  cd web && pkill -f 'vite preview'
  sg docker -c "docker run --rm --network host -v \$PWD:/work -w /work \
    -e CI=1 mcr.microsoft.com/playwright:v1.61.1-noble npx playwright test"
  # S36 census: 60/60. The container writes ROOT-OWNED playwright-report/ + test-results/;
  # delete them via docker (bare-metal rm hits permission-denied) or eslint chokes on them.
  ```
- **Go cannot run bare-metal here:**
  ```sh
  sg docker -c "docker run --rm -v \$PWD:/src -w /src/server -e GOFLAGS=-buildvcs=false \
    golang:1.25 go test ./..."      # S36 census: 24/24 packages, exit 0
  ```
- **`sleep` in the foreground is BLOCKED** — it kills the whole invocation (exit 144). S35 lost two
  runs and S36 lost one build+test invocation to this.

## Mission

**The remaining distance to a first sale is still almost entirely NOT engineering.**

> ### The two operator items still outrank everything a session can do.
> **GHCR is private** (anonymous pull → 401; one click). **The AMS licence expires
> 2026-07-27T13:45Z** — from ~07-25 a lapse + the next `antmedia` restart = total ingest death.
> Surface both, every time. **No session work substitutes for either.**

## S37 candidates — pick by leverage

1. **§2.16 AMS operational early-warning** [S–M, **OPERATOR-APPROVED**, D-086 addendum] — the
   feature deferred twice now. S36 cleared the intake blockers that were ahead of it, so it is again
   the strongest *approved, unblocked* candidate. **Start here unless the evidence says otherwise.**
2. **The intake non-blocker gaps S36 surfaced** (see D-098 / operator-expected table) — pick if the
   operator prioritizes sell-readiness over monitoring depth:
   - **Team-management UI** — `/admin/users` CRUD exists server-side with **no page**. Highest-value
     of this group: it's the difference between "one operator" and "a team can use Pulse."
   - **OIDC licence-gating** — SSO is priced at Enterprise in the PRD but any tier can enable it.
     Small, revenue-relevant; needs a `CheckSSO`/entitlement + a gate on the OIDC routes.
   - **Out-of-band licence-expiry alerting** — the alert evaluator has no `license_expiry` metric;
     the only warning is a UI banner. Directly relevant to the operator's OWN 07-27 expiry.
   - **Audit trail** — no actor recorded on writes. Larger; gates SOC 2 / ISO buyers.
3. **The e2e gaps still open from S34's audit** [S] — Reports **Schedules** tab never activated by a
   test; Probes **create** happy-path never driven. Real holes; small.
4. **Reconcile the dead `PULSE_LICENSE_OFFLINE_FILE` path** [XS] — read by `internal/config/config.go`
   but `config.Load()` never runs (`main.go` uses `loadEnvConfig()`; `HOOK(BE-02)`). Working var is
   `PULSE_LICENSE_FILE`. Either wire BE-02 or delete the ghost.
5. **§2.7 CI job promotions** — **date-gated ≥ 2026-07-23.** As of S37 open this MAY now be unlocked
   (today ≥ 07-23). `web-e2e` and `csp-e2e` still carry `continue-on-error: true` — a red e2e does
   not block a merge. Promoting them is a real hardening win and is now in-window. **Check the date.**
6. **§2.6 unsigned-webhook ingest** — OPERATOR DECISION FIRST (D-V2-1). Re-surface only.
7. **§2.12 Mobile SDKs** [L] — do not start without an explicit operator call.

**Not candidates:** §2.3 (licensegen — done), §2.19 (uipro — COMPLETE), §2.1 `enforce_admins`
(RESOLVED-deferred). §2.20 ship-readiness (S35) and the intake blockers (S36) are DONE.

## ⚠️ Binding lessons — carry into every wave

1. **★ RUN the doc / the path; do not read it.** Every S35 and S36 blocker had passed prior review.
2. **★ A green test suite is not a working feature.** S36's role-enforcement agent shipped a fix that
   was fully green while the escalation path stayed wide open — because it enforced on `"viewer"`
   while the product mints `"read"`. Mutation-prove security fixes against the scope the product
   *actually issues*, not the role name the design doc uses.
3. **★ Playwright tests `dist/`.** Rebuild before every Playwright run (D-098).
4. **★ A gate you cannot point at in the repo is not a gate** (D-097). Scope drift checks to
   `schema.d.ts`, not `git diff --exit-code` against this permanently-dirty tree.
5. **Adversarially verify every finding.** S36: 51 raw → 29 confirmed, 22 refuted. Two of the
   refuted (AMS-cleartext, billing-severity) would have shipped operator noise or worse.
6. **Positive allowlists over blocklists** for authz. Enumerate what may pass, never what may not.
7. **Shared-worktree parallelism makes every agent's `git diff` show every other agent's edits** —
   S36's per-agent reviewers each flagged the others' files as "undisclosed scope violations." Not
   real; an artifact of running parallel implementers in one tree. If you need per-agent scope
   review, give each agent its own worktree (`isolation: 'worktree'`).

## Gates (before any commit)

- Web: lint + `tsc --noEmit` + build + `npx vitest run --coverage` (floors 59/54/45;
  **S36 census: 638 tests / 38 files**) + **Playwright in docker AFTER `npm run build`** (**60/60**).
- Contract drift: `npm run gen:api` then check **`schema.d.ts` only**.
- Any Go change: full suite in `golang:1.25` docker (**24/24**), `vet`, `gofmt`.
- WCAG re-check on changed components via `web/src/styles/__tests__/wcag-tokens.test.ts`.
- **`brandkit/` byte-untouched unless the operator has ruled (D-071).** G7 still NOT approved.
- `docs/marketplace/` stays DRAFT-INTERNAL (D-081).
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** — bcrypt hash, public repo. **It is the only
  expected dirty file. A CLEAN `git status` is a FAILURE signal.**
- **NEVER** `git reset --hard` / `git checkout -- .` / `git stash` / `git clean` (D-096).
- **Never restart or "fix" AMS** — observe-only.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — and VERIFY the merge landed.**
2. `decisions.md` **D-099** evidence — append EARLY, not at the end.
3. RESUME-PROMPT ▶ START HERE → SESSION-38; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-38.md` (carry the standing-directive header).
6. **Roll prod forward** if server/web code changed, per `deploy/runbooks/upgrade-rollback.md` —
   smoke with **evidence**, not the compose "Healthy" label.
