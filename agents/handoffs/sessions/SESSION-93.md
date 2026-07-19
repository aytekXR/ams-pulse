# SESSION-93 — planned at S92 close (D-156) — the wildcard `stream_offline` HIGH fix (design-gated), else low-frequency wait

> Written by SESSION-92 close (2026-07-19). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146` (**this host IS
> prod**; no SSH). **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-119** (unchanged — S92 was a finding, no code).
> **S92 found a real HIGH defect** (the default `critical` wildcard "Stream offline" alert never fires — D-156) and ESCALATED
> it for a firing-semantics product call. SESSION-93's primary job is to BUILD that fix **once the semantics are settled**.

## ⚡ STANDING DIRECTIVE — carried
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move WHEN ONE EXISTS. Ultracode is on
(apply to the *quality* of real work). **Workflow gotcha:** no backticks in workflow prompt prose. `gofmt` is NOT on the host
PATH — run via docker (`docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server golang:1.25 gofmt -l .`).

## THE FIRST THING TO DO AT OPEN: the two-minute gate
1. **CHECK THE DATE.** `date +%Y-%m-%d`. §2.7 CI-promotion gate unlocks **≥ 2026-07-23** (at S92 close it was 07-19).
1b. **CHECK THE ANDROID TOOLCHAIN (standing GO D-154).** `command -v gradle && command -v java` (or `kotlinc`). If PRESENT →
   START `sdk/beacon-kotlin` (Lead B, ROADMAP §2.12), no operator prompt. If absent → "toolchain absent, waiting".
2. **CHECK `operator-expected.md`** — did the operator answer the **S92 `stream_offline` firing-semantics** question
   ((a)/(b)/(c)/"use your judgment"); answer **[20]**; ask for **iOS Phase 2**; or name a new priority?

## Lead — pick by state (priority order)
**★ PRIMARY: the wildcard `stream_offline` HIGH fix (D-156 / ROADMAP §2.40).** This is the highest-value autonomous move —
a confirmed HIGH defect in a headline feature. It is **design-gated** on the firing-semantics call:
- **IF the operator answered** ((a) one-shot+auto-clear / (b) sticky / (c) document-as-unsupported / "use your judgment") →
  BUILD that variant.
- **ELSE (no answer)** → you MAY proceed with **(a)** under your own judgment — it is the clear default (a one-shot
  "stream X dropped" page that auto-clears after a grace window is how offline paging should behave) — but state that you are
  self-authorizing (a) and keep the adversarial review mandatory. Do NOT pick (b): with no framework stale-sweep, sticky
  wildcard fires can pile up unbounded.
- **The verified defect (re-read D-156 + the code at open — verify-first):** wildcard `stream_offline` (incl. the default
  `critical` rule, `wave2.go:494` `ScopeJSON:"{}"`) never fires because `evalStreamOffline` (`evaluator.go:730-742`) looks
  for `!s.Active` in `snap.Streams`, but the aggregator removes ended streams from the snapshot BEFORE marking them inactive
  (`onPublishEnd` aggregator.go:306-315; `EvictStale` aggregator.go:242) → the state is unreachable. Scoped rules work.
- **Recommended design (a) — evaluator-local windowed offline-edge, NO `LiveSnapshot` contract/WS change:**
  1. In the evaluator, track per-wildcard-`stream_offline`-rule the set of scope-matching (`app`/`node`) stream IDs present
     in the previous tick.
  2. Each tick, compute present→gone streams; add them to a per-rule "recently offline" set stamped with the tick/time.
  3. While a stream is within the grace window (e.g. `max(rule.WindowS, 2×evalInterval)`), the wildcard path emits `val=1`
     for it (fires). After the window, emit `val=0` for it EXACTLY ONCE (so the alert framework — which has no stale-sweep,
     `evaluator.go:790-795` — RESOLVES it), then drop it from the set. This is the crux: prove BOTH the fire and the resolve.
  4. Keep the SCOPED path unchanged (it works). Keep `compare(val, Operator, Threshold)` honored (D-129).
- **Tests:** REPLACE `TestEvalStreamOffline_WildcardInactive_FiresValueOne_S67` (`s67_d129_test.go:226-236` — it masks the
  bug by injecting an impossible `Active:false` snapshot entry) with a test that drives the REAL aggregator flow
  (publish-start → publish-end → eval) and asserts fire-then-resolve. Mutation-prove the fire AND the resolve edges.
- **Review:** MANDATORY adversarial review of the alert state machine (flapping / false critical page / stuck-firing / the
  cooldown+hysteresis interaction / multi-stream + group_by). This is a critical-alert change — keep the bar high.
- **Deploy:** alert eval is server SOURCE → server rebuild + prod roll + 5-check smoke. Record D-157.

**A) IF today ≥ 2026-07-23 → ALSO §2.7 CI-promotions** (drop `web-e2e` `continue-on-error` in `.github/workflows/ci.yml`,
`actionlint`; hand the operator the branch-protection FULL-LIST PUT adding e2e/csp-e2e/web-e2e/docker-build/sdk-swift). No prod roll.

**B) operator named a different priority / provided Android tooling → do their pick** (Android §2.12, iOS Phase 2, [20]).

**C) IF you do NOT take the stream_offline fix** (semantics unsettled AND you decline to self-authorize (a)) → low-frequency
wait: quick health check (git clean but `Caddyfile.prod`; CI green on main; only Dependabot PRs, operator-held). **Do NOT run
another fresh "is anything broken?" sweep** — S89/S91/S92 have swept three times; the contract-drift class is drained and S92
spent this cycle's sanctioned sweep. Re-arm at low frequency and stop in one line.

## Pipeline (if you take the fix, A, or B)
1. Verify-at-open (git clean; date+operator; RE-READ D-156 + the evaluator/aggregator code — verify-first). Record **D-157
   IN PROGRESS**. Branch `s93-d157`.
2. Execute (contracts before code — this fix needs NO contract change if you keep it evaluator-local). 3. Validate: Go 26-pkg
   suite via docker (+ mutation-prove the SOURCE change — BOTH fire and resolve); web full `npm test`/build/typecheck/lint if
   web touched (it should NOT be). 4. **Adversarial review (mandatory — critical-alert state machine).** 5. PR → CI →
   squash-merge --delete-branch → verify origin/main. 6. **Roll prod** (alert eval is server SOURCE): stamped rebuild +
   5-check smoke. 7. Close docs: D-157, ROADMAP §2.40 → DONE, RESUME → SESSION-94, operator-expected, SESSION-93 CLOSED,
   SESSION-94 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`.
  Mutation copy `/tmp/*.orig`; restore via `cp` (NEVER `git checkout`, D-096). Node at `/home/aytek/.local/bin` (v20.20.2).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. D-058 stamped build; prod **v0.4.0-119**. 5-check smoke:
  version, healthz 200, signed webhook 200, limits 512M/0.5cpu, 0 errors. Rollback tags `pulse-prod-pulse:pre-d151` etc.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>` (D-096;
  `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
