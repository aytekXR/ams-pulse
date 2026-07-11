# SESSION-18 — D-078 program Phases 3+4 P1 scenarios + Phase 6 doc-gap list (ROADMAP-V2 S18, planned at S17 close)

> Written by SESSION-17 close (D-079, 2026-07-11). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` + `RESUME-PROMPT.md` §7/§8/§12 AND
> `docs/assessment/session-plan.md` § S18 + `docs/assessment/scenario-matrix.md`
> (**including the ⚠ S17 Corrections block — the S16 rows embed refuted
> assumptions; the corrections are binding**) before dispatching.

## Mission

Execute ROADMAP-V2 §3 S18. Exit = (a) D-078 Phases 3+4 P1 scenarios executed with
evidence (see list below); (b) Phase 6 `docs/assessment/documentation-gaps.md`
produced; (c) any new FAIL → BUG-NNN doc; (d) post-07-12 AMS trial-license
behavior observed and recorded (observe-report only, operator waived); (e) CI
promotions IF run date ≥ 2026-07-23 (else skip carry ×7 with streak re-measure);
(f) recurring re-checks. ALL WORK VIA PRs, ≤2 pushes (D-076).

## Context you must load first

- **S17 landed (D-079):** `qa/realams/` harness + 26 P0 scenario scripts;
  P0 result 25 PASS / 1 SKIP (TC-APP-02 — no IP-blocked app exists to trigger it).
  Headline parity: stream start→Pulse 4 s, stop→Pulse 7 s (PRD ≤10 s); bitrate
  ±10% parity; WebRTC/RTMP/HLS probes live-green; fleet/standalone honest-absent
  semantics hold. Bugs: BUG-001 (BroadcastStatistics dead code), BUG-002
  (recording_gb=0, webhook-blocked). Run 1 of the suite false-greened 17
  scenarios (auth.sh exit-on-source) — the runner now requires a fresh
  verdict.txt for PASS; keep that property.
- **Live-verified AMS drift (S17 corrections, scenario-matrix.md top):** HLS at
  flat `/{id}.m3u8`; implicit RTMP broadcasts DELETED on stop (404, never
  `finished`); app inventory 16→4 (all open — operator asked to confirm the
  reset was theirs, check operator-expected.md for their answer);
  `applications/info` → 405; `versionType`="Enterprise Edition"; one test VoD
  now exists on `pulse-test` (S17-created ground truth).
- **AMS trial license expired 2026-07-12T12:09Z** (operator-waived, "handled").
  First S18 action: read-only sweep — what still answers, what 403s; record
  which scenarios are blocked. AMS streams + basic REST survive tier loss.
- Harness how-to: `qa/realams/README.md`. Auth lockout rules unchanged (2 fails
  = 5-min email-keyed lock; admin@ is prod's poller — auth.sh once, never retry).
- Playwright ONLY in docker (`mcr.microsoft.com/playwright:v1.61.1-noble`).
  Docker root artifacts → alpine-container cleanup.

## Work orders

1. **WO-A [M–L, PRIMARY]** P1 scenarios per session-plan.md § S18, using the
   existing harness (add `qa/realams/scenarios/` scripts following the P0
   pattern — source-safe, `|| true` asserts, verdict.txt, EXIT traps):
   - Lifecycle: TC-L-04 (rapid cycling), TC-L-05 (simultaneous start/stop).
   - Viewers: TC-V-05 (ramp 10→30), TC-V-06 (join/leave), TC-V-07 (per-peer
     stats via live WebRTC viewer), TC-V-08 (RTT unit ×1000 check — pin the
     exact conversion), TC-V-09 (BroadcastStatistics dead-code grep — evidence
     already in BUG-001, make it a scripted check).
   - Ingest: TC-I-04 (bitrate drop → health_score degradation), TC-I-05 (loss,
     if injectable without sudo), TC-I-07 (drop counts stored-vs-dropped).
   - Failure: TC-F-05 (AMS session expiry → Pulse re-auth; needs AMS restart —
     OPERATOR-COORDINATED or FORCE_DISRUPT=1 + prod-poll-outage awareness),
     TC-F-06 (invalid stream key — needs publishTokenControl on pulse-test),
     TC-F-08 (network reconnect).
   - Health: TC-H-04 (ingest_bitrate_floor alert on realams stack), TC-H-05
     (viewer_count alert), TC-H-06 (cpu anomaly empty on standalone).
   - Stress: TC-S-01 (20 publishers — tell the operator before the run, per
     the load heads-up in operator-expected.md).
   - Probes: TC-P-07 (interval consistency), TC-P-08 (3 concurrent probes).
   - Analytics: TC-A-05/A-06 (beacon QoE via realams beacon port 18091),
     TC-A-08 (egress=0), TC-A-09 (recording=0 — reuse the pulse-test VoD).
   - Anomaly: TC-AN-03 (standalone empty), TC-AN-05 (error_rate not tracked).
2. **WO-B [M]** Phase 6: `docs/assessment/documentation-gaps.md` — compile from
   P0+P1 evidence per session-plan.md § S18 Phase 6 (TC-DOC-01…06 + beacon
   integration path + per-app CIDR + lockout strategy + the S17 drift items).
3. **WO-C [S, gate ≥2026-07-23]** CI promotions (§2.7): JOB-level streak
   re-measure FIRST (S17 evidence: csp-e2e 30/30 green through 2026-07-11,
   still continue-on-error; web-e2e clock restarted at S16 merge 07-11, so
   earliest ~07-25); if gate open → csp-e2e FULL-LIST PUT + GET-diff proof;
   else skip carry ×7.
4. **WO-D [XS]** standing re-checks: branch protection (9 contexts,
   enforce_admins, strict, 0 reviews), dependabot queue, prod health read-only,
   operator browser-accept ping, AMS-reset confirmation answer (operator-expected).

*(Backlog-if-light: TC-V-10 beacon QoE real viewer; P0 TC-APP-02 if the
operator created a blocked test app; VoD REST poll fallback design note for
BUG-002 — design only, implementation is a ROADMAP item.)*

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; S17 PR merged; ci+e2e+codeql GREEN at HEAD.
- Standings (D-079): Go 74.5% (floor 70.2) — untouched by S17; web
  lines/branches/functions from the S17 gate run (see decisions.md D-079;
  ≥ 63.15/61.66/54.85, gates 59/54/45); sdk untouched (3.52 KB).
- Prod: v0.3.0-4-ge8f8f5f healthy, ENTERPRISE license; prod untouched by S17.
- pulse-realams stack: bring up per qa/realams/README.md (it may have been
  torn down); its bootstrap token auto-extracts from container logs.
- Operator queue: 👀 browser-accept (standing); AMS-reset confirmation (S17);
  optionals D-V2-1/O7/O11/workflow-scope.

## Gates (ORCH, before any commit)

- New harness scripts → shellcheck + the source-safety and fresh-verdict rules
  (memory: shell-harness-false-green-patterns).
- Web/Go touched → the standard §8 gates (repo-root mount, floors, Playwright
  docker if web).
- Prod untouched (read-only) unless the operator asks for a rollout.
- Any AMS-disrupting scenario (TC-F-05 AMS restart) → FORCE_DISRUPT=1 explicit,
  logged, and only after checking no operator streams are live.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-080 evidence — append EARLY, commit handoffs FIRST.
3. RESUME-PROMPT ▶ START HERE → SESSION-19; ROADMAP-V2 §3/§4/§5 ledgers.
4. REFRESH `docs/operator-expected.md` + `docs/assessment/*` progress + PushNotification.
5. Write `sessions/SESSION-19.md` from ROADMAP-V2 §3 + session-plan.md (S19 =
   Phase 7 PRD matrix + Phase 8 final assessment draft).
