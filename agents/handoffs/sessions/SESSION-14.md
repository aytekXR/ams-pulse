# SESSION-14 — WebRTC pion media path + OIDC phase 2 + CI promotions (ROADMAP-V2 S14, planned at S13 close)

> Written by SESSION-13 close (D-073, 2026-07-10). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP-V2.md`
> (plan of record) + `RESUME-PROMPT.md` §7/§8/§12 before dispatching.
> ⚠ CHECK `docs/operator-expected.md` ANSWERS FIRST — S14 has FOUR operator-answer-dependent
> switches (unchanged from S13, all still unanswered at S13 close): (1) "ship v0.3.0" →
> WO-E rollout fires (now carries D-068+D-070+D-072+D-073); (2) CodeQL yes/no → WO-A shape;
> (3) PR-first yes/no → WO-G re-arm; (4) mobile-SDK need → WO-H iOS SDK fires ONLY on
> explicit yes ("no" cuts §2.12).

## Mission

Execute ROADMAP-V2 §3 S14. Exit = (a) pion phase-2a lands (ICE-connected, `ice_state`,
CH 0007) with phase-2b (rtt/jitter/loss) landed OR explicitly re-gated with evidence;
(b) CI promotions decided-and-applied or re-gated (date gate ≥2026-07-23 — check the
calendar THIS session; CodeQL only with operator OK); (c) SPA login uses the OIDC cookie
flow when configured; (d) anomaly metric expansion landed or re-gated (manifest-owner
ruling first); (e) probe segment LimitReader hardening landed; (f) conditional carries
executed or re-recorded.

## Work orders

1. **WO-A [S, ≥07-23]** CI promotions (§2.7) — JOB-level streak re-measure first;
   FULL-LIST PUT; GET-diff proof; CodeQL only with explicit operator OK. (Carry ×2:
   skipped S12+S13 on the date gate.)
2. **WO-B [L]** WebRTC pion media path (§2.11 phase 2, D-073 triage spec):
   - Phase-2a: pion/webrtc dep added to BOTH server AND qa/mock-ams go.mods (CGO=0
     verified per-dep — any transitive CGO breaks the shipping gate); mock-ams
     wsSignalingHandler rewrite: keep WS open post-offer, exchange takeCandidate, pion
     answerer completes ICE+DTLS (~300-400 LOC, budget as [M] on its own); probe
     continues after offer: SDP answer + candidate exchange → assert ICE-connected;
     new `ice_state` field (contract CR + CH **0007** — 0006 is taken by TTL) —
     explicit INSERT column list + domain + api mapping updated ATOMICALLY (D-072
     positional-append hazard).
   - Phase-2b: hold connection ~2s of RTP, read RTCP receiver reports → rtt/jitter/loss
     (contract CR; resolution/fps only if cheaply available). FIRST to yield if hot.
   - Fixture: live capture of client→server shapes (answer + candidates) from the real
     AMS (the S12 capture is server→client only, "partially-captured").
   - ICE in CI docker is a NEW flake surface (ephemeral UDP) — budget generously ONCE,
     per D-042 read-the-scheduler rule, not repeatedly.
3. **WO-C [M]** OIDC phase 2 (§ D-070 carry) — SPA login screen offers "Sign in with
   SSO" when `/auth/oidc/*` is configured; AuthGate accepts the session cookie (today
   localStorage-token only); e2e or Playwright assertion.
4. **WO-D [M]** anomaly metric expansion (§2.14) — FIRST: ORCH assigns `internal/anomaly`
   a manifest owner (gap noted since D-070). Then extend Detector metrics beyond
   viewer_count/cpu_pct/mem_pct per §2.14 spec.
5. **WO-E [M, operator-gated "ship v0.3.0"]** prod rollout — tag v0.3.0; carries
   D-068 O(N²) + D-070 (PDF logo/anomaly rules/OIDC) + D-072 (Postgres/WebRTC
   probe/brandkit UI) + D-073 (RTMP+DASH probes, TTL fix). §8.8 smoke + runbook;
   rollback tags stand. AFTER rollout: ping operator for browser-accept of the
   re-branded UI (U5 pattern) — prod renders the OLD UI until this ships.
6. **WO-F [S]** probe segment-body cap (D-073 verifier note): io.LimitReader on segment
   fetch in probeHLS + probeDASH, SAME commit; a truncated segment must NOT silently
   produce a wrong bitrate_kbps (detect via LimitReader(N+1) length check → treat as
   read error or skip bitrate; decide + test both probes symmetrically).
7. **WO-G [XS]** enforce_admins/PR-first re-check — same rationale-or-flip rule
   (re-recorded D-072 + D-073; enforce_admins=false, strict, 7 contexts, 1 review).
8. **WO-H [L, operator-gated]** iOS beacon SDK phase 1 (§2.12) — ONLY on explicit
   "need mobile SDKs: yes". Swift package; REST parity with sdk/beacon-js; size gate.

Backlog-if-light: brandkit phase 2 (light theme, §2.15); DASH live-fixture capture (only
if the operator enabled DASH muxing on an AMS app — else leave the D-073 gap note).

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; ci+e2e+codeql GREEN at HEAD (e2e now asserts HLS-implicit + WebRTC +
  RTMP + DASH probe chains).
- Dependabot queue: triage per `docs/dependabot-policy.md`.
- Standings (D-073): Go **74.0%** (floor 70.2; prober 70.1); web
  lines 62.68/branches 58.78/functions 51.54 (gates 59/54/45, vitest-4 — NEVER compare
  to pre-rebaseline notation-artifact numbers like "79.69"); sdk untouched
  (66.06/45.79/70.42; gates 63/43/67; 3.52 KB).
- Prod: **v0.2.0** healthy until WO-E fires. AMS trial license nominally expired
  2026-07-12 (operator-waived — observe + report only).
- U3: if `PULSE_LICENSE_KEY` appeared in `deploy/.env`, restart pulse + live-verify
  beacon→QoE, record.
- Binding rules unchanged: golang:1.25 docker REPO-ROOT mount (D-028); gofmt gate on
  OUTPUT EMPTINESS; `sg docker -c`; pristine-copy compose staging (D-061), unique `-p`;
  commit by explicit path; no subagent reverts (D-063); contracts frozen — CR via INT-01
  (D-004); serial wiring author for shared prober/mock-ams files (D-073 pattern: protocol
  authors = NEW FILES ONLY); adversarial verify BEFORE push (D-072/D-073 precedent —
  D-073's verify caught the DASH BaseURL-chain gap + live-evidenced the RTMP S2-echo
  against real AMS); e2e poll conditions: omission semantics BINDING (.get(key, default);
  error_code/bitrate_kbps/connect_time_ms have KEY-ABSENT cases).

## Gates (ORCH, before any commit)

- Contract CR → redocly + ajv + gen:api drift (§8.6). CH 0007 → explicit INSERT column
  list + Append args updated in the SAME commit (D-072 hazard).
- Go → full `-race` repo-root mount, floor 70.2, 0 FAIL/0 unexpected SKIP, gofmt
  emptiness, CGO_ENABLED=0 build (pion deps make this gate NON-TRIVIAL this session —
  run it EARLY, not just at the end).
- Web touched → lint + typecheck + coverage gates + build.
- e2e.yml touched → yaml parse + STATIC per-key cross-check of every poll condition
  against wave3.go probeResultToAPI (omission semantics).
- Prod untouched unless WO-E fires.

## Closing protocol (ROADMAP §6, unchanged — plus the D-072 lesson)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-074 close evidence — **append EARLY in the close, commit handoffs
   FIRST, trail the cheap steps** (D-072 interruption lesson: the ledger must not be
   the casualty of a terminal crash).
3. RESUME-PROMPT ▶ START HERE → SESSION-15; ROADMAP-V2 §3/§4/§5 ledgers updated.
4. REFRESH `docs/operator-expected.md` + PushNotification at completion.
5. Write `sessions/SESSION-15.md` from ROADMAP-V2 §3.
