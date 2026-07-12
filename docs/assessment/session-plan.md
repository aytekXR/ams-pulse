# Session Plan — Validation Program Execution

**Produced:** S16 close (2026-07-11)
**Updated:** Each session ORCH-00 updates the Status column

This document maps the 8 validation phases to sessions (S17, S18, S19+),
defines dependencies, estimates effort, and separates autonomous agent
work from tasks that require operator involvement.

---

## Phase Status Summary

| Phase | Name | Status | Target | Assigned |
|-------|------|--------|--------|---------|
| 1 | Product Understanding | DONE (S17: AV triage — 9 CONFIRMED live, see capability-map § AV Triage) | S16 | WRITER |
| 2 | Test Environment | DONE (S17: `qa/realams/` harness + Makefile, D-079) | S17 | BE agent |
| 3 | E2E User Scenarios | DONE for P0+P1 (S18: P0 25/1/0, P1 21/3/0; remaining P2 + remote-viewer/SRT items → S19+ backlog) | S17–S18 | BE agent + ORCH |
| 4 | Automated Validation Scripts | DONE (`make validate-realams-p0` + `validate-p1`, 50 scenarios) | S17–S18 | BE agent |
| 5 | Bug Investigation | ROLLING (BUG-001..010 filed; **002 FIXED S23 (VoD REST poll, live-validated TC-REC-01), 003/004 FIXED S20, 005 FIXED S21, 006/007/010 FIXED S22, 008 FIXED S22+S24 (Group A D-084, Group B D-086 — flagHistoryBridge + anomaly_flag_events), 009 PARTIALLY FIXED S22** — declared-param violations 27→2, all pinned by `param_conformance_test.go` (37 probes): 009 tenant ×2 → F6 multi-tenancy; only 001 (low) open) | S17+ | Any |
| 6 | Documentation Program | TOP-3 AUTHORED (S19: DG-04 + DG-11 → AMS-INTEGRATION.md, DG-07 → docs/beacon-sdk.md); remaining gaps carry | S18–S19+ | DOC agent |
| 7 | PRD Validation Matrix | DONE (S19: prd-validation-matrix.md — 66 sub-rows F1–F10 + 36 numeric criteria N1–N36, all evidence-cited, adversarially verified) | S19 | QA + ORCH |
| 8 | Final Assessment | DRAFTED (S19: final-assessment.md — DRAFT banner; operator review REQUIRED before external use; 5 NEEDS-OPERATOR-CONTACT rows) | S19+ | ORCH |

---

## S17 — Environment + Core Scenarios

**Goal:** Working harness + all P0 scenarios executed and documented.
**Effort estimate:** 1 full agent session (6–8 h with evidence capture).

### Tasks

#### Phase 2 — Harness Implementation

| Task | Autonomous | Operator needed |
|------|-----------|----------------|
| Create `qa/realams/harness/` directory and 5 helper scripts | Yes | No |
| Write `qa/realams/Makefile` with `validate-all`, `validate-p0` targets | Yes | No |
| Test `auth.sh` against real AMS (`admin@` credentials from deploy/.env) | Yes | No |
| Test `publisher.sh` with ffmpeg Docker container | Yes | Confirm docker permission |
| Document AMS trial license expiry status | Yes | Operator confirms if renewed |

**Dependency:** `deploy/.env` must contain `PULSE_AMS_USER`, `PULSE_AMS_PASS`,
and the `PULSE_TOKEN` (`plt_0352...` from oguz-testing.md line 159).
ORCH-00 must not write these values to any committed file.

#### Phase 3+4 — P0 Scenarios (first half)

Execute and evidence the following P0 scenarios:
- TC-L-01 (broadcast lifecycle), TC-L-02 (concurrent broadcasts),
  TC-L-03 (publisher crash), TC-F-01 (graceful stop), TC-F-02 (abrupt kill)
- TC-V-01 (HLS viewer count), TC-V-02 (WebRTC viewer count),
  TC-V-03 (cross-check), TC-V-04 (RTMP -1 clamp)
- TC-I-01 (normal ingest), TC-I-02 (bitrate conversion), TC-I-06 (FPS=0)
- TC-H-01 (fleet standalone), TC-H-02 (healthz)
- TC-P-01 (WebRTC probe), TC-P-03 (RTMP probe), TC-P-04 (HLS probe),
  TC-P-05 (HLS 404), TC-P-06 (DASH 404 expected)
- TC-WH-01 (webhook audit), TC-WH-02 (poll covers webhook gap),
  TC-WH-03 (VoD recording gap)
- TC-FL-01 (standalone node), TC-FL-02 (version display)
- TC-APP-01 (multi-app polling), TC-APP-02 (403 handling)

**Evidence output:** `qa/realams/evidence/S17-*/` with verdict.txt per scenario.

#### Phase 5 — Bug Investigation (rolling from S17)

For each FAIL verdict from a P0 scenario: file
`docs/assessment/bugs/BUG-NNN-<slug>.md` using the template in
`docs/assessment/README.md` § Bug Document Template.

Bugs expected from known gaps:
- `BUG-001-broadcast-statistics-dead-code` — AV-02: BroadcastStatistics
  never called; confirm severity
- `BUG-002-recording-gb-zero-webhook-blocked` — TC-VOD-01 / TC-WH-03:
  VoD recording bytes always 0; root cause: webhook unsigned
- Any additional bugs found during P0 scenario runs

### S17 Success Criteria

- `make validate-realams-p0` runs without script errors
- All 24 P0 scenarios have a verdict.txt (PASS or FAIL with evidence)
- At least 18/24 P0 scenarios PASS (≥75% pass rate before escalation)
- All FAIL cases have a filed BUG-NNN document

### S17 Dependencies Not Yet Resolved

1. **AMS trial license (expires 2026-07-12):** If the trial has lapsed,
   Enterprise AMS features may be unavailable. Operator decision: renew,
   replace, or document which scenarios are blocked. Session observes and
   reports. AMS streams and basic REST API are available regardless of
   license tier.

2. **docker permission for ffmpeg publisher:** The ffmpeg container must
   run in the same Docker context as the VPS. Confirm `sg docker` is
   sufficient or whether a group membership change is needed.

3. **PULSE_AMS_APPLICATIONS env:** Confirm whether it is set in
   `deploy/.env` (filters which apps Pulse polls). If set to only
   `LiveApp`, the TC-APP-01 multi-app test needs adjustment.

---

## S18 — P1 Scenarios + Documentation Gap List

**Goal:** All P1 scenarios run; documentation gap list complete.
**Effort estimate:** 1 full agent session.

### Tasks

#### Phase 3+4 — P1 Scenarios

Execute P1 scenarios not covered in S17:
- TC-L-04 (rapid cycling), TC-L-05 (simultaneous start/stop)
- TC-V-05 (viewer ramp), TC-V-06 (viewer leave), TC-V-07 (WebRTC per-peer stats),
  TC-V-08 (RTT unit conversion), TC-V-09 (BroadcastStatistics dead-code confirm)
- TC-I-04 (bitrate drop / health degradation), TC-I-05 (packet loss),
  TC-I-07 (drop counts)
- TC-F-05 (AMS session expiry), TC-F-06 (invalid stream key),
  TC-F-08 (network reconnect)
- TC-H-04 (alert: bitrate floor), TC-H-05 (alert: viewer count),
  TC-H-06 (alert: standalone CPU empty)
- TC-S-01 (20 concurrent publishers)
- TC-P-07 (probe interval), TC-P-08 (multiple simultaneous probes)
- TC-A-05 (QoE startup time), TC-A-06 (QoE rebuffer ratio),
  TC-A-08 (egress always 0), TC-A-09 (recording always 0)
- TC-AN-03 (CPU/mem empty standalone), TC-AN-05 (error_rate not tracked)
- TC-V-10 (beacon QoE real viewer) — requires headless Playwright session

#### Phase 6 — Documentation Gap List

Based on P0+P1 findings, compile the list of documentation gaps. Each
TC-DOC-* scenario entry in the scenario matrix maps to a documentation
action. Produce:

```
docs/assessment/documentation-gaps.md   (in-session artifact)
```

Columns: Gap ID, Description, Target Document, Severity (missing entirely
/ incomplete / misleading), Sample user question it would answer.

Expected gaps (from capability map):
- HLS viewer count CDN degradation (TC-DOC-01)
- RTMP pull count = -1 semantics (TC-DOC-02)
- FPS always 0 on AMS 3.x REST (TC-DOC-03)
- Webhook + AMS 3.x signing limitation (TC-DOC-04)
- CPU/mem standalone unavailability (TC-DOC-05)
- Egress bytes placeholder (TC-DOC-06)
- Beacon SDK integration path (not covered in current AMS-INTEGRATION.md)
- Per-app CIDR configuration on AMS (operators must manually open each app)
- Multi-user lockout avoidance (admin@ vs aytek@ account strategy)

#### Phase 5 — Additional Bug Filing

File bugs for any P1 FAIL verdicts not covered by S17 bug docs.

### S18 Dependencies

1. **Real WebRTC viewer for TC-V-07/V-08:** Requires Playwright with
   WebRTC support in headless Chromium. Confirm `npx playwright install chromium`
   available on VPS or use a local machine targeting the prod AMS URL.

2. **Anomaly baseline warmup for TC-AN-01/AN-02:** 30-minute warmup
   period in prod (60 s tick × 30 samples minimum). Either:
   - Schedule the warmup as a background task and check results at session
     end, or
   - Accept CI-proven (PULSE_ANOMALY_TICK_S=5, 2.5 min warmup) as a
     proxy and document why prod warmup is impractical in a session.

3. **Kafka path exploration:** If the operator wants CPU/mem for standalone
   AMS, the Kafka path (`PULSE_KAFKA_BROKERS`) must be configured. This
   requires a Kafka instance accessible from AMS. Operator decision.

### S18 Success Criteria

- All P1 scenarios have a verdict.txt
- `docs/assessment/documentation-gaps.md` produced
- All new bugs filed
- P0 regressions re-run if S17 left any FAIL on a now-fixed bug

---

## S19 — PRD Matrix + Final Assessment Draft

**Goal:** PRD validation matrix complete; final assessment drafted.
**Effort estimate:** 1.5 agent sessions.

### Phase 7 — PRD Validation Matrix

Source: `docs/prd-report.md` §7 — PRD feature list F1–F10 and the numeric
acceptance criteria from `docs/ARCHITECTURE.md` §4.

For each PRD requirement, produce a verdict in one of:

| Verdict | Meaning |
|---------|---------|
| FULLY | Requirement implemented and validated end-to-end |
| PARTIALLY | Core implemented; edge case missing or approximated |
| MISSING | Requirement in PRD; not implemented in Pulse |
| DIFFERENTLY | Implemented but not as the PRD specified; document the delta |
| NEEDS-CLARIFICATION | Requirement is ambiguous; Ant Media team input needed |

Produce: `docs/assessment/prd-validation-matrix.md`

Columns: PRD Feature ID, Requirement text, Verdict, Evidence (scenario ID
or reference), Notes (WHY the verdict; specific gaps or deviations).

#### Pre-populated verdicts from capability map

| PRD Feature | Provisional Verdict | Why |
|-------------|-------------------|-----|
| F1 — Live stream monitoring | FULLY | TC-L-01/02 proven in CI; real-AMS P0 confirms |
| F2 — Audience analytics | PARTIALLY | Viewer counts proven; beacon QoE requires real viewers |
| F3 — Fleet/node health | PARTIALLY | Cluster mode FULL; standalone REST has no CPU/mem |
| F4 — Ingest health | PARTIALLY | Bitrate/loss/RTT work; FPS always 0 on AMS 3.x REST |
| F5 — Alerting | FULLY | CI-proven A1–A5b; real-AMS P0 validates threshold rules |
| F6 — Usage/billing reports | PARTIALLY | viewer_minutes works; egress=0; recording=0 |
| F7 — Probes | FULLY | WebRTC/RTMP/HLS proven; DASH missing (AMS disabled) |
| F8 — Prometheus export | PARTIALLY | Endpoint exists; metric coverage to validate in TC-H-03 |
| F9 — Anomaly detection | PARTIALLY | Viewer/bitrate/CPU(cluster) tracked; error_rate/rebuffer_ratio NOT |
| F10 — Webhook events | MISSING (prod) | AMS 3.0.3 cannot sign hooks; fail-closed listener useless |

### Phase 8 — Final Assessment

Produce: `docs/assessment/final-assessment.md`

Sections:
1. **Executive Summary** (1 page for Ant Media team)
2. **Product Completeness Score** — % of PRD requirements fully met
3. **Marketplace Readiness Checklist** — against antmedia.io/marketplace
   listing requirements
4. **Missing Opportunities** — capabilities AMS provides that Pulse does
   not yet consume (Kafka CPU/mem, object detection, SRT-specific loss,
   scheduled stream pre-alert, WHIP viewer counts)
5. **Prioritized Roadmap** — items ranked by customer value × complexity
   × marketplace differentiation, using the format:

   | Priority | Item | Customer Value | Complexity | Marketplace Impact |
   |---------|------|---------------|------------|-------------------|
   | P0 | unsigned webhook ingest mode (O3 D-V2-1) | High | Medium | High |
   | P0 | VoD recording_gb via REST poll fallback | High | Low | Medium |
   | P1 | FPS via Kafka/analytics-log integration | Medium | High | Medium |
   | P1 | error_rate + rebuffer_ratio anomaly signals | Medium | Medium | High |
   | P1 | Standalone CPU/mem via Kafka | Medium | High | Medium |
   | P2 | RTMP pull viewer count via /{app}/connections | Low | Low | Low |
   | P2 | Scheduled stream pre-event alerting | Medium | Medium | Medium |

6. **Open Questions for Ant Media Team** — items that need AMS team input
   to resolve (webhook HMAC, cluster phantom viewer count, WHEP viewers,
   analytics log FPS field)

### S19 Dependencies

1. **All S17+S18 scenario verdicts finalized.** The PRD matrix draws from
   scenario evidence packages.
2. **Operator review of documentation gaps (S18 output).** Some gaps may
   be already documented in AMS docs; operator cross-check avoids
   duplicating existing AMS documentation.
3. **Ant Media team contacts** for the marketplace listing and
   co-marketing blog post process. Operator to initiate.

---

## Dependency Graph

```
S16: Phase 1 (capability-map) ─────────────────────────────┐
                                                            │
S17: Phase 2 (harness) ──► Phase 3+4 P0 ──► Phase 5 bugs  │
          ↓                                                 │
S18: Phase 3+4 P1 ──► Phase 5 bugs ──► Phase 6 doc gaps   │
          ↓                                                 │
S19: Phase 7 (PRD matrix) ◄── all scenario verdicts ◄──────┘
          ↓
     Phase 8 (final assessment)
```

---

## Operator vs. Autonomous Work Split

### Operator must:

| Task | Why operator only |
|------|------------------|
| Confirm AMS trial license status / renewal | Requires Ant Media account |
| Provide AMS credentials if rotated | Security; stored in gitignored deploy/.env |
| Decide on Kafka integration (standalone CPU/mem) | Requires Kafka infra decision |
| Decide on unsigned webhook ingest mode (D-V2-1) | Open operator decision |
| Initiate Ant Media marketplace listing contact | Business relationship |
| Review final-assessment.md before sharing with Ant Media | QA of external-facing content |
| Confirm browser-accept of re-branded UI (open item from operator-expected.md) | Human UI review |
| Confirm `PULSE_AMS_APPLICATIONS` filter setting for S17 scenarios | Config knowledge |

### Autonomous agents can:

| Task | Why autonomous |
|------|---------------|
| Write and run harness scripts (P0+P1 scenarios) | Scripted; no secrets beyond deploy/.env |
| File bug documents from FAIL verdicts | Structured template |
| Compile documentation gap list | Derived from scenario findings |
| Draft PRD validation matrix | Derived from scenario + capability map |
| Draft final assessment roadmap | Derived from validated data |
| Run prober scenarios (WebRTC/RTMP/HLS probes) | No operator interaction needed |
| Cross-check AMS vs. Pulse API numeric values | Pure API calls |

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| AMS trial license expired (2026-07-12) limits Enterprise features | High | Low | Operator-waived; observe-report; document blocked features |
| AMS brute-force lockout during automated tests | Medium | Medium | `auth.sh` called once per session; separate admin@ and aytek@ |
| Real WebRTC viewers hard to simulate headlessly | Medium | Medium | Use Playwright + Chromium; fallback to mock data for unit test |
| Anomaly baseline warmup (30 min in prod) slows S18 | High | Low | Pre-start publisher 30 min before session; or accept CI proxy |
| VoD recording gap blocks billing use case | High | High | File BUG-002; add to roadmap as P0 fix (REST poll fallback) |
| Webhook gap blocks real-time stream events | High | Medium | Confirmed by O3; REST poll covers start/end within 15 s |
| docker permission for ffmpeg publisher on VPS | Low | High | Confirm `sg docker` group in pre-session check |
| S19 PRD matrix lacks enough scenario evidence | Low | Medium | Track scenario count coverage in ORCH notes before S19 |

---

## Session Handoff Checklist

At the end of each session, ORCH-00 should verify:

**S17 close:**
- [ ] `qa/realams/harness/` scripts exist and are tested
- [ ] `make validate-realams-p0` exits 0 or failing scenarios documented
- [ ] All P0 FAIL verdicts have a BUG-NNN.md file
- [ ] `session-plan.md` Status column updated for S17 rows
- [ ] Commit and push (up to 2 pushes, per operator directive)

**S18 close:**
- [ ] All P1 scenarios have verdict.txt
- [ ] `docs/assessment/documentation-gaps.md` exists
- [ ] `session-plan.md` Status column updated for S18 rows
- [ ] No open BUG-NNN without severity assignment

**S19 close:**
- [x] `docs/assessment/prd-validation-matrix.md` complete (all F1–F10 + N1–N36)
- [x] `docs/assessment/final-assessment.md` drafted (DRAFT banner + operator gate)
- [x] Operator review requested before sharing with Ant Media (operator-expected.md)
- [x] Session-plan.md updated with final status of all phases
