# SESSION-13 — probe protocol completion (RTMP + DASH + WebRTC media path) + CI promotions (ROADMAP-V2 S13, revised at S12 close)

> Written by SESSION-12 close (D-072, 2026-07-10). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP-V2.md`
> (plan of record) + `RESUME-PROMPT.md` §7/§8/§12 before dispatching.
> ⚠ CHECK `docs/operator-expected.md` ANSWERS FIRST — S13 has FOUR operator-answer-dependent
> switches: (1) "ship v0.3.0" → WO-E prod rollout fires; (2) CodeQL yes/no → WO-A shape;
> (3) PR-first yes/no → enforce_admins re-arm; (4) mobile-SDK need (operator was asked at S12
> whether native iOS/Android apps exist — if "drop mobile SDKs", delete §2.12 from ROADMAP-V2
> and re-plan S14; iOS work does NOT start without an explicit yes).
> ⚠ S13 SCOPE REVISION (D-072): the original S13 sketch (RTMP+DASH + iOS SDK) collided with
> the WebRTC phase-2 carry added by D-072's re-scoping. Ruling: S13 = PROBE COMPLETION,
> mobile SDKs move to S14 (operator-gated). Triage order if hot: WO-D (pion) yields first,
> then WO-C (DASH).

## Mission

Execute ROADMAP-V2 §3 S13 (revised). Exit = (a) CI promotions decided-and-applied or
re-gated with evidence (web-e2e/csp-e2e streak re-measure; CodeQL only with operator OK);
(b) RTMP probe returns a real handshake result (not `not_probed`) in CI; (c) DASH probe
returns a real manifest+segment result in CI; (d) WebRTC pion phase 2 lands OR is
explicitly re-gated to S14 with the triage record; (e) probe_results TTL honors
`{retention_days}` (new CH migration; D-072 verifier finding); (f) date-gated/conditional
carries executed or re-recorded.

## Work orders

1. **WO-A [S, date-gated ≥2026-07-23]** CI promotions (§2.7) — JOB-level streak re-measure
   first; FULL-LIST PUT; GET-diff proof; CodeQL only with explicit operator OK.
2. **WO-B [M]** RTMP probe phase 1 (§2.11) — pure-Go RTMP handshake probe (C0/C1/C2 +
   connect, stdlib-only preferred; measure handshake_time_ms), contract CR for any new
   fields (INT-01 single writer), mock-ams RTMP listener or fixture-replay, CI assertion.
   The D-072 slice pattern (signaling/handshake-level first, media later) applies.
3. **WO-C [S–M]** DASH probe (§2.11) — closest to HLS: MPD manifest fetch + first segment;
   reuse the HLS probe measurement shape (ttfb/bitrate/segment_ttfb); CI fixture.
4. **WO-D [L]** WebRTC probe phase 2 (§2.11, D-072 carry) — pion/webrtc media path
   (ICE/DTLS/SRTP), rtt/jitter/loss/resolution/fps fields (contract CR), CGO=0 verified;
   full fixture capture from the real AMS (the S12 fixture has signaling shapes;
   phase 2 needs candidate/DTLS exchanges). FIRST candidate to yield if the session
   runs hot.
5. **WO-E [M, operator-gated "ship v0.3.0"]** prod rollout — tag v0.3.0 (release pipeline
   proven D-058/D-067), carries: O(N²) fix (D-068), S11 features (D-070), S12 Postgres/
   WebRTC-probe/brandkit UI (D-072). §8.8 smoke + runbook; rollback tags stand. After
   rollout ping the operator for the U5-pattern browser check of the re-branded UI.
6. **WO-F [XS]** probe_results TTL fix (D-072 verifier finding #6) — new CH migration
   `0006_probe_results_ttl.sql`: `ALTER TABLE {db}.probe_results MODIFY TTL toDate(ts) +
   toIntervalDay({retention_days})`; verify the migration runner substitutes
   `{retention_days}` (it does for 0001 tables — confirm mechanism), integration-test it.
7. **WO-G [XS]** enforce_admins / PR-first re-check (§2.1) — same rationale-or-flip rule.

Backlog seeded but NOT S13 (pick up only if light): §2.14 anomaly metric expansion
(`internal/anomaly` still needs a manifest owner); OIDC phase 2 (SPA login UI); light-theme
brandkit phase 2.

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; ci+e2e+codeql GREEN at HEAD (e2e now includes the WebRTC probe step).
- Dependabot queue: triage per `docs/dependabot-policy.md`.
- Standings (D-072): Go **73.9%** (floor 70.2; meta 67.9→ratchet candidate); web
  lines 62.68/branches 58.78/functions 51.54 (gates 59/54/45 — vitest-4 instrumentation,
  never compare to pre-rebaseline handoff numbers); sdk untouched (63/43/67; 3.52 KB).
- Prod: **v0.2.0** (pre-D-068) healthy until WO-E fires. Backup keep-7 verified cycle-8
  (D-072). AMS trial license nominally expired 2026-07-12 (operator-waived — observe).
- U3: if `PULSE_LICENSE_KEY` appeared in `deploy/.env`, restart pulse + live-verify
  beacon→QoE, record.
- Binding rules unchanged: golang:1.25 docker REPO-ROOT mount (D-028); gofmt gate on
  OUTPUT EMPTINESS; `sg docker -c`; pristine-copy compose staging (D-061), unique `-p`;
  commit by explicit path; no subagent reverts (D-063); contracts frozen — CR via INT-01
  (D-004); serial wiring author for cmd/pulse (D-070); adversarial verify workflow before
  push (D-072 caught a CRITICAL e2e inversion this way — keep it).

## Gates (ORCH, before any commit)

- Contract CR → redocly + ajv + gen:api drift (§8.6).
- Go → full `-race` repo-root mount, floor 70.2, 0 FAIL/0 unexpected SKIP, gofmt
  emptiness, CGO_ENABLED=0 build.
- Web touched → lint + typecheck + coverage gates + build.
- e2e.yml touched → yaml parse + STATIC cross-check of every poll condition against the
  actual API response shape (D-072: `get('error_code','not_probed')` was always-False on
  success because the key is omitted — verify omission semantics, not just names).
- Prod untouched unless WO-E fires.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope; push; `gh run watch` ci AND e2e AND codeql green.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-073 (per-WO evidence incl. skip/yield records).
3. RESUME-PROMPT ▶ START HERE → SESSION-14; ROADMAP-V2 §3/§4/§5 ledgers updated.
4. REFRESH `docs/operator-expected.md` + PushNotification at completion.
5. Write `sessions/SESSION-14.md` from ROADMAP-V2 §3 (content depends on the mobile-SDK
   answer: either iOS SDK phase 1 or OIDC phase 2 + anomaly expansion).
