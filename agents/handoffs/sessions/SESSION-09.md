# SESSION-09 — GA closeout: tag release (operator word), promotions, operator-unblocked items, ROADMAP-v2

> Written by SESSION-08 on 2026-07-09 per ROADMAP §6. Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) before dispatching.
> **GA is DECLARED (D-065).** This session ships whatever the operator has unblocked and seeds
> the post-GA roadmap. Every work order below is CONDITIONAL on its trigger — check each
> trigger cheaply at session start; skip (and record) the ones whose trigger hasn't fired.

## Mission

Exit = the operator-triggered GA actions that are unblocked get DONE (tag release O13,
LICENSE O5, live QoE verify U3, webhook recheck O3→O4); the date-gated promotions land if
due; ROADMAP-v2 (post-GA backlog) is seeded if the operator wants to continue. A session
where NO trigger has fired is legitimate — do the promotions-date check, re-verify prod
health + operator ledger, refresh handoffs, and close short.

## Preconditions (re-verify cheaply — fix this prompt if stale, note drift in decisions.md)

- `git log --oneline -3` shows the D-065 handoff commits; tree clean; ci AND e2e AND codeql
  green on main (`gh run list --branch main -L 10` — ignore Dependabot rows; remember
  0-step queue-cancelled jobs = GitHub infra, `gh run rerun --failed`, D-065 lesson).
- Prod runs `v0.1.0-50-g5d77a05` or later (authed `/live/overview` 200; healthz ok;
  `docker inspect pulse-prod-pulse-1` shows cpus=1000000000). Rollback tags pre-d064/
  pre-d061/pre-d058 exist.
- Coverage floor is 70.2 (ci.yml); Go total 73.2%.
- Binding rules unchanged: Go ONLY in docker golang:1.25 repo-root mount + pulse-gomod/
  pulse-gobuild volumes, `sg docker -c`; compose staging ONLY from a pristine copy
  (deploy/.env!); prod ops = the 5-overlay combo; commit per scope, agents author / ORCH
  commits; subagents NEVER `git restore`/`checkout --` shared-tree files (D-063 §12);
  concurrent-session hazard (D-062: STOP if HEAD moves unexpectedly).

## Work orders (all conditional — check triggers first)

### WO-A — GA tag release · trigger: operator says "tag v1.0.0" or "tag v0.2.0" (O13) · [M]
- Pre-flight: ci+e2e+codeql green at HEAD; if O5 landed, LICENSE is committed BEFORE tagging.
- Rename the CHANGELOG GA-section heading to the chosen version + date; trim the header
  block off `agents/handoffs/RELEASE-NOTES-DRAFT.md` into the release body.
- `git tag vX.Y.Z && git push origin vX.Y.Z` → `gh run watch` the release run (CI-gated,
  Trivy, multi-arch, SBOM+provenance, cosign). Then `cosign verify` per release.yml header
  (needs O7 public or `gh auth refresh -s read:packages`).
- Prod rollout carrying the tag per `deploy/runbooks/upgrade-rollback.md` (staging first,
  `pre-<tag>` rollback tag, stamped build — VERSION arg = the tag, §8.8 smoke). Note: the
  runbook's inspect command uses container `pulse-prod-pulse-1` (fixed D-065).
- `gh release edit vX.Y.Z --notes-file <trimmed draft>`.
### WO-B — promotions · trigger: date ≥2026-07-23 AND job-level streaks held · [S]
- Evidence command: per-run `gh api .../actions/runs/<id>/jobs` job conclusions (web-e2e in
  ci.yml, csp-e2e in e2e.yml — both continue-on-error so workflow-level lies). Streaks were
  7/7 + 7/7 on 2026-07-09 (D-065).
- FULL-LIST PUT (a partial list silently de-requires the rest): contracts, server, web, sdk,
  docker-build, helm, compose **+ web-e2e + csp-e2e**; then drop `continue-on-error` from
  both jobs (red-first: prove the PUT took effect via a GET diff). actionlint + reproduce
  the touched ci.yml steps. CodeQL as required context ONLY with operator OK.
### WO-C — LICENSE · trigger: operator picks one (O5) · [XS]
- Draft the chosen license verbatim (Apache-2.0/AGPL-3.0/BUSL-1.1/other), root `LICENSE`;
  reconcile install.md/README/SECURITY.md references; note the MIT SDK subdir explicitly.
### WO-D — live QoE verify · trigger: PULSE_LICENSE_KEY uncommented+set in deploy/.env (U3) · [S]
- Restart pulse (`up -d` recreates on env change), verify license tier in boot log; send a
  real beacon batch (pattern: e2e.yml A2, but against prod with a prod ingest token —
  mint via the API); poll `/qoe/summary` for startup_p50_ms > 0; confirm rebuffer_ratio
  rules now read real data (create+delete a canary). Record in decisions.md; update
  alerting.md's "requires U3" phrasing if it becomes stale.
### WO-E — webhook recheck · trigger: operator configured AMS console (O3) · [XS]
- Caddy logs show POSTs to /webhook/ams*; `invalid signature` WARN does NOT recur (O4);
  lifecycle events visible in Pulse within one poll interval.
### WO-F — ROADMAP-v2 seeding · trigger: always (unless operator says stop) · [S]
- Seed `ROADMAP.md` v2 section (or a new file if cleaner) from the §2 post-GA list +
  D-065 carry-overs: O(N²) `rebuildSnapshot` hot path (the real fix behind the CPU-cap
  bump), Postgres meta backend, SSO/OIDC, native probes, mobile SDKs, white-label PDF,
  anomaly expansion, keep-7 cycle-8 confirmation (time), dependabot majors absorption (O8),
  `enforce_admins=true` revisit (D-058 said revisit at GA declaration — now due: flip it
  once sessions stop pushing directly to main, or record why not yet).
- Backup keep-7: cycle count will pass 8 around 2026-07-16 — verify pruning actually
  deletes the oldest artifact (first real keep-7 exercise).

## Gates (ORCH, before any commit)

- Full `-race` repo-root when Go is touched (floor 70.2); gofmt on OUTPUT EMPTINESS.
- WO-A: staging before prod; rollback tag BEFORE swap; smoke transcript in decisions.md.
- WO-B: GET-diff proof of the protection contexts after the PUT; actionlint.
- Reproduce every touched ci.yml step; no secrets in diffs.

## Closing protocol (ROADMAP §6 — the session is NOT done without these)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-066 (per-WO evidence incl. skipped-trigger records).
3. RESUME-PROMPT ▶ START HERE → SESSION-10; ROADMAP ledgers + §5 O-items re-verified.
4. **REFRESH `agents/handoffs/OPERATOR-TODO.md`** (user directive: keep the operator's
   expected-actions file current at every close).
5. Write `sessions/SESSION-10.md` (or, if ROADMAP-v2 says the project is feature-complete
   and the operator has gone quiet, a maintenance-mode resume prompt).
