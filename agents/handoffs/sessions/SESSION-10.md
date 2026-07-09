# SESSION-10 — housekeeping + O(N²) fix + licensegen flags (ROADMAP-V2 S10)

> Written by SESSION-09 close (D-067, 2026-07-09). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP-V2.md`
> (plan of record for post-GA; ROADMAP.md covers the GA sprint S1–S8) + `RESUME-PROMPT.md`
> §7/§8/§12 before dispatching. Prod: **pulse v0.2.0** (commit 4657512) + D-067 digest-refreshed
> caddy/clickhouse/backup containers, healthy.

## Mission

Execute ROADMAP-V2 §3 S10. Exit = (a) `enforce_admins` flipped to true OR rationale committed;
(b) keep-7 backup cycle-8 pruning observed + recorded (date-gated ≥2026-07-16); (c) `qa/licensegen`
`-privkey`/`-expires` flags TDD-green + docs/licensing.md §3 updated (formally deferred from
D-066 "S9 WO" → S10, recorded in D-067); (d) O(N²) `rebuildSnapshot`
(server/internal/collector/aggregator/aggregator.go:459) profiled → fixed → benchmarked at
100/500/1k streams (O(N) or flat on the 500-stream fixture) + TDD regression; (e) dependabot
steady-state policy committed; (f) CI promotions if date ≥2026-07-23 AND job-level streaks held
(web-e2e / csp-e2e; FULL-LIST PUT per SESSION-09 WO-A spec; CodeQL-required ONLY with explicit
operator OK — ask/check OPERATOR-TODO for the answer).

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; ci+e2e+codeql GREEN at HEAD (`gh run list --branch main`; 0-step cancelled jobs
  = GitHub capacity blip → `gh run rerun <id> --failed`, D-065 lesson).
- **Dependabot queue: 0 open PRs expected** (S9/D-067 absorbed all 20: batch-1 actions ×5 +
  release dry-run proof; batch-2 digests ×5 + staging smoke + prod refresh; batch-3 majors —
  #22 carried the web vite-8/vitest-4/coverage-v8-4/plugin-react-6 cluster, #17/#16 carried sdk
  co-bumps, #18/#21/#19/#20 auto-closed as superseded, #15/#13/#14 merged after dependabot
  rebase). If new PRs appeared since, triage per the S9 pattern.
- **Coverage standings (RE-BASELINED under vitest 4 — D-067):** Go total 73.2% / ci floor 70.2
  (unchanged). Web gates **59/54/45** (achieved 62.13/57.6/51 — vitest-4 rolldown instrumentation
  reads systematically lower on identical code; guard test `web/src/test/coverage-gate.test.ts`
  pins gates + exclude list). SDK gates **63/43/67** (achieved 66.06/45.79/70.42; branches drop =
  vitest-4 v8 branch-granularity change; lines RATCHETED UP 62→63). SDK size 3.52 KB / 15 KB
  gate. Do NOT "restore" the old 76/72 / 62/73 numbers — they are not comparable across
  instrumentation engines.
- Binding rules unchanged: Go ONLY in docker golang:1.25 with REPO-ROOT mount (D-028); gofmt
  gate on OUTPUT EMPTINESS, never `&&`-chained; `sg docker -c`; pristine-copy compose staging
  (D-061) with unique `-p`, never from real deploy/ (prod .env!); commit by explicit path; no
  subagent reverts (D-063); concurrent-session hazard (§14); LICENSE = PolyForm NC 1.0.0 at
  root + MIT for SDK — never "fix" it.
- **PR-automation lessons (D-067, follow them):** gh token LACKS `workflow` scope → API
  update-branch 403s on PRs touching `.github/workflows/*` (use `@dependabot rebase`); API
  update-branch on lockfile PRs textually merges package-lock.json → EUSAGE desync (use
  `@dependabot rebase` for pristine dependabot PRs); NEVER `@dependabot rebase` a PR carrying
  session-pushed commits (force-push destroys them — API update-branch is safe there);
  repo has NO auto-merge → poll + `gh pr merge --squash`.

## Work orders (sizes from ROADMAP-V2 §3)

1. **WO-A [XS]** `enforce_admins=true` revisit (§2.1) — overdue since GA. Flip via
   `gh api -X PATCH .../branches/main/protection/enforce_admins` style call ONLY if sessions can
   work PR-first; otherwise commit the rationale (sessions still push directly to main) to
   ROADMAP-V2 §4 D-V2-3 and set the next revisit date.
2. **WO-B [XS]** keep-7 backup cycle-8 verification (§2.2) — trigger ~2026-07-16. If session
   date < 07-16: skip + record. Else: inspect the backup sidecar volume listing; confirm the
   oldest (cycle-1) backup was pruned at cycle 8 and restore-verify still passes.
3. **WO-C [S]** `qa/licensegen` `-privkey`/`-expires` flags — TDD red→green (main_test.go
   pattern exists: TestOutputTwoLines); then docs/licensing.md §3 vendor-key ceremony steps.
4. **WO-D [M]** O(N²) rebuildSnapshot fix — profile under the 500-stream A10 fixture first
   (evidence memo: 147% single-core bursts at poll boundaries, D-065 WO-C); redesign (incremental
   snapshot or dirty-marking); benchmark 100/500/1k; TDD regression pinning the complexity;
   update ARCHITECTURE.md §4 numbers; consider REVERTING the CPU cap 1.0→0.5 in compose+helm
   if the fix lands (record either way).
5. **WO-E [XS]** Dependabot steady-state policy write-up (§2.4) — cadence, auto-absorb rules
   for patch/digest, the co-upgrade-cluster lesson (vitest/@vitest/* must move together), where
   fixes get pushed (carrier PR pattern).
6. **WO-F [S, date-gated ≥2026-07-23]** CI promotions carry-over — re-measure JOB-level streaks
   (`gh api .../runs/<id>/jobs`; both jobs are continue-on-error so workflow-level lies);
   FULL-LIST PUT (contracts, server, web, sdk, docker-build, helm, compose + web-e2e + csp-e2e);
   GET-diff proof; drop `continue-on-error`; actionlint; reproduce touched ci.yml steps.

## Gates (ORCH, before any commit)

- Go touched → full `-race` repo-root mount, floor 70.2, 0 FAIL/0 unexpected SKIP; gofmt on
  output emptiness. Web/sdk touched → full package gates at the NEW baselines. Workflow files
  touched → actionlint + faithful step reproduction. Prod touched → staging first (pristine
  copy), rollback path stated, §8.8-style smoke after.
- CH-startup flake watch: `TestQuery_GeoBreakdown_NonEmptyRows` timed out ONCE on a PR run
  (D-067, 60s budget at query_integration_test.go:86; same pattern in vd19/vd24/accounting
  harnesses). If it recurs: bump 60s→180s in ALL four copies as one TDD-gated commit (D-039
  precedent) — do not just rerun a third time.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-068 (per-WO evidence incl. skipped-trigger records).
3. RESUME-PROMPT ▶ START HERE → SESSION-11; ROADMAP-V2 ledgers re-verified.
4. **REFRESH `agents/handoffs/OPERATOR-TODO.md`** + PushNotification at completion.
5. Write `sessions/SESSION-11.md` from ROADMAP-V2 §3 S11.
