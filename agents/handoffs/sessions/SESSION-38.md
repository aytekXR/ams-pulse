# SESSION-38 — planned at S37 close (D-099)

> Written by SESSION-37 close (2026-07-15). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146`. Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + the final-assessment §5
> roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-39

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **revise this
plan if a higher-leverage move exists.** This file is a starting point, not a contract. Record any
revision in the D-100 open block.

**S35, S36 and S37 all exercised that clause and all three were right** — S35 → ship-readiness audit;
S36 → user-intake audit (after the operator's question); S37 → the §2.16 goal was **already built
S25/S26**, so it became a tier-entitlement enforcement audit instead. Do the same if the evidence
points elsewhere. **Verify candidate status against the ROADMAP ledger before committing to a goal.**

## ⛔ At open — verify, do not assume

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.**

- `git log --oneline origin/main -3` — S37 (D-099, PR #71) should be on `origin/main`.
- Prod should print the **S37 build** (rolled forward at S37 close — see D-099 evidence for the exact
  `vX.Y.Z-N-g<sha>`):
  ```sh
  sg docker -c "docker exec pulse-prod-pulse-1 /usr/local/bin/pulse version"
  ```
- `/healthz` should still report **`ams_env_configured: true`** (else the operator gets bounced to
  onboarding — S36 regression check):
  ```sh
  curl -sf --resolve beyondkaira.com:443:161.97.172.146 https://beyondkaira.com/healthz \
    | python3 -c "import sys,json;print(json.load(sys.stdin).get('ams_env_configured'))"
  ```
- **Prove an S37 gate is live** — the SSO status endpoint is the cheapest read (prod is Enterprise, so
  it must report `enabled` per whether OIDC is configured — on prod OIDC is off, so `false`, but the
  endpoint must be 200, not a 403/500):
  ```sh
  curl -s --resolve beyondkaira.com:443:161.97.172.146 https://beyondkaira.com/auth/oidc/status
  ```
- Re-check the operator queue **live** (do not trust the doc): GHCR anonymous pull → 401; AMS licence
  expiry (now ~10 days at S38 open — from ~07-25 it outranks GHCR).

## 🔧 Environment gotchas — read BEFORE running any gate

- **Go cannot run bare-metal here.** Repo-root mount + `-w /repo/server` (the test helpers resolve the
  meta DDL via `runtime.Caller` → `<server>/../../../contracts/...`, so mounting `server/` alone
  breaks them — mount the whole repo):
  ```sh
  sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo \
    -v pulse-gomod:/go/pkg/mod -v pulse-gocache2:/root/.cache/go-build \
    -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./..."   # S37 census: 24/24
  ```
- **Playwright tests the built `dist/`, not source.** After ANY web change, `npm run build` **before**
  Playwright (D-098). Run it in `mcr.microsoft.com/playwright:v1.61.1-noble` with `--network host -e CI=1`.
  Node 20 + npm ARE on PATH locally, so `npm run typecheck`/`test`/`build` run bare-metal.
- **`sleep` in the foreground is BLOCKED** — it kills the whole invocation. To wait on a background
  docker run, poll with a `python3 -c` loop using `time.sleep`, not the shell `sleep` command.
- **Background docker runs use `$(pwd)` — pin an ABSOLUTE repo path.** An earlier `cd server` left
  `pwd` inside `server/`, so `-v $(pwd):/repo` mounted the wrong dir and every package "not found".
  Use `-v /home/aytek/repo/ams-pulse:/repo`.

## Mission

**The remaining distance to a first sale is still almost entirely NOT engineering.**

> ### The two operator items still outrank everything a session can do.
> **GHCR is private** (anonymous pull → 401; one click). **The AMS licence expires
> 2026-07-27T13:45Z** — from ~07-25 a lapse + the next `antmedia` restart = total ingest death.
> Surface both, every time. **No session work substitutes for either.**

## S38 candidates — pick by leverage (verify each against the ROADMAP ledger first)

1. **Team-management UI** [S–M] — **highest-value sell-readiness gap.** `/admin/users` CRUD exists
   server-side with **no page**; it's the difference between "one operator" and "a team can use Pulse."
   Named the top of the D-098 non-blocker group. **Start here unless the evidence says otherwise.**
2. **Out-of-band licence-expiry alerting** [S] — the alert evaluator has no `license_expiry` metric;
   the only warning is a UI banner. Small, and directly relevant to the operator's OWN 07-27 expiry
   (a customer who never opens the dashboard gets no warning before downgrade). Strong small alternative.
3. **Audit trail** [M–L] — no actor recorded on writes; gates SOC 2 / ISO 27001 buyers. Larger.
4. **The e2e gaps still open from S34's audit** [S] — Reports **Schedules** tab never activated by a
   test; Probes **create** happy-path never driven. Real holes; small.
5. **§2.7 CI job promotions** — **date-gated ≥ 2026-07-23.** At S38 open (if < 07-23) still locked.
   `web-e2e` / `csp-e2e` carry `continue-on-error: true`; promoting them is real hardening. Check the date.
6. **Reconcile the dead `PULSE_LICENSE_OFFLINE_FILE` path** [XS] — read by `internal/config/config.go`
   but `config.Load()` never runs (`main.go` uses `loadEnvConfig()`; `HOOK(BE-02)`). Wire or delete.
7. **§2.6 unsigned-webhook ingest** / **§2.12 Mobile SDKs** — OPERATOR DECISION FIRST; re-surface only.

**Not candidates:** §2.16 (BUILT S25/S26), §2.19 (uipro COMPLETE), §2.20 (S35), §2.21 (S36), §2.22
(S37 — entitlement enforcement, DONE), §2.3 (licensegen — done).

## ⚠️ Binding lessons — carry into every wave

1. **★ Verify candidate status against the ledger before you commit to a goal.** S37 opened on a goal
   (§2.16) that had shipped **two sessions earlier** — the "deferred twice" note was a propagated
   planning error. One `grep` of ROADMAP-V2 caught it.
2. **★ A green test suite is not a working feature — and a gate with no test is not a gate.** S37's own
   `handleOIDCCallback` licence gate was fully written yet **deletable with zero test failures**; the
   adversarial review caught it. Mutation-prove every security/entitlement guard: remove it, watch its
   test go RED, restore.
3. **★ Audit the whole family, not the instance.** S37 fixed retention on four analytics reads and
   *missed* `QueryProbeResults` — the review found it. When you gate one method, grep every sibling
   that takes the same range/scope and gate them in the same pass.
4. **★ RUN the doc / the path; do not read it** (D-097/D-098). Every S35/S36 blocker had passed review.
5. **Adversarially verify every finding** with refuters that default to REFUTED. S37: 2 raised, 2
   confirmed; S36: 51 raw → 29 confirmed.
6. **Positive allowlists over blocklists** for authz (D-098): enumerate what may pass, never what may not.
7. **Shared-worktree parallelism makes every agent's `git diff` show every other agent's edits** — give
   parallel implementers their own worktree (`isolation: 'worktree'`) if you need per-agent scope review.

## Gates (before any commit)

- Web: lint + `tsc --noEmit` + build + `npx vitest run` + **Playwright in docker AFTER `npm run build`**
  (S36 census: 60/60). Skip Playwright ONLY if zero web files changed (server-only work) — and say so.
- Contract drift: `npm run gen:api` then check **`schema.d.ts` only** (never `git diff --exit-code`).
- Any Go change: full suite in `golang:1.25` docker (24/24), `vet`, `gofmt`.
- WCAG re-check on changed components via `web/src/styles/__tests__/wcag-tokens.test.ts`.
- **`brandkit/` byte-untouched unless the operator has ruled (D-071).** G7 still NOT approved.
- `docs/marketplace/` stays DRAFT-INTERNAL (D-081).
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** — bcrypt hash, public repo. **It is the only
  expected dirty file. A CLEAN `git status` is a FAILURE signal.**
- **NEVER** `git reset --hard` / `git checkout -- .` / `git stash` / `git clean` / `git restore` (D-096).
- **Never restart or "fix" AMS** — observe-only.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — and VERIFY the merge landed on `origin/main`.**
2. `decisions.md` **D-100** evidence — append EARLY, not at the end.
3. RESUME-PROMPT ▶ START HERE → SESSION-39; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-39.md` (carry the standing-directive header).
6. **Roll prod forward** if server/web code changed, per `deploy/runbooks/upgrade-rollback.md` —
   smoke with **evidence**, not the compose "Healthy" label. (STAMPED build: `--build-arg` on `build`,
   then `up -d` WITHOUT `--build`, or the build-args are dropped.)
