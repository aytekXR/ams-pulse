# SESSION-08 — Punch-list + prod currency + promotions → GA declaration

> Written by SESSION-07 on 2026-07-09 per ROADMAP §6. Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record, §3.S8) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) before dispatching.

## Mission

Exit = ROADMAP S8: **G2 restored (prod runs current main), the D-064 punch items landed,
date-gated promotions taken if due — then declare GA if every remaining gap is operator- or
time-owned.** The tag (v1.0.0 vs v0.2.0) and its push are the OPERATOR's call — prepare the
release material, do not push a tag without their word.

## Preconditions (re-verify cheaply — fix this prompt if stale, note drift in decisions.md)

- `git log --oneline -3` shows the D-064 handoff commit (or later); tree clean; ci AND e2e AND
  codeql green on main (`gh run list --branch main -L 6`).
- **Prod is STALE (the whole point of WO-A):** runs `v0.1.0-25-gbc15d43`; the D-062 functional
  commits (8b4e4c7 QoEReader wiring + B7 startup secret load, b94155f B7 handler+contract,
  6865dba/5c8fe96 honest QoE) are NOT ancestors of that image (`git merge-base --is-ancestor
  8b4e4c7 <prod-commit>` fails). healthz ok; rollback tags pre-d061/pre-d058 exist.
- A10 load smoke PASSED (D-064, numbers in ARCH §4); CH memory WATCH 0 hits in soak + prod 24h.
- Promotion clocks: streaks restart 2026-07-09 (D-063 evidence); **due ~2026-07-23** — job-level
  green only (`gh api .../runs/<id>/jobs`), both jobs are continue-on-error so workflow-level lies.
  CodeQL: 4 consecutive green since `5dacb7d`; promote ONLY with operator OK.
- Binding rules unchanged: Go ONLY in docker golang:1.25 repo-root mount + pulse-gomod/
  pulse-gobuild volumes, `sg docker -c`; compose ONLY from a pristine copy (deploy/.env!);
  commit per scope, agents author / ORCH commits; **subagents NEVER `git restore`/`checkout --`
  shared-tree files (D-063 §12)**; concurrent-session hazard (D-062: STOP if HEAD moves).

## Work orders

### WO-A — prod rollout to current main · deploy scope · [L] — BLOCKS GA (G2)
- Pattern: D-058/D-062 rollouts + `deploy/runbooks/upgrade-rollback.md` (S6-written, use it —
  this is also its first real exercise, note any doc lie in decisions.md).
- Steps: staging-verify on an isolated compose project FIRST (D-054); tag the running image
  `pulse-prod-pulse:pre-d064`; stamped-build (build with explicit VERSION/COMMIT/BUILD_DATE
  args THEN `up -d` WITHOUT `--build`); §8.8 smoke — healthz, overview, logs clean, stamped
  version assert, **NEW spot-checks: (a) B7 route live (`/webhook/ams/<name>` 401 fail-closed
  on bad sig), (b) honest-QoE alert path (rebuffer_ratio rule create → nil-CH-data behaviour
  per alerting.md 3-case semantics — on Free tier expect evaluate-vs-0.0, NOT the old proxy)**.
- Migrate leg only if new DDL exists between bc15d43..HEAD (check `contracts/db/` +
  `store/clickhouse/migrations/` diff — B7 added `ams_sources.webhook_secret_enc` via
  applySchemaUpgrades on the META store, which runs at boot; verify it applied: sqlite table has
  the column, log line clean).
### WO-B — image pinning · deploy/helm scopes · [S]
- Pin mock-ams builder (hardened overlay uses floating `golang:1.25`) by digest; pin helm
  `busybox:1.36` initContainer (GAP-206-03) by digest; helm goldens will drift → red-first
  regen ×3; compose config parity check.
### WO-C — 500-stream observability polish · server scope · [S]
- Rate-limit the per-stream health-degraded INFO log (~100 lines/s at 500 degraded streams —
  aggregate to one line per tick with a count; TDD the aggregation); review the pulse 0.5-vCPU
  hardened cap vs measured 147% poll-boundary bursts (raise to 1.0 or document the throttling
  as accepted — decide with evidence, D-064 latency was unaffected).
### WO-D — test-harness tail · server scope · [XS]
- A11 `t.Skipf` on missing /tmp/clickhouse → fail loud when `CI=true` (env-gated Fatalf,
  keep local-dev skip); investigate the 27 migration-time CH `CANNOT_PARSE_INPUT` startup
  errors (cosmetic per D-064 — root-cause or document).
### WO-E — promotions (date-gated) · `.github/` + protection API · [S]
- If ≥2026-07-23 AND job-level streaks held: FULL-LIST PUT (contracts, server, web, sdk,
  docker-build, helm, compose **+ web-e2e + csp-e2e** — a partial list silently de-requires
  the rest), drop `continue-on-error` from both jobs. CodeQL: only with operator OK.
  If before the date: record not-due, hand to SESSION-09.
### WO-F — GA verdict · docs scope · [M]
- If G1–G8 gaps are now ONLY operator (O5 LICENSE, O7 GHCR, U3, U5, O3) / time (promotions if
  not yet due, keep-7 cycle-8): **declare GA in decisions.md** with the evidence table;
  CHANGELOG: move [Unreleased] into a release section; draft release notes. Tag choice + push
  = OPERATOR (via the S1 release pipeline; after their word: push tag, watch run, cosign
  verify, prod rollout carrying the tag).

### Operator asks to surface at session start:
The operator works from `agents/handoffs/OPERATOR-TODO.md` IN PARALLEL with this session —
do NOT re-ask items mid-session; re-verify each at close and refresh that file + ROADMAP §5.
Items: U3 Pro+ license (if the key lands in deploy/.env before WO-A, the rollout restart picks
it up — then live-verify beacon/QoE) · U5 browser/CSP · O3 AMS webhook (→O4 re-check) ·
**O5 LICENSE (last G7 gap)** · O7 GHCR visibility · O8 21 PRs · O11 Slack rotation ·
**O12 secret-scanning** · GA tag choice (v1.0.0 vs v0.2.0) once WO-F recommends declaration.

## Gates (ORCH, before any commit)

- Full `-race` repo-root, 0 FAIL / 2 expected npx skips, floor 70 (ratchet to achieved−3 at GA
  per §4 ledger); gofmt gate on OUTPUT EMPTINESS (never `&&`-chained).
- WO-A: staging before prod (D-054); §8.8 smoke transcript in decisions.md; rollback tag taken
  BEFORE the swap.
- WO-B: helm lint + 3 goldens red-first regen (alpine/helm:3.17.0); caddy/compose config green.
- WO-C: TDD red→green for the log aggregation; before/after log-rate numbers.
- Reproduce every touched ci.yml step; actionlint if workflows change; no secrets in diffs.

## Closing protocol (ROADMAP §6 — the session is NOT done without these)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-065 (per-WO evidence; GA verdict or the remaining gap list; promotion
   decision recorded either way).
3. RESUME-PROMPT ▶ START HERE → SESSION-09; ROADMAP §3.S8 ✅ + §4 ledger + §5 O-items.
4. Write `sessions/SESSION-09.md`: GA-closeout/ROADMAP-v2 seeding if GA declared, else the
   remainder session.
