# SESSION-20 — post-expiry sweep + operator-review intake + P0 bug fixes (ROADMAP-V2 S20, planned at S19 close)

> Written by SESSION-19 close (D-081, 2026-07-11). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §3 S20 + `RESUME-PROMPT.md` §7/§8/§12 AND
> `docs/assessment/final-assessment.md` §5 (the roadmap S20 executes) before
> dispatching.

## Mission

Execute ROADMAP-V2 §3 S20. Exit = (a) post-expiry AMS delta recorded (D-082)
with the blocked-scenario list; (b) operator review of final-assessment.md
ingested (or re-surfaced, non-blocking); (c) BUG-004 fixed TDD (from/to honored
by /qoe/ingest handler; contract unchanged) + BUG-003 fixed TDD (probe scheduler
duplicate-row race) — full §8 gates; (d) CI promotions if run date ≥2026-07-23
(else skip carry ×9); (e) standing re-checks + operator-answer sweep. PR-first,
≤2 pushes.

## Context you must load first

- **FIRST ACTION — post-expiry sweep (now real):** the AMS trial lapsed
  2026-07-12T12:09Z. Read-only sweep vs the pre-expiry baseline (D-081:
  versionName=3.0.3, versionType="Enterprise Edition", build 20260504_1443 at
  07-11T18:2xZ): version endpoint, app-scope REST, WebRTC/HLS serving,
  enterprise-feature 403s. Record the delta in decisions.md **D-082** + which
  scenarios become blocked. Use the lockout-safe harness auth
  (`qa/realams/harness/auth.sh` — NEVER retry a failed login; 2 strikes =
  5-min email-keyed lock; admin@ is also prod's polling account).
- **Operator answers to sweep from `docs/operator-expected.md` FIRST:**
  final-assessment review verdict (approved / edits / silence — silence is
  non-blocking, re-surface politely), AMS-reset confirm, Kafka yes/no,
  marketplace contact, brandkit sign-off, browser-accept.
- **BUG-004** (`docs/assessment/bugs/BUG-004-qoe-ingest-ignores-from-to.md`):
  GET /api/v1/qoe/ingest declares from/to in OpenAPI but the handler calls
  IngestTimeseries with no window → era-mixed buckets (TC-I-04 root cause).
  Fix = parse + plumb the window; contract UNCHANGED (implementation catches
  up to spec, no INT-01 CR needed). TDD: red test proving from/to filters, then
  green. Consider the §6 response-body/parameter contract-test angle.
- **BUG-003** (`bugs/BUG-003-probe-scheduler-duplicate-results.md`): probe
  scheduler emits near-duplicate result rows 0–1 ms apart phase-aligned ~60 s;
  suspected immediate-on-create goroutine racing the periodic ticker. Read the
  scheduler FIRST (D-042: never bump timeouts; find the mechanism). Regression
  test pinning single-row-per-tick. Beware N24 (first-result <100 ms budget —
  don't break After(0) immediate fire).
- **Harness discipline:** memory `shell-harness-false-green-patterns` BINDING
  for any new/edited bash; PASS needs a fresh positive artifact.
- Go work: Docker `golang:1.25`, REPO-ROOT mount, `-race`, 0 api SKIPs (D-028);
  coverage floor 70.2; gofmt gate on output emptiness.

## Work orders

1. **WO-A [S, FIRST]** post-expiry read-only sweep → D-082 delta + blocked-
   scenario list; update validation docs only where reality changed.
2. **WO-B [S]** operator-review intake: apply final-assessment edits / resolve
   answered NEEDS-OPERATOR-CONTACT rows; else re-surface (non-blocking).
3. **WO-C [M, PRIMARY]** BUG-004 + BUG-003 fixes, TDD red→green, full §8 gates
   (build, lint, vet, -race repo-root, coverage, contract drift, staging
   verify on an isolated compose copy — NEVER from the real repo dir, D-061).
   Scopes: BUG-004 = server/internal/api (+query plumb); BUG-003 =
   server/internal/prober scheduler. Disjoint → parallel authors OK.
4. **WO-D [S, gate ≥2026-07-23]** CI promotions: JOB-streak re-measure FIRST;
   if open → csp-e2e FULL-LIST PUT + GET-diff proof; web-e2e earliest ~07-25;
   else skip carry ×9.
5. **WO-E [XS]** standing re-checks (protection drift, dependabot, prod health
   read-only, prod untouched unless a rollout is operator-approved).

*(Backlog-if-light: BUG-002 VoD REST-poll design note; remote-host WebRTC
viewer for non-zero viewer-QoE parity; SRT publisher loss validation;
TC-APP-02 if a blocked test app appeared; DG-05+DG-15 Kafka doc pair.)*

## Gates (ORCH, before any commit)

- WO-C touches Go: FULL §8 set (repo-root mount -race, 0 unexpected SKIP,
  coverage ≥ floor, gofmt-on-emptiness, vet, contract drift `npm run gen:api`
  + git diff --exit-code if API types could shift — BUG-004 changes handler
  behavior, NOT the spec, so generated types must NOT drift).
- Sweep + harness edits: bash -n + shellcheck + the false-green patterns.
- Prod untouched by default; the fixes reach prod only with a future
  operator-approved rollout (they ride main after merge).
- Any new external claims → final-assessment stays DRAFT until operator OK.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-082 evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-21; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + `docs/assessment/session-plan.md`
   (bugs table status) + PushNotification.
5. Write `sessions/SESSION-21.md` (likely: assessment finalization if operator
   answered, remaining roadmap P1s, DG authoring tail).
