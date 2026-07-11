# SESSION-16 — CI promotions (date gate OPENS ≥2026-07-23) + brandkit phase 2 + probe-stats UI (ROADMAP-V2 S16, planned at S15 close)

> Written by SESSION-15 close (D-075, 2026-07-10). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP-V2.md`
> (plan of record) + `RESUME-PROMPT.md` §7/§8/§12 before dispatching.
> ★ REVISED at D-076 (2026-07-11): the operator answered ALL FOUR questions in S15b —
> v0.3.0 SHIPPED (WO-E done, D-076), CodeQL ENABLED as required (D-076), **PR-FIRST is
> now ACTIVE** (enforce_admins=true, reviews 0 — sessions branch → PR → contexts green →
> merge; NO direct pushes to main), mobile SDKs DEFERRED (WO-F CUT). U3 resolved (license
> live). DASH-muxing fixture skipped by operator. Remaining S16 scope below.

## Mission

Execute ROADMAP-V2 §3 S16. Exit = (a) CI promotions decided-and-applied (the ≥2026-07-23
date gate OPENS if run on schedule — if run before 07-23, re-record the skip carry ×5;
CodeQL is ALREADY required per D-076 — the remaining assessment is e2e / web-e2e /
csp-e2e streaks); (b) brandkit phase 2 (light theme/density/motion) landed OR explicitly
re-gated with evidence; (c) probe-stats UI surface landed; (d) recurring re-checks done.
ALL WORK VIA PRs (D-076 PR-first): branch per scope-group → PR → contexts green → merge
(merge-commit to preserve per-scope commits; squash for single-commit PRs).

## Work orders

1. **WO-A [S, gate ≥07-23]** CI promotions (§2.7) — JOB-level streak re-measure first;
   FULL-LIST PUT; GET-diff proof. CodeQL already required (D-076); assess `e2e`,
   `web-e2e` (non-required since D-055, ~2 weeks green by 07-21) and `csp-e2e` streaks
   via gh before promoting. (Carry ×4: S12/S13/S14/S15 date-gate skips.)
2. **WO-B [S–M]** brandkit phase 2 (§2.15 backlog): light theme, density, motion.
   `brandkit/design-system/tokens.json` is authoritative (never invent values); WCAG
   table in `brandkit/documentation/design-rationale.md` §2 is BINDING; fonts self-
   hosted only. Web gates: lint/typecheck/coverage(59/54/45)/build + Playwright specs
   still green.
3. **WO-C [S]** probe-stats UI surface (D-075 verifier backlog note): ProbesPage
   results panel gains WebRTC columns/badges — `ice_state` + `rtt_ms`/`jitter_ms`/
   `loss_pct` (types already in `schema.d.ts`; key-absent ⇒ render a dash, do NOT
   coerce to 0 — nil-vs-zero is contract semantics, D-075). Update the local
   `ProbeResultsChartData` mapping carefully (all `.map()` call sites).
4. **WO-D [XS]** protection re-check under the NEW PR-first regime (D-076): verify
   enforce_admins=true, strict, 9 contexts (7 + 2 CodeQL), 0 reviews — unchanged; any
   drift is a finding.
5. ~~WO-E v0.3.0 rollout~~ — **DONE in S15b (D-076, 2026-07-11)**; verify prod still
   healthy at open (v0.3.0, license tier active) + confirm operator browser-accept
   happened (ping again if not).
6. ~~WO-F iOS beacon SDK~~ — **CUT (D-076: operator deferred mobile SDKs; revisit only
   on operator re-open).**

Backlog-if-light: **post-U3 beacon-QoE anomaly metrics (§2.14 revisit — U3 is NOW
RESOLVED, real beacon data flows in prod; this item is finally actionable)**; RTMP AMF0
`connect` round-trip (§2.11 tail). DASH live-fixture capture SKIPPED by operator (D-076).

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; ci+e2e+codeql GREEN at HEAD (e2e now also asserts rtt_ms/jitter_ms/
  loss_pct key-presence on the connected WebRTC item).
- Dependabot queue: triage per `docs/dependabot-policy.md`.
- Standings (D-075): Go **74.5%** (floor 70.2; prober 72.8, api 77.1, anomaly 81.6);
  web lines 62.96 / branches 59.04 / functions 52.05 (gates 59/54/45, vitest-4 — NEVER
  compare to pre-rebaseline artifacts); sdk untouched (66.06/45.79/70.42; gates
  63/43/67; 3.52 KB).
- Prod: **v0.3.0** (D-076) healthy; rollback tags `pre-v0.3.0`/`pre-v0.2.0` stand. AMS
  trial license nominally expired 2026-07-12 (operator-waived — observe + report only).
  `ams-teststream` Up at S15b close; live WebRTC checks ONLY on an idle box (D-074).
- U3 RESOLVED (D-076): prod runs with the operator's license; beacon→QoE live-verified.
  Watch: QoE dashboards should accumulate real viewer data once players embed beacon-js.
- Binding rules unchanged: golang:1.25 docker REPO-ROOT mount (D-028); gofmt gate on
  OUTPUT EMPTINESS; `sg docker -c`; pristine-copy compose staging (D-061), unique `-p`;
  commit by explicit path; no subagent reverts (D-063); contracts frozen — CR via
  INT-01 (D-004); adversarial verify BEFORE push; e2e poll conditions: omission
  semantics BINDING (`.get(key, default)`).
- D-075 lessons: latency-budget assertions must DISCRIMINATE (make the sync path
  measurably slow, don't just measure scheduler noise); pion v4 `NewAPI` auto-registers
  default interceptors (incl. stats) when no registry is supplied — `pc.GetStats()`
  suffices; livecheck pattern = `//go:build livecheck` test in the PRISTINE COPY only,
  env-gated URL, idle box.

## Gates (ORCH, before any commit)

- Contract CR (if any) → redocly + ajv + gen:api drift (§8.6).
- Go → full `-race` repo-root mount, floor 70.2, 0 FAIL/0 unexpected SKIP, gofmt
  emptiness, CGO_ENABLED=0 build (both modules).
- Web touched → lint + typecheck + coverage gates + build (+ Playwright if UI flows
  changed — WO-B/WO-C both touch UI).
- e2e.yml touched → yaml parse + STATIC per-key cross-check of every poll condition
  against wave3.go probeResultToAPI (omission semantics).
- Prod untouched (v0.3.0 already live, D-076) — read-only health checks only.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge (PR-first, D-076 — direct
   pushes to main are now blocked by enforce_admins=true).
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-077 close evidence — append EARLY, commit handoffs FIRST.
3. RESUME-PROMPT ▶ START HERE → SESSION-17; ROADMAP-V2 §3/§4/§5 ledgers updated.
4. REFRESH `docs/operator-expected.md` + PushNotification at completion.
5. Write `sessions/SESSION-17.md` from ROADMAP-V2 §3.

## D-076b addendum (2026-07-11, written at S15c close — THIS FILE EDIT + operator-expected.md are UNCOMMITTED in the working tree by design (push budget); fold them into S16's FIRST PR together with the D-076b ledger evidence below

- **S15c hotfix (operator-reported at browser-accept):** mountWebUI served index.html
  for every root-level asset → broken favicon. Fixed (exists-check before SPA fallback,
  TDD `webui_static_test.go`), merged PR #27 (9/9 required green), prod redeployed
  `v0.3.0-4-ge8f8f5f`, `/favicon.svg → image/svg+xml` verified live. AuthGate
  placeholder `pulse_tok_…`→`plt_…`. Fresh admin login token minted for the operator
  (bottom of oguz-testing.md); one mint lost to shell quoting was REVOKED (204).
- **Ledger TODO for S16's first PR:** append D-076b evidence to decisions.md (PR #26
  merge = first PR-first cycle proven; PR #27 = first hotfix cycle; prod stamp
  v0.3.0-4-ge8f8f5f; the favicon fix + placeholder fix; token mint/revoke note).
- **web-e2e FLAKED on PR #26** (vite-proxy ECONNREFUSED noise, auth-401 spec, docs-only
  diff — cannot be diff-related) and PASSED on PR #27 — direct streak input for the
  WO-A promotion decision; measure the full history before promoting web-e2e.
  **[S16 CORRECTION (D-077): this bullet was wrong — web-e2e FAILED on PR #27 too
  (check-runs 29131118536); continue-on-error masked it. Not a flake: deterministic
  D-074 AuthGate regression (SPA-fallback 200 on unproxied /auth/me ⇒ gate never
  renders), red 12 straight runs since ci-run-235; root-caused + fixed in S16.]**
- **Operator items pending:** 👀 UI accept (icon now fixed) · 🔑 vault private key,
  delete deploy/.env.bak-d076 on operator say-so.
- **Push budget note:** S15b+S15c used 3 pushes (close batch, PR #26 branch, PR #27
  branch — the third flagged and operator-motivated). Keep S16 to ≤2.

## S16 session log (2026-07-11, live — D-077)

- **OPEN:** 🔑 key-hygiene operator item RESOLVED — operator: "I have stored the file
  for myself" → `deploy/.env.bak-d076` SHREDDED (`shred -u -z -n 3`); `deploy/.env`
  (live prod config) + `.env.example` (committed) untouched; no other stray .env files
  repo-wide. Prod healthy at open (healthz ok; favicon `image/svg+xml` live). WO-D done
  at open: protection UNCHANGED (enforce_admins=true, strict, 9 contexts, 0 reviews).
  Dependabot queue ZERO. WO-A date gate CLOSED (07-11 < 07-23) → skip carry ×5; streaks
  measured for the record.
- **Operator action required this session: NONE (nothing blocks).** Only standing item:
  👀 browser-accept of the re-branded UI (+ optionals D-V2-1/O7/O11/workflow-scope).
  Session proceeds autonomously: WO-B (brandkit phase 2) + WO-C (probe-stats UI) via
  workflows, ledger folds (D-076b evidence + this log), single close PR, ≤2 pushes.
- **CRASH + RECOVERY:** terminal closed mid-`s16-implement` (authors done, 3 verifiers
  in flight, ~03:26). New session recovered from the persisted workflow script +
  journal — author output intact in the tree, Verify phase re-ran verbatim
  (`s16-verify-continue`). No work lost; workflow persistence validated end-to-end.
- **VERIFY:** 3 lenses → 3 must-fixes (LiveDashboard DensityProvider wrapper; 4 eslint
  DOM globals; invented light `--color-info` removed → inherits dataviz[1]). All applied.
- **GATES:** lint 0 / tsc clean / vitest 339/339, coverage 65.80/61.13/54.85 (gates
  59/54/45, all UP vs S15) / build clean / **Playwright-in-docker 15/15** — the docker
  gate caught 3 real spec bugs first (auth mocks vs the new /auth proxy; ambient
  color-scheme pin; minifier 0ms→0s). web-e2e regression PROVEN fixed: auth-gate ×2 +
  auth-401 specs green under the proxied topology.
- **★ NEW OPERATOR DIRECTIVE (mid-session, recorded as D-078):** Pulse × AMS
  real-validation & product-fit program — 8 phases (product understanding → real test
  env w/ stream+viewer control → e2e scenarios → automated AMS-vs-Pulse parity checks →
  bug protocol → docs program → PRD matrix → marketplace assessment). Plan docs
  authored this session under docs/assessment/ (workflow: 3 scouts + writer + critic);
  EXECUTION starts S17 — S17 scope re-planned around it.
