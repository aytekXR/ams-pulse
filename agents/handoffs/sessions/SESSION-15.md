# SESSION-15 — CI promotions (date gate OPEN) + pion phase-2b + conditional v0.3.0 (ROADMAP-V2 S15, planned at S14 close)

> Written by SESSION-14 close (D-074, 2026-07-10). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP-V2.md`
> (plan of record) + `RESUME-PROMPT.md` §7/§8/§12 before dispatching.
> ⚠ CHECK `docs/operator-expected.md` ANSWERS FIRST — S15 has FOUR operator-answer-dependent
> switches (all still unanswered at S14 close): (1) "ship v0.3.0" → WO-C rollout fires
> (now carries D-068+D-070+D-072+D-073+D-074); (2) CodeQL yes/no → WO-A shape; (3) PR-first
> yes/no → WO-E re-arm; (4) mobile-SDK need → WO-F iOS SDK fires ONLY on explicit yes.

## Mission

Execute ROADMAP-V2 §3 S15. Exit = (a) CI promotions decided-and-applied (the ≥2026-07-23
date gate OPENS if run on schedule — if run before 07-23, re-record the skip carry ×4);
(b) pion phase-2b (RTCP rtt/jitter/loss) landed OR explicitly re-gated with evidence
(same pre-declared-yield rule as S14); (c) conditional carries executed or re-recorded;
(d) v0.3.0 rollout if the operator said ship.

## Work orders

1. **WO-A [S, gate ≥07-23]** CI promotions (§2.7) — JOB-level streak re-measure first;
   FULL-LIST PUT; GET-diff proof; CodeQL only with explicit operator OK. (Carry ×3:
   skipped S12/S13/S14 on the date gate.)
2. **WO-B [M]** pion phase-2b (§2.11, D-074 triage record):
   - mock-ams: send RTP over the EXISTING VP8 TrackLocalStaticRTP for ~2s after ICE
     connects (`-webrtc-ice` mode only; deterministic payload).
   - probe: after ice_state=connected, hold ~2s; read inbound-RTP stats (jitter, loss)
     via pion stats interceptor + RTT from the selected ICE candidate pair.
   - contract CR: `rtt_ms`/`jitter_ms`/`loss_pct` nullable on ProbeResult; CH **0008**
     (0007 is taken by ice_state); explicit INSERT column list + Append + SELECT/Scan
     ATOMIC (D-072 hazard); wave3 mapping key-absent semantics.
   - e2e: extend the WebRTC step to assert the stats keys present (`.get` defaults).
   - FIRST to yield if hot (D-074 precedent — record triage, don't land half-tested).
3. **WO-C [M, operator-gated "ship v0.3.0"]** prod rollout — tag v0.3.0; carries
   D-068 + D-070 + D-072 + D-073 + D-074. §8.8 smoke + runbook; rollback tags stand.
   AFTER rollout: ping operator for browser-accept of the re-branded UI (prod renders
   the OLD UI until this ships). NOTE: prod pulse image predates D-056 too — the full
   carry list is long; read ROADMAP §5 + decisions.md D-065→D-074 before the rollout.
4. **WO-D [S]** brandkit phase 2 (light theme/density/motion, §2.15 backlog) — only if
   the session is light.
5. **WO-E [XS]** enforce_admins/PR-first re-check — same rationale-or-flip rule
   (re-recorded D-072/D-073/D-074; enforce_admins=false, strict, 7 contexts, 1 review).
6. **WO-F [L, operator-gated]** iOS beacon SDK phase 1 (§2.12) — ONLY on explicit
   "need mobile SDKs: yes". Swift package; REST parity with sdk/beacon-js; size gate.

Backlog-if-light: DASH live-fixture capture (only if the operator enabled DASH muxing);
post-U3 beacon-QoE anomaly metrics (§2.14 revisit note).

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; ci+e2e+codeql GREEN at HEAD (e2e now asserts the FULL WebRTC chain incl.
  `ice_state=='connected'` + A5b bitrate anomaly).
- Dependabot queue: triage per `docs/dependabot-policy.md`.
- Standings (D-074): Go **74.4%** (floor 70.2; prober 72.6, anomaly 81.6); web
  lines 62.96 / branches 59.04 / functions 52.05 (gates 59/54/45, vitest-4 — NEVER
  compare to pre-rebaseline artifacts); sdk untouched (66.06/45.79/70.42; 3.52 KB).
- Prod: **v0.2.0** healthy until WO-C fires. AMS trial license nominally expired
  2026-07-12 (operator-waived — observe + report only). `ams-teststream` container was
  restarted at S14 (it had crashed, exit 1) — check it's Up if a live WebRTC/media check
  is needed, and check AMS `highResourceUsage` refusals: run live WebRTC checks ONLY on
  an idle box (D-074 lesson — the verify fleet saturating the 2 vCPUs made AMS refuse
  new sessions).
- U3: if `PULSE_LICENSE_KEY` appeared in `deploy/.env`, restart pulse + live-verify
  beacon→QoE, record.
- Binding rules unchanged: golang:1.25 docker REPO-ROOT mount (D-028); gofmt gate on
  OUTPUT EMPTINESS; `sg docker -c`; pristine-copy compose staging (D-061), unique `-p`;
  commit by explicit path; no subagent reverts (D-063); contracts frozen — CR via INT-01
  (D-004); serial wiring author for shared prober/mock-ams files; adversarial verify
  BEFORE push; e2e poll conditions: omission semantics BINDING (`.get(key, default)`).
- NEW (D-074 lessons): test harness wait budgets must STRICTLY dominate the probe's own
  deadline (budget-inversion class, not flake — read the scheduler per D-042);
  `agents/handoffs/real-ams-captures/` is gitignored — pin capture shapes via in-repo
  fixture-replay tests; pion v4.2.16 is the pinned version in BOTH modules.

## Gates (ORCH, before any commit)

- Contract CR → redocly + ajv + gen:api drift (§8.6). CH 0008 → explicit INSERT column
  list + Append + SELECT/Scan in the SAME commit (D-072 hazard).
- Go → full `-race` repo-root mount, floor 70.2, 0 FAIL/0 unexpected SKIP, gofmt
  emptiness, CGO_ENABLED=0 build (pion in both modules — keep running it EARLY).
- Web touched → lint + typecheck + coverage gates + build.
- e2e.yml touched → yaml parse + STATIC per-key cross-check of every poll condition
  against wave3.go probeResultToAPI (omission semantics).
- Prod untouched unless WO-C fires.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-075 close evidence — append EARLY, commit handoffs FIRST.
3. RESUME-PROMPT ▶ START HERE → SESSION-16; ROADMAP-V2 §3/§4/§5 ledgers updated.
4. REFRESH `docs/operator-expected.md` + PushNotification at completion.
5. Write `sessions/SESSION-16.md` from ROADMAP-V2 §3.
