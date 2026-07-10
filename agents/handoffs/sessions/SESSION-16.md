# SESSION-16 — CI promotions (date gate OPENS ≥2026-07-23) + brandkit phase 2 + probe-stats UI (ROADMAP-V2 S16, planned at S15 close)

> Written by SESSION-15 close (D-075, 2026-07-10). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP-V2.md`
> (plan of record) + `RESUME-PROMPT.md` §7/§8/§12 before dispatching.
> ⚠ CHECK `docs/operator-expected.md` ANSWERS FIRST — S16 has FOUR operator-answer-dependent
> switches (all still unanswered at S15 close): (1) "ship v0.3.0" → WO-E rollout fires
> (now carries D-068+D-070+D-072+D-073+D-074+D-075); (2) CodeQL yes/no → WO-A shape;
> (3) PR-first yes/no → WO-D re-arm; (4) mobile-SDK need → WO-F iOS SDK fires ONLY on
> explicit yes.

## Mission

Execute ROADMAP-V2 §3 S16. Exit = (a) CI promotions decided-and-applied (the ≥2026-07-23
date gate OPENS if run on schedule — if run before 07-23, re-record the skip carry ×5);
(b) brandkit phase 2 (light theme/density/motion) landed OR explicitly re-gated with
evidence; (c) probe-stats UI surface landed; (d) conditional carries executed or
re-recorded; (e) v0.3.0 rollout if the operator said ship.

## Work orders

1. **WO-A [S, gate ≥07-23]** CI promotions (§2.7) — JOB-level streak re-measure first;
   FULL-LIST PUT; GET-diff proof; CodeQL only with explicit operator OK. Also assess
   promoting `web-e2e` to required (non-required since D-055, 2026-07-07 — ~2 weeks
   green by 07-21; verify the streak via gh before promoting). (Carry ×4:
   S12/S13/S14/S15 date-gate skips.)
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
4. **WO-D [XS]** enforce_admins/PR-first re-check — same rationale-or-flip rule
   (re-recorded D-072…D-075; enforce_admins=false, strict, 7 contexts, 1 review).
5. **WO-E [M, operator-gated "ship v0.3.0"]** prod rollout — tag v0.3.0; carries
   D-068+D-070+D-072+D-073+D-074+D-075. §8.8 smoke + runbook; rollback tags stand.
   AFTER rollout: ping operator for browser-accept of the re-branded UI (prod renders
   the OLD UI until this ships). Prod image also predates D-056 — read ROADMAP §5 +
   decisions.md D-065→D-075 before the rollout.
6. **WO-F [L, operator-gated]** iOS beacon SDK phase 1 (§2.12) — ONLY on explicit
   "need mobile SDKs: yes". Swift package; REST parity with sdk/beacon-js; size gate.

Backlog-if-light: DASH live-fixture capture (only if the operator enabled DASH muxing);
post-U3 beacon-QoE anomaly metrics (§2.14 revisit); RTMP AMF0 `connect` round-trip
(§2.11 tail — the last probe-depth item).

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; ci+e2e+codeql GREEN at HEAD (e2e now also asserts rtt_ms/jitter_ms/
  loss_pct key-presence on the connected WebRTC item).
- Dependabot queue: triage per `docs/dependabot-policy.md`.
- Standings (D-075): Go **74.5%** (floor 70.2; prober 72.8, api 77.1, anomaly 81.6);
  web lines 62.96 / branches 59.04 / functions 52.05 (gates 59/54/45, vitest-4 — NEVER
  compare to pre-rebaseline artifacts); sdk untouched (66.06/45.79/70.42; gates
  63/43/67; 3.52 KB).
- Prod: **v0.2.0** healthy until WO-E fires. AMS trial license nominally expired
  2026-07-12 (operator-waived — observe + report only). `ams-teststream` Up at S15
  close; live WebRTC checks ONLY on an idle box (D-074 highResourceUsage lesson).
- U3: if `PULSE_LICENSE_KEY` appeared in `deploy/.env`, restart pulse + live-verify
  beacon→QoE, record.
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
- Prod untouched unless WO-E fires.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-076 close evidence — append EARLY, commit handoffs FIRST.
3. RESUME-PROMPT ▶ START HERE → SESSION-17; ROADMAP-V2 §3/§4/§5 ledgers updated.
4. REFRESH `docs/operator-expected.md` + PushNotification at completion.
5. Write `sessions/SESSION-17.md` from ROADMAP-V2 §3.
