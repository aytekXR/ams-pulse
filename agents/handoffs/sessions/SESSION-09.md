# SESSION-09 — post-GA: promotions (date-gated), dependabot absorption, ROADMAP-v2

> Written by SESSION-08 close (D-066, 2026-07-09). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> + `RESUME-PROMPT.md` §7/§8 before dispatching. **GA SHIPPED as v0.2.0** (D-065 declaration,
> D-066 release: tag at `4657512`, release run 29023647495, prod rolled onto the tag).

## Mission

Exit = (a) the CI job promotions land if ≥2026-07-23 and streaks held; (b) the 20 open
dependabot PRs are absorbed with REAL verification (or explicitly re-deferred with reasons);
(c) ROADMAP-v2 (post-GA plan) is seeded and its first session prompt written; (d) the
conditional operator-triggered items (below) execute if their trigger fired. A short session
is fine if the date-gate hasn't opened.

## Preconditions (re-verify cheaply — fix this prompt if stale, note drift in decisions.md)

- Tree clean; ci+e2e+codeql green at HEAD (`gh run list --branch main` — 0-step cancelled
  jobs = GitHub infra blip → `gh run rerun --failed`, D-065 lesson).
- Prod runs the v0.2.0-stamped image (`docker compose ... exec pulse pulse version` →
  `pulse v0.2.0 (commit 4657512 ...)`); healthz ok; `pulse-prod-pulse-1` cpus=1000000000.
- Release run 29023647495 green; GH release v0.2.0 has the notes
  (`gh release view v0.2.0`). Rollback tags: pre-v0.2.0 (= 5d77a05 image), pre-d064,
  pre-d061, pre-d058.
- Coverage floor 70.2; Go total 73.2%.
- Binding rules unchanged (docker golang:1.25 repo-root mount, sg docker, pristine-copy
  compose staging, per-scope commits, no subagent reverts (D-063 §12), concurrent-session
  hazard). LICENSE = PolyForm NC 1.0.0 (D-066) — do not "fix" license headers to MIT.

## Work orders

### WO-A — CI job promotions · trigger: date ≥2026-07-23 AND job-level streaks held · [S]
- Streak evidence at 2026-07-09: web-e2e 7/7 (ci.yml), csp-e2e 7/7 (e2e.yml). Re-measure
  job-level (`gh api .../runs/<id>/jobs`) — both jobs are continue-on-error so
  workflow-level lies.
- FULL-LIST PUT (a partial list silently de-requires the rest): contracts, server, web,
  sdk, docker-build, helm, compose **+ web-e2e + csp-e2e**; GET-diff proof after. Then drop
  `continue-on-error` from both jobs; actionlint; reproduce touched ci.yml steps.
- CodeQL as a required context ONLY with explicit operator OK (streak evidence first).
### WO-B — dependabot absorption (20 PRs) · [M]
- Verdict of record (D-066): #4 closed (golang 1.26 vs D-032 pin + ignore rule shipped).
- Batch 1 (low risk, but release-pipeline-relevant — verify!): actions bumps #8-#12
  (buildx 4, setup-go 6, login 4, qemu 4, cosign-installer 4.1.2). These touch release.yml
  paths that PR CI does NOT exercise — after merging, run the release.yml
  `workflow_dispatch` dry-run input (build+scan, no push) to prove the pipeline still
  works BEFORE the next real tag.
- Batch 2: digest bumps (#1,2,3,5,6 — node/alpine/golang-digest/caddy/clickhouse) —
  rebase (my D-065 hardened.yml edits likely conflict #5/#6), merge on green, then a
  staging boot-smoke of the new digests (pristine-copy stack) + schedule a prod rollout.
- Batch 3 (majors, web/sdk tooling: vite 8, vitest 4×2, plugin-react 6, eslint 10,
  size-limit 12, coverage-v8 4, + grouped minor/patch #13/#14/#18): absorb per-package —
  merge → full web/sdk test+build+size gates locally reproduced; fix breakages TDD;
  one PR at a time, newest toolchain first (vitest 4 before coverage-v8 4).
- Each merge needs owner approval: `gh pr review <n> --approve` then `gh pr merge <n>
  --squash --auto`. NEVER merge with failing required contexts.
### WO-C — ROADMAP-v2 seeding · [S]
- New §"v2 (post-GA)" in ROADMAP.md (or ROADMAP-V2.md if cleaner) from the §2 post-GA list
  + D-065/D-066 carry-overs: O(N²) `rebuildSnapshot` hot path (real fix behind the CPU-cap
  bump); `qa/licensegen` `-privkey`/`-expires` flags for production minting
  (docs/licensing.md §2.1 promise); Postgres meta backend; SSO/OIDC; native probes; mobile
  SDKs; white-label PDF; anomaly expansion; `enforce_admins=true` revisit (due since GA —
  flip once sessions stop pushing to main, or record why not); keep-7 cycle-8 pruning
  verification (~2026-07-16, first real retention exercise); optional unsigned-webhook
  ingest mode w/ IP allowlist (the O3 finding: AMS 3.0.3 can't sign hooks — decide
  build-vs-wontfix with the operator).
### WO-D — conditional operator triggers (check each at start, skip+record if unfired)
- **U3 landed** (PULSE_LICENSE_KEY set in deploy/.env): restart pulse, verify tier in boot
  log + `GET /api/v1/license`; real beacon batch → `/qoe/summary` startup_p50_ms > 0;
  rebuffer_ratio canary on real data. Update alerting.md/OPERATOR-TODO.
- **O7 clicked** (GHCR public): anonymous `docker pull ghcr.io/aytekxr/ams-pulse:v0.2.0`
  + `cosign verify` per release.yml header; record output in decisions.md; close G1 fully.
- **O11 rotated**: confirm new secret works (Slack step green on next push).

## Gates (ORCH, before any commit)

- Full `-race` repo-root when Go/tests touched (floor 70.2); gofmt on OUTPUT EMPTINESS.
- WO-A: GET-diff proof; WO-B: per-batch verification incl. release dry-run after actions
  bumps; staging smoke after digest bumps.
- Reproduce every touched ci.yml step; actionlint on workflow changes; no secrets in diffs.

## Closing protocol (ROADMAP §6)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-067 (per-WO evidence incl. skipped-trigger records).
3. RESUME-PROMPT ▶ START HERE → SESSION-10; ROADMAP ledgers + §5 re-verified.
4. **REFRESH `agents/handoffs/OPERATOR-TODO.md`** (standing user directive) + notify the
   user at session completion (PushNotification).
5. Write `sessions/SESSION-10.md` from ROADMAP-v2.
