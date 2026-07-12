# SESSION-21 — post-expiry sweep (FINALLY REAL) + operator intake + P1 backlog (ROADMAP-V2 S21, planned at S20 close)

> Written by SESSION-20 close (D-082, 2026-07-12). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §3 S21 + `RESUME-PROMPT.md` §7/§8/§12 AND
> `docs/assessment/final-assessment.md` §5 (the roadmap this executes) before
> dispatching.

## Mission

Exit = (a) the **post-expiry AMS delta** finally recorded (D-083) with the
blocked-scenario list — S19 AND S20 both ran pre-expiry and re-gated it; S21 is
the first session that actually runs after the 2026-07-12T12:09Z lapse, so this
is no longer deferrable; (b) operator intake (caddy-vhost decision +
final-assessment review) applied or re-surfaced; (c) BUG-005 fixed TDD + the
parameter-conformance contract test that would have caught BUG-004/BUG-005 as a
class; (d) CI promotions if run date ≥2026-07-23 (else skip carry ×10);
(e) standing re-checks. PR-first, ≤2 pushes.

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` — two live items.**
   - **Caddy vhost:** branch `caddy-bedirhan-vhost` (commit `2d3f539`) holds the
     operator's own `bedirhandemirel.beyondkaira.com` vhost. **`origin/main` does
     NOT have it; live prod Caddy DOES.** A Caddy reload from a clean main
     checkout would drop that site. If the operator said "merge it" → open a
     one-commit PR from that branch (it is deploy-config only; the `deploy/`
     scope). If not → **leave it alone**, do NOT revert
     `deploy/config/Caddyfile.prod` on disk (prod mounts that file), and
     re-surface. The untracked `Caddyfile.prod.bak-bedirhan-20260712` is the
     operator's, not ours.
   - **final-assessment review:** still DRAFT, still unreviewed. Nothing external.
2. **Concurrent-session hazard is now CONFIRMED RECURRENT (D-062, D-082).** If HEAD
   moved or the tree dirtied with work you did not do: STOP, inspect (scan the diff
   for secrets), **preserve the foreign commit on its own branch, reset your branch,
   never revert their working-tree file, never absorb it into your PR.**

## Context you must load first

- **FIRST ACTION — post-expiry sweep (finally real).** Read-only, vs the pre-expiry
  baseline re-confirmed at S20 open (D-082: `versionName=3.0.3`,
  `versionType="Enterprise Edition"`, `buildNumber=20260504_1443`, apps
  `[LiveApp, WebRTCAppEE, live, pulse-test]` at 2026-07-11T22:34Z). Sweep: version
  endpoint, app-scope REST, WebRTC/HLS serving, enterprise-feature 403s, and whether
  Pulse's own polling (prod uses `admin@`) still works. Record the delta in
  decisions.md **D-083** + which `qa/realams` scenarios become blocked. **If nothing
  changed, say so explicitly** — a null delta is a real result, not a non-finding.
  Use the lockout-safe harness auth (`qa/realams/harness/auth.sh`) — NEVER retry a
  failed login (2 strikes = 5-min email-keyed lock; `admin@` is ALSO prod's polling
  account, so a lockout breaks prod polling).
- **BUG-005** (documented in `bugs/BUG-004-qoe-ingest-ignores-from-to.md` §Residual):
  `/qoe/ingest` still ignores the declared `interval` param (enum hour|day, default
  day) — `BucketSeconds` stays 0 → silent 60 s buckets. Same declared-but-ignored
  class as BUG-004. Map `hour→3600`, `day→86400`, but **keep the 60 s default when
  `interval` is absent** (the F4 "degradation visible within 15 s" criterion depends
  on it). TDD red→green; contract unchanged.
- **The class fix (higher value than BUG-005 itself):** RESUME-PROMPT §6 has wanted
  **response-body/parameter conformance tests (kin-openapi)** in `internal/api` for
  a long time. CI lints the OpenAPI spec but never asserts handlers HONOR what the
  spec declares — which is exactly why BUG-004 and BUG-005 were both invisible to
  the pipeline. Consider a table-driven test that, for every declared query param on
  every route, asserts the handler observably reacts to it. That closes the class,
  not just the instance.
- Go work: Docker `golang:1.25`, **REPO-ROOT mount**, `-race`, **0 api SKIPs** (D-028
  — assert the skip count, don't eyeball it); coverage floor 70.2 (now at **74.8%**);
  gofmt gate on **output emptiness**, never on exit code.
- **Rate-limit lesson (D-082):** subagents can die mid-phase on the weekly limit,
  leaving ungated code in the tree. **Never trust a tree a dead workflow left** —
  ORCH re-runs the gates, and re-derives any RED proof the dead author never
  produced (pristine-copy revert, never the real tree).

## Work orders

1. **WO-A [S, FIRST]** post-expiry read-only sweep → D-083 delta + blocked-scenario
   list; update validation docs only where reality actually changed.
2. **WO-B [S]** operator intake: caddy-vhost merge if approved; final-assessment
   edits if reviewed; else re-surface both (non-blocking).
3. **WO-C [M, PRIMARY]** BUG-005 fix + the parameter-conformance contract test that
   closes the declared-but-ignored class. Full §8 gates.
4. **WO-D [S, gate ≥2026-07-23]** CI promotions: JOB-streak re-measure FIRST; if open
   → csp-e2e FULL-LIST PUT + GET-diff proof; web-e2e earliest ~07-25; else skip
   carry ×10.
5. **WO-E [XS]** standing re-checks (protection drift, dependabot, prod health
   read-only, prod untouched unless a rollout is operator-approved).

*(Backlog-if-light: BUG-002 VoD REST poll — the design note is written and its two
migrations need INT-01 CRs, so this is now a BUILD decision, not a design one;
remote-host WebRTC viewer for non-zero viewer-QoE parity; SRT publisher loss
validation; DG-05+DG-15 Kafka doc pair.)*

## Gates (ORCH, before any commit)

- Any Go change: FULL §8 (repo-root `-race`, 0 unexpected SKIP, coverage ≥ floor,
  gofmt-on-emptiness, vet, contract drift `npm run gen:api` + `git diff --exit-code`).
- Sweep + harness edits: `bash -n` + shellcheck + the false-green patterns (memory
  `shell-harness-false-green-patterns` is BINDING for any new/edited bash).
- Prod untouched by default. The S20 fixes (BUG-003/004) ride `main` and reach prod
  only with a future operator-approved rollout.
- final-assessment stays DRAFT until the operator OKs it. Nothing external.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-083 evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-22; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + `docs/assessment/session-plan.md` (bugs
   table status) + PushNotification.
5. Write `sessions/SESSION-22.md`.
