# SESSION-07 — GA gate + post-GA backlog seeding (+ date-gated promotions)

> Written by SESSION-06 on 2026-07-09 per ROADMAP §6. Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record, §3.S7) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) before dispatching.

## Mission

Exit = ROADMAP S7: **adversarial GA audit — declare GA with evidence, or produce the punch
list.** Re-run the 9-scout audit (same dimensions as D-057), diff against G1–G8, turn every
unmet criterion into a work order (execute the small ones in-session, roadmap the rest). Plus:
A10 load smoke; the promotion duty if the date has arrived; the release-tag decision is the
OPERATOR's (v1.0.0 vs v0.2.0) — prepare, don't push a tag without their word.

## Preconditions (re-verify cheaply — fix this prompt if stale, note drift in decisions.md)

- `git log --oneline -3` shows the D-063 handoff commit (or later); tree clean; ci AND e2e AND
  codeql green on main (`gh run list --branch main -L 6`).
- Prod runs `v0.1.0-25-gbc15d43` (unchanged by S6 — docs/Helm only): healthz ok, alert delivery
  live-proven (D-062). Rollback tags `pulse-prod-pulse:pre-d061` + `:pre-d058`. WATCH: CH
  "Memory limit (total) exceeded 1.80 GiB" on server_events inserts (D-062, seen once; grep
  signature is documented in deploy/runbooks/monitoring.md) — if it recurred, that's a work order.
- Standing numbers post-S6 (D-063): Go total **73.2%** (floor **70.0**); web 76/72/45 + guard;
  SDK 62/73/70 (3.52 KB); conformance 51/52 + 1 waived; only 2 skips (domain npx).
- **G-status going in:** G1 ✅ except O7 (GHCR package private — re-verify; if the operator
  flipped it, run the cosign verify + anonymous pull from release.yml header and close it);
  G2 ✅; G3 ✅; G4 ✅; G5 partial (promotions below); G6 ✅; **G7 ✅ except LICENSE (O5)**;
  G8 = operator items (U3 license, U5 browser/CSP, O3 AMS-side webhook → then O4 re-check).
- **Concurrent-session hazard (D-062/O11, §12):** if HEAD moves or the tree dirties with foreign
  work mid-session, STOP and inspect. O11 (Slack webhook rotation + other session's local reset)
  — check with the operator whether done; re-surface if not.
- **⚠️ NEW binding rule (D-063, RESUME §12):** workflow subagents NEVER `git restore`/`git
  checkout --` shared-tree files — violations are reported, ORCH reverts; ORCH commits early per
  scope. Put this line in every work order you dispatch.
- CodeGraph: scouts query it BEFORE grep; closing runs `codegraph sync` + `status`.
- Binding env unchanged: Go ONLY in Docker golang:1.25 repo-root mount (+pulse-gomod/pulse-gobuild
  volumes), `sg docker -c`; compose stacks ONLY from a pristine copy (never the real repo dir —
  deploy/.env); commit by explicit path; agents author, ORCH commits.

## Work orders (one Workflow: read-only scouts → gap work orders → adversarial verify → ORCH gate/commit)

### WO-1 — 9-scout GA re-audit · read-only fan-out · [L]
- Same dimensions as D-057: (1) Go tests/coverage, (2) CI workflows + required contexts,
  (3) release pipeline (incl. O7 state: cosign verify + anonymous GHCR pull), (4) dockerization/
  compose, (5) Helm, (6) contracts conformance, (7) web/SDK gates + e2e, (8) stubs/feature
  honesty + docs truth spot-check (S6 claims: pick 5 operator commands from the new runbooks and
  RUN them), (9) git/GitHub hygiene + live prod smoke (§8.8, incl. CH memory-WATCH grep).
- Output: per-dimension verdict with command evidence, diffed against ROADMAP §2 G1–G8.
  Every unmet criterion → a punch-list item with size estimate. NO fix work inside the audit.
### WO-2 — A10 load smoke · isolated stack, read-only vs prod · [M]
- Mock-ams at scale on an ISOLATED compose project (pristine copy): several hundred streams
  (`/control/set_viewers`, the D-061 VD-04 path proved 500), sustained soak ≥15 min; watch pulse
  memory/CPU vs limits, CH insert lag/errors (the 1.80 GiB WATCH!), WS fan-out, dashboard render.
- Record numbers in ARCHITECTURE §4 (VD-04 pattern); regressions → punch list.
### WO-3 — promotions (date-gated) · scope `.github/` + branch-protection API · [S]
- If today ≥ 2026-07-23 AND job-level streaks held since 2026-07-09 (verify per D-063's method:
  job-level conclusions via `gh api .../runs/<id>/jobs`, NOT workflow-level — web-e2e and csp-e2e
  are continue-on-error): promote BOTH into required contexts with a **FULL-LIST PUT**
  (contracts, server, web, sdk, docker-build, helm, compose + web-e2e + csp-e2e — a partial list
  silently de-requires the rest), and drop `continue-on-error` from csp-e2e (and web-e2e).
- CodeQL: promote ONLY if ≥1 week green (first green 2026-07-09) AND the operator explicitly
  agrees (linter-class gate). Record the decision either way in decisions.md.
- If before 2026-07-23: record "not due" and hand the duty to SESSION-08.
### WO-4 — GA verdict + punch list / backlog seeding · docs scope · [M]
- If G1–G8 all met (or every gap is operator-only): declare GA in decisions.md with the evidence
  table; prepare the release: CHANGELOG release section from [Unreleased], release-notes draft.
  **Tag choice + push is the OPERATOR's** (v1.0.0 vs v0.2.0, via the S1 pipeline; watch + cosign
  verify after). Prod rollout carrying the tag AFTER the release run is green.
- Else: punch list as ROADMAP §3 session entries (S8…), sized and ordered; seed the post-GA
  backlog (§2 list: Postgres meta, SSO/OIDC, native probes, mobile SDKs, PDF logo, anomaly
  expansion) as ROADMAP v2 §post-GA — only if the operator wants to continue past GA.

### Operator asks to surface at session start (they gate GA):
U3 Pro+ license (QoE in prod) · U5 browser/CSP check · O3 AMS-console webhook config (then O4
WARN re-check) · **O5 LICENSE pick — the only G7 gap** · O7 GHCR package visibility · O8 21
dependabot PRs · O11 Slack-webhook rotation + concurrent session's local reset.

## Gates (ORCH, before any commit)

- Full `-race` repo-root suite green at close (floor 70) even if no Go touched; if punch-list
  fixes touched Go, coverage report + ratchet check (§4 ledger: ratchet to achieved−3 at GA).
- Reproduce EVERY ci.yml step a change touches (D-053/D-055); helm lint + 3 goldens if Helm
  touched (alpine/helm:3.17.0); actionlint if workflows touched; no secrets in diffs.
- Load-smoke numbers recorded before/after any tuning change; staging-verify before prod for
  any boot-behavior change (D-054).

## Closing protocol (ROADMAP §6 — the session is NOT done without these)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-064 (audit table + GA verdict or punch list; promotion decision recorded).
3. RESUME-PROMPT ▶ START HERE → SESSION-08; ROADMAP §3.S7 ✅ + §4 ledger + §5 O-items re-checked.
4. Write `sessions/SESSION-08.md`: either the punch-list execution session, or the GA-closeout /
   ROADMAP-v2 seeding session — per WO-4's outcome.
