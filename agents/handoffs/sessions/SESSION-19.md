# SESSION-19 — D-078 Phases 7+8: PRD validation matrix + final assessment draft (ROADMAP-V2 S19, planned at S18 close)

> Written by SESSION-18 close (D-080, 2026-07-11). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §3 S19 + `RESUME-PROMPT.md` §7/§8/§12 AND
> `docs/assessment/session-plan.md` § S19 + `docs/assessment/documentation-gaps.md`
> (§ authoring plan) before dispatching.

## Mission

Execute ROADMAP-V2 §3 S19. Exit = (a) `docs/assessment/prd-validation-matrix.md`
complete (all F1–F10 + ARCHITECTURE §4 numeric criteria, every verdict
evidence-cited); (b) `docs/assessment/final-assessment.md` DRAFTED (operator
reviews before anything external); (c) top-priority gap docs authored (DG-04,
DG-07, DG-11 first); (d) CI promotions if run date ≥2026-07-23 (else skip carry
×8); (e) recurring re-checks + operator-answer sweep. PR-first, ≤2 pushes.

## Context you must load first

- **Evidence base (cite, don't re-run):** P0 25 PASS / 1 SKIP; P1 21 PASS /
  3 SKIP / 0 FAIL — 50 scenario scripts under `qa/realams/scenarios/`, evidence
  conventions in `qa/realams/README.md`. Bugs: BUG-001 (dead code), BUG-002
  (recording_gb=0, webhook-blocked; fix = VoD REST poll), **BUG-003 (probe
  scheduler duplicate results — real)**, **BUG-004 (/qoe/ingest ignores declared
  from/to — contract violation)**. AV triage: capability-map § AV Triage — S17.
- **FIRST ACTION — post-expiry sweep:** the AMS trial license lapsed
  2026-07-12T12:09Z (operator-waived, observe-report). Read-only sweep: version
  endpoint, app-scope REST, WebRTC/HLS serving, enterprise-feature 403s. Record
  the delta in decisions.md D-081 + note which scenarios would now be blocked.
  S17/S18 runs were pre-expiry — they are the baseline.
- **Known env limits (verdicts depend on them):** VPS AMS caps at ~5–7 concurrent
  RTMP streams → TC-S-01/TC-L-05 ENV-LIMIT (stress claims in the matrix must say
  "validated to N=5; N=20 needs larger instance"); hlsViewerCount is a sliding
  request-window metric (~9× session inflation, >90 s expiry lag — DG-01);
  RTMP/TCP masks packet loss (DG-18); same-host WebRTC viewers yield all-zero
  RTT (non-zero viewer-QoE parity needs a REMOTE viewer — backlog); AMS
  settings mutate via POST; applications/info is 405.
- **Harness discipline:** memory `shell-harness-false-green-patterns` (now 5
  landmines) is BINDING for any new/edited bash; runner PASS requires fresh
  verdict.txt.
- Operator answers to sweep from `docs/operator-expected.md`: AMS-reset
  confirmation (S17), Kafka yes/no, marketplace contact (needed for Phase 8's
  listing-requirements section — if absent, mark those rows NEEDS-OPERATOR),
  brandkit token proposals sign-off, browser-accept.

## Work orders

1. **WO-A [M, PRIMARY]** Phase 7 — `docs/assessment/prd-validation-matrix.md`:
   for each PRD feature F1–F10 (docs/prd-report.md §7) and each ARCHITECTURE §4
   numeric criterion: verdict FULLY / PARTIALLY / MISSING / DIFFERENTLY /
   NEEDS-CLARIFICATION + Evidence column (TC-x / BUG-x / AV-x / decisions ref) +
   WHY notes. Start from session-plan.md § S19 pre-populated verdicts but
   re-derive each from actual S17/S18 evidence (several changed: F7 probes now
   FULLY incl. rtt/jitter/loss but carries BUG-003; F4 ingest health carries
   BUG-004 + FPS gap; F6 usage carries egress-estimate semantics + BUG-002).
2. **WO-B [M]** Phase 8 — `docs/assessment/final-assessment.md` DRAFT per
   README.md Phase-8 spec: exec summary (1 page), completeness score (computed
   from the matrix), marketplace-readiness checklist (rows needing the Ant Media
   listing requirements → NEEDS-OPERATOR-CONTACT), missing opportunities, the
   prioritized roadmap table (VoD REST poll P0, unsigned-webhook D-V2-1, Kafka
   CPU/mem, error_rate/rebuffer anomaly signals, SRT loss, remote-viewer QoE),
   open questions for the Ant Media team. Mark clearly DRAFT — operator review
   gate before external use.
3. **WO-C [S–M]** Phase 6 authoring, top of the documentation-gaps plan:
   DG-04 (webhook limitation → AMS-INTEGRATION §4.5 expansion), DG-07 (new
   docs/beacon-sdk.md), DG-11 (lifecycle deleted-on-stop semantics →
   AMS-INTEGRATION §1.1). More only if light.
4. **WO-D [S, gate ≥2026-07-23]** CI promotions: JOB-streak re-measure FIRST;
   if open → csp-e2e FULL-LIST PUT + GET-diff proof; web-e2e earliest ~07-25;
   else skip carry ×8.
5. **WO-E [XS]** standing re-checks (protection drift, dependabot, prod health
   read-only, prod untouched) + operator-answer sweep (see context).

*(Backlog-if-light: remote-host WebRTC viewer (a second VPS or operator laptop)
for non-zero viewer-QoE parity; SRT publisher for real loss validation; BUG-002
VoD REST-poll design note; TC-APP-02 if a blocked test app appeared.)*

## Gates (ORCH, before any commit)

- Docs-only session expected: no Go/web gates unless code is touched (then §8
  full set). Any new/edited harness bash → bash -n + shellcheck + the 5
  false-green patterns.
- Prod untouched (read-only). AMS: read-only except nothing — no settings
  changes planned this session.
- final-assessment.md carries a DRAFT banner + "operator review required before
  sharing" — do NOT push any external communication.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-081 evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-20; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` (incl. final-assessment review request —
   that one IS an operator action) + `docs/assessment/session-plan.md` +
   PushNotification.
5. Write `sessions/SESSION-20.md` (likely: operator-review intake + assessment
   finalization + remaining P2/backlog scenarios).
