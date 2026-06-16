# Pulse — Development Log

Running log of the MVP build session. Maintained by ORCH-00 (orchestrator) as work
progresses. Newest entries at the bottom. Companion file: `IMPLEMENTATION_LOG.md`
(per-feature summary, written at consolidation).

---

## 2026-06-11 — Session start

**Goal:** Implement the full Pulse MVP per PRD (`prd-report.md` §7). All features
F1–F10 in MVP form, functional end-to-end, validated against PRD acceptance
criteria, then consolidated and documented.

**Scope ruling (ORCH-00):** PRD §7.14 stages features across Phases 1–3; the user
directive is "build all features in MVP form, do not skip any PRD-specified
functionality." Therefore waves 1+2 run in full per `agents/manifest.yaml`, and
F9 (anomaly detection) + F10 (synthetic probes) are added in minimal-but-working
form. Recorded in `agents/handoffs/decisions.md`.

**Environment found:**
- macOS arm64, Node v26.0.0, npm 11.12.1 — OK for web/ and sdk/.
- Go toolchain NOT installed → installing via Homebrew.
- Docker NOT installed → Docker Compose deliverables will be authored and
  lint-validated but cannot be executed here. End-to-end verification will use a
  local process stack: pulse binary + ClickHouse single binary (curl install) +
  mock AMS server. Logged as an environment limitation for the compose-up gate.

**Plan:**
1. Understand-phase workflow: parallel readers map the skeleton (server, web, sdk,
   contracts, deploy/CI) and collect `TODO(<AGENT-ID>)` markers.
2. Wave 0 (INFRA-01): build/test/lint targets real and green locally.
3. Wave 1: INT-01 contract freeze → BE-01 ∥ BE-02 ∥ FE-01 → QA-01 → DOC-01.
   Features: F1, F2-core, F5-core, installer, Free-tier licensing.
4. Wave 2: INT-01 → SDK-01 ∥ BE-01 ∥ BE-02 ∥ FE-01 ∥ INFRA-01 → QA-01 → DOC-01.
   Features: F3, F4, F2-full, F6, F7, F8, extra alert channels, Helm.
5. Wave 3-MVP: F9 + F10 minimal.
6. Validation: per-feature acceptance-criteria sweep, adversarial verification,
   defect-fix loop until clean.
7. Consolidation + IMPLEMENTATION_LOG.md + final review notification.

## 2026-06-11 — Session 2: orchestration start

- Go 1.26.4 now installed (session-1 blocker cleared). Docker still absent →
  decision D-002 stands (local process stack for verification).
- ClickHouse single binary v26.6.1 downloaded to `/tmp/clickhouse`.
- Repo placed under git, skeleton committed as baseline (decision D-003); BE-01
  and BE-02 serialized within waves to avoid go.mod write races.
- `agents/handoffs/decisions.md` created with D-001..D-003.
- Understand-phase workflow dispatched: 4 parallel readers (server, contracts,
  web+sdk, infra), structured maps + build-state probes.

### Understand-phase findings (workflow `pulse-understand`, 4 agents)

- **server/**: compiles clean but pure skeleton — every exported type an empty
  struct; only `collector.Source` and `channels.Channel` interfaces defined;
  go.mod has zero deps (intended deps listed as comments).
- **contracts/**: OpenAPI 18 paths / 23 operations, ALL response bodies "TODO";
  3 event schemas have solid envelopes but open `data` objects; both SQL
  migrations are `SELECT 1` placeholders. 10 concrete gaps catalogued (beacon
  ingest path missing, geo/device enrichment unspecified, no error envelope,
  no query params, rollup dims missing, etc.) → folded into WO-101.
- **web/**: React 19 + RR7 + Vite skeleton, no router yet; tsc clean; needs
  openapi-typescript, charting lib, test setup. **sdk/**: stubs build, but
  size gate broken — tsup emits `dist/index.*` while package.json/size-limit
  expect `dist/pulse-beacon.*` → WO-002.
- **infra**: Makefile targets real but `build-web` breaks without prior
  `npm install`; ci.yml has 3 echo-stub jobs (contracts, web, sdk); images
  unpinned; AMS matrix workflow stub → WO-001.

### Orchestration

- Decisions D-004 (single full contract freeze) and D-005 (cmd/pulse assembly
  shared sequentially BE-01→BE-02) recorded.
- Wave 0 dispatched (workflow `pulse-wave-0`): INFRA-01 WO-001 ∥ SDK-01 WO-002,
  then QA-01 gate verification.
- Wave 1 work orders written while wave 0 runs: WO-101 (INT-01 full freeze),
  WO-102 (BE-01 data plane), WO-103 (BE-02 product plane), WO-104 (FE-01
  shell/live/analytics/alerts), WO-105 (QA-01 gate + mock AMS), WO-106 (DOC-01
  install runbook).

## 2026-06-12 — Session 2 end: Wave 0 gate PASS

- User directive: stop after the wave-0 gate; continue next session.
- Wave-0 workflow: INFRA-01 (WO-001) and SDK-01 (WO-002) both DONE, changes
  committed as `df66509`. The QA-01 gate agent was lost to a session
  interruption; criteria are mechanical, so ORCH-00 re-ran them directly
  (one-time protocol deviation, recorded in the gate report).
- **Gate verdict: PASS** — `make build` / `make test` / `make lint` /
  `make validate-contracts` all exit 0; SDK size gate runs (15 kB limit, 88 B
  stub). Details + carried-forward gaps: `agents/handoffs/wave-0/gate-report.md`.
- Task state: #1 understand DONE, #2 wave 0 DONE; #3–#7 pending.

### RESUME POINT (next session)

Full hand-off prompt: `agents/handoffs/RESUME-PROMPT.md`. Short form: dispatch
Wave 1 as one workflow per WO-101..106 — INT-01 freeze first, then
BE-01 → BE-02 sequential (D-003/D-005) ∥ FE-01, then QA-01 gate, then DOC-01.
ClickHouse binary lives at `/tmp/clickhouse` (re-download if /tmp was cleared:
v26.6.1 single binary). No Docker on this machine (D-002).

## 2026-06-12 — Session 3: Wave 1 dispatched

- Resumed per `agents/handoffs/RESUME-PROMPT.md`. Tree clean at `44f02f1`.
- `/tmp` had been cleared → ClickHouse single binary re-downloaded
  (v26.6.1.694, macos-aarch64) to `/tmp/clickhouse` before dispatch.
- Wave 1 workflow dispatched (`pulse-wave-1`, run `wf_23764fb7-417`), per
  D-003/D-005 ordering: INT-01 (WO-101 freeze, alone) → [BE-01 (WO-102) →
  BE-02 (WO-103) sequential] ∥ FE-01 (WO-104) → QA-01 gate (WO-105) →
  DOC-01 (WO-106, dispatched only on PASS / PASS_WITH_LIMITATIONS).
- All agents return structured `{status, reportPath, gaps[],
  cmdEditsDeclared[], changeRequests[]}`; QA returns per-criterion verdicts +
  defect list (owner/severity/repro). Gate decision (fix-loop vs proceed) is
  ORCH-00's on workflow completion.

### Wave 1 results (workflow `pulse-wave-1`, 6 agents, ~140 min)

- **All six agents done.** Reports in `agents/handoffs/wave-1/WO-10x-report.md`;
  gate report `qa/wave-1/gate-report.md`.
- **INT-01 freeze:** OpenAPI 18→32 paths / 46 ops / 66 schemas, zero redocly
  errors/warnings (was 55 warnings); 3 event schemas finalized w/ per-type
  payloads + 9 fixtures; ClickHouse DDL 9 tables + 5 MVs executes on v26.6.1;
  meta DDL 14 tables in SQLite. Rulings: epoch-ms responses / RFC3339-or-epoch
  input; probe results→ClickHouse, probe config+anomaly baselines→meta;
  /metrics unauthenticated by default w/ optional PULSE_METRICS_TOKEN.
- **BE-01:** data plane green (build/vet/test); 10k events inserted in 1.01 s,
  0 drops; stream-visibility 1.50 s measured (10 s budget); 4 cmd/pulse files
  declared w/ HOOK(BE-02) assembly hooks.
- **BE-02:** product plane green; 20 tests / 6 packages, CGO_ENABLED=0;
  kin-openapi conformance on 8 endpoints; alert latency 15 s worst-case by
  construction (30 s budget); AES-256-GCM at-rest secrets; serve.go hooks
  filled (declared per D-005).
- **FE-01:** build/lint/test green (21 tests); all four wave-1 surfaces;
  generated types only (zero hand-rolled API shapes); virtualized streams
  table ≤20 DOM nodes at 500 rows. Filed 3 contract CRs (AlertRule.name,
  AlertRule.enabled, source-test endpoint) + codegen path doc fix.
- **QA-01 gate: PASS_WITH_LIMITATIONS.** 12/12 testable criteria PASS —
  stream visible 1 052 ms (≤10 s), viewer counts 0% error (±2%), alert 15 s
  fake-clock-proven (<30 s), migrate 15 tables in 49 ms, /healthz all ok,
  install <2 min (<15 min), dashboard shows scenario streams. WAIVED:
  compose-up execution (D-002). Defects: D-W1-001 major (node CPU/mem 100×,
  normalize.go) + 5 minors.
- **DOC-01:** install/alerting runbooks + README + ARCHITECTURE updated to
  verified reality; 3 QA doc defects fixed; every documented command re-run.
- **ORCH-00 ruling (D-006):** fix-loop in-wave for D-W1-001/002/003/004/005 +
  approved CRs 1–4 (CR-3 contract-only, impl wave 2); D-W1-006 carried to
  wave 2. Housekeeping: .gitignore for ClickHouse artifacts/binaries, repo-root
  test junk removed. Checkpoint commit before fix-loop dispatch (`408915b`).

### Fix-loop dispatched + Wave 2 work orders written (in parallel)

- Fix-loop workflow `pulse-wave-1-fixloop` (run `wf_e7294343-88a`):
  INT-01 (CR-1..4) ∥ BE-01 (D-W1-001/005) → BE-02 (D-W1-002/003/004 + CR-1/2
  impl) ∥ FE-01 (consume name/enabled) → QA-01 re-gate (per-defect repros +
  full mechanical re-run) → DOC-01 reconcile on pass.
- While it runs (protocol precedent: wave-1 WOs were written during wave 0):
  decision **D-007** (wave-2 structure: no INT-01 step per D-004; beacon
  ingest → BE-02 scope exception; BE-02 split WO-203/WO-204; geo = mmdb
  reader-only; Kafka fake-broker verification waiver) and wave-2 work orders
  **WO-201..WO-208** written to `agents/handoffs/wave-2/`:
  201 SDK-01 F3 beacon SDK · 202 BE-01 Kafka/enrichment/ingest-health/fleet ·
  203 BE-02 beacon-ingest/QoE/Prometheus/channels/gating · 204 BE-02
  reports/exports · 205 FE-01 QoE/ingest/reports/fleet/analytics-full ·
  206 INFRA-01 Helm/compose/CI · 207 QA-01 gate (beacon round trip, billing
  ±1%) · 208 DOC-01 SDK guide/Prometheus guide/runbooks.
- Wave-2 dispatch order (one workflow, after wave-1 closes):
  parallel([BE-01→BE-02(203)→BE-02(204)], SDK-01, FE-01, INFRA-01) → QA-01 →
  DOC-01.

## 2026-06-12 — Session 3 end: Wave 1 GATE CLOSED (fix-loop re-gate PASS_WITH_LIMITATIONS)

- Fix-loop workflow done (6 agents, ~23 min). All five defects verified fixed
  with fresh measurements (re-gate section in `qa/wave-1/gate-report.md`):
  - D-W1-001 ✅ normalize multipliers removed; regression test pins
    cpuUsage=15.0 → cpu_pct=15.0 (was 1500).
  - D-W1-002 ✅ /healthz really probes ClickHouse+meta (3 s timeout, measured
    latency_ms), HTTP 503 when a critical component is down (tested).
  - D-W1-003 ✅ meta DDL embedded (go:embed); `pulse migrate` creates all 14
    meta tables without PULSE_META_DDL_PATH (env var now optional override).
  - D-W1-004 ✅ duplicate import alias gone; D-W1-005 ✅ dead get() deleted.
  - CR-1/CR-2 ✅ round-tripped contract→DDL→store→API→evaluator→FE: rules have
    real `name`; `enabled=false` skips evaluation (tested), distinct from
    muted; FE workarounds removed. CR-3 contract-only per D-006; CR-4 fixed.
  - Full mechanical re-run green: gate script exit 0 (stream 1 061 ms, viewer
    error 0%), all 8 budget tests, Go 7/7 pkgs, web build/lint/21 tests,
    redocly 0/0. Remaining waivers: D-002 (compose) + D-W1-006 (AMS matrix
    tests, carried to wave 2). **Wave 1 gate CLOSED — proceed to Wave 2.**
- DOC-01 reconciled install/alerting runbooks, ARCHITECTURE, README with the
  fixes (PULSE_META_DDL_PATH optional, healthz 503 semantics, enabled-vs-muted
  truth table).
- Housekeeping: `web/pulse_secret.key` (generated dev encryption key)
  gitignored — never commit; ClickHouse artifacts ignore rules landed earlier.
- **D-008 (user directive):** from wave 2 on, every agent commits its own
  scope after its acceptance criteria pass — explicit-path staging only,
  `<AGENT-ID> WO-XXX:` messages, no blanket `git add`, no push; ORCH-00 keeps
  a small wave-close commit. Encoded in decisions.md; wave-2 dispatch prompts
  must carry it.
- Task state: wave 1 DONE; wave-2 WOs written (WO-201..208) — next session
  dispatches them. Resume prompt: `agents/handoffs/RESUME-PROMPT.md`.

### RESUME POINT (next session)

Dispatch Wave 2 as one workflow per WO-201..208 with the D-008 commit
protocol: parallel([BE-01(202) → BE-02(203) → BE-02(204)], SDK-01(201),
FE-01(205), INFRA-01(206)) → QA-01 gate (207) → DOC-01 (208). ClickHouse
binary at `/tmp/clickhouse` (re-download if /tmp cleared). No Docker (D-002).

## 2026-06-14 — Session 4: Wave 2 DISPATCHED

- Pre-flight: `git status` clean (only the RESUME-PROMPT header strip — cosmetic);
  `/tmp/clickhouse` had been wiped, re-downloaded v26.6.1.778 (726 MB) per D-002;
  baseline `server` builds green (`CGO_ENABLED=0 go build ./...` exit 0).
- Re-read all 8 wave-2 work orders, decisions D-001..D-008, ARCHITECTURE refs.
  Work orders NOT rewritten — dispatched as written.
- Wave 2 launched as one Workflow (`pulse-wave-2`, run `wf_e0a0efbd-2ed`,
  script `agents/handoffs/wave-2/wave2.workflow.js`):
  - Phase Implement (parallel barrier): lane A = BE-01(202) → BE-02(203) →
    BE-02(204) sequential (shared go.mod/cmd, D-003); lanes B/C/D = SDK-01(201),
    FE-01(205), INFRA-01(206) concurrent (disjoint trees).
  - Phase Gate: QA-01(207) verifies the integrated tree (barrier justified —
    needs all commits): beacon→dashboard round trip, billing ±1%, budgets.
  - Phase Docs: DOC-01(208) only on PASS / PASS_WITH_LIMITATIONS.
  - Each agent prompt carries: read-order (charter+WO+prereq reports+ARCH §3-4),
    hard rules, env (CH path, CGO=0, no Docker), D-008 self-commit protocol,
    structured-output schema. Reminder added to BE-02(203) to ALSO land the
    CR-3 source-test endpoint (D-006, server impl deferred to wave 2).
- Awaiting workflow completion notification. On return: review gate verdict →
  fix-loop vs proceed → wave-close commit (decisions/DEVLOG/handoffs) → write +
  dispatch Wave 3-MVP (F9 anomaly baselines, F10 single probe runner).

### Wave 2 gate result + ORCH-00 ruling (D-009..D-012)

- **Workflow `pulse-wave-2` (run `wf_e0a0efbd-2ed`) complete** — 8 agents,
  ~2 h, 6/6 implementation COMPLETE + self-committed (D-008), DOC-01 done.
  Commits f327da9 (INFRA), 2d2910f (SDK), 4be5549 (FE), 8c53a7b (BE-01 202),
  f1554ed (BE-02 203), 599a5a3 (BE-02 204), 8eddbe2 (QA gate), 06cc6b4 (DOC).
  Measured: SDK 3.44 KB gzip, billing unit drift 0.0000%, F4 250µs, node 24ms,
  13-mo query 126ms, /metrics bounded, tier gate live-verified.
- **QA-01 gate (WO-207): PASS_WITH_LIMITATIONS** (`qa/wave-2/gate-report.md`).
  12/14 criteria PASS/WAIVED. Waivers: D-002 (no Docker), D-007.5 (no Kafka
  broker). Defects: **D-W2-002 major** (accounting.go wrong CH columns + wrong
  rollup table → live billing 500s; unit test masked it by bypassing CH) —
  the only wave-3 blocker; D-W2-001/003 minor (wave-1 gate script missing the
  now-required alert-rule `name` field).
- **Rulings:** D-009 (focused fix-loop for D-W2-002 + QA script; precedent
  D-006 — no major defect carried forward); D-010 (APPROVE /admin/tenants CRUD
  CR → schedule into validation sweep, not the fix-loop; DEFER global
  white-label endpoint to Phase 3; non-blocking gaps GAP-2-001..005 + INFRA
  206-x tracked); D-011 (D-008 note: SDK-01 commit 2d2910f blanket-staged and
  absorbed FE-01's web/ files — content correct, attribution wrong; reinforce
  explicit-path staging); D-012 (Wave 3-MVP structure: F9→BE-02, F10 split
  BE-01 runner+CH store / BE-02 CRUD+API via ProbeConfigSource seam).
- **Fix-loop dispatched** (`pulse-wave-2-fixloop`, run `wf_02779f15-126`):
  BE-02 fixes D-W2-002 properly (source from rollup_usage_1d per WO-204, fix
  ALL SQL drift, add a live-ClickHouse reconcile integration test that exercises
  the real a.conn path) → QA-01 fixes its gate script + re-gates live. Running
  at this checkpoint.
- **Wave 3-MVP work orders written** (`agents/handoffs/wave-3/WO-301..305`):
  301 BE-01 (probe runner + probe_results CH store + ProbeConfigSource seam),
  302 BE-02 (F9 anomaly baselines/detector/API + F10 probe CRUD/results API +
  source impl), 303 FE-01 (anomalies + probes surfaces, synthetic labeling),
  304 QA-01 (probe round-trip + anomaly false-alarm gate), 305 DOC-01. Ready to
  dispatch as one Workflow once the fix-loop re-gate passes.
- **Process (user directive, this session):** commit orchestration work as it
  lands (don't leave it dangling) and keep `RESUME-PROMPT.md` current as the
  next-session handoff every session. This commit applies that — orchestration
  files only, explicit-path staged (fix-loop's in-flight server/ edits left for
  BE-02 to commit, per D-008/D-011).

## 2026-06-14 — Wave 2 CLOSED + Wave 3-MVP dispatched

- **Wave-2 fix-loop done** (run `wf_02779f15-126`, 2 agents). D-W2-002 CLOSED:
  BE-02 (`77e32c3`) sourced billing from `rollup_usage_1d`, corrected the wrong
  CH columns in BOTH `accounting.go` and `query.go`, and added
  `TestAccountant_CHIntegration` (build tag `integration`) exercising the REAL
  ClickHouse path — ComputeUsage drift 0.0000%, Reconcile drift 0.0000%, tenant
  attribution correct; live `GET /reports/usage` 200, `pulse diag --reconcile`
  0.0000%. QA-01 re-gate (`558377c`): **PASS_WITH_LIMITATIONS, 0 defects** —
  D-W2-001/003 also closed (gate script `name` field), full regression green
  (15 server pkgs, 58/58 web, 56/56 SDK, 8/8 budgets, wave-2 gate exits 0).
  Waivers: D-002 + D-007.5 only. **Wave 2 GATE CLOSED.**
- **Wave 3-MVP dispatched** (`pulse-wave-3-mvp`, run `wf_4320e819-3b5`,
  script `agents/handoffs/wave-3/wave3.workflow.js`): one Workflow per D-012 —
  Implement `parallel([BE-01(301) → BE-02(302)], FE-01(303))` → Gate QA-01(304)
  (probe round-trip + anomaly false-alarm) → Docs DOC-01(305) on pass. Each
  prompt carries read-order, hard rules, env (CH path, CGO=0, no Docker), the
  D-008 commit protocol with explicit-path emphasis (D-011), and the structured
  schema. Running at this checkpoint.
- Next on return: review wave-3 gate → fix-loop vs proceed → wave-close commit →
  **validation sweep** (F1–F10 adversarial vs PRD + deferred D-010 tenant-CRUD
  CR) → consolidation + `IMPLEMENTATION_LOG.md` → notify user, STOP for review.

## 2026-06-14 — Wave 3-MVP CLOSED (D-013)

- **Wave 3-MVP complete** (run `wf_4320e819-3b5`, 5 agents): BE-01 `31e0a13`
  (probe runner + probe_results CH store + ProbeConfigSource seam), BE-02
  `e9e4a99` (F9 Welford anomaly baselines + flags-on-read; F10 probe CRUD +
  results API + MetaProbeConfigSource + serve wiring), FE-01 `d63a28b`/`844abbf`
  (anomalies + probes pages, 4-level synthetic labeling), QA-01 `05e0fd6` gate,
  DOC-01 `2b55235` docs. Verdict PASS_WITH_LIMITATIONS.
- Measured: **F9 false-alarm 0.2594/node-week** (σ=4.0, MinSamples=30,
  HysteresisTicks=10; PRD <1/node-week, 3.8× margin) + 20σ→1 flag true positive;
  **F10 probe round-trip** success ttfb=1ms bitrate=66.7kbps, degraded→http_5xx;
  tier gates (anomalies Enterprise, probes Pro+) live; kin-openapi conformance;
  regression 17 Go pkgs / 109 web / 56 SDK / SDK 3.44 KB.
- **D-013 ruling:** the gate report listed D-W2-001/D-W2-002 as "carried from
  wave-2" — SPURIOUS. ORCH-00 disproved empirically: `accounting.go` untouched
  since `558377c` (only `query.go` changed, columns still correct);
  `TestAccountant_CHIntegration` PASSES (4.2s) on a fresh `/tmp/pulse`;
  `run-gate.sh:380` still has the `name` fix. Root cause: QA-3 tested a STALE
  `/tmp/pulse` + copied the wave-2 defect table. Both wave-2 defects remain
  CLOSED. No fix-loop needed. Correction note appended to the gate report;
  future QA prompts must rebuild binaries + re-verify prior defects.
- Accepted minor scope crossing: BE-02 edited `internal/prober/prober.go`
  (1-line TTFB floor, BE-01's flaky-test fix) — declared, sequential, no revert.
- **All F1–F10 now implemented in MVP form.** Non-blocking gaps (GAP-3-001/003/
  004/006, FE act() warning) → validation sweep / Phase-3 backlog.
- **Next: validation sweep** — adversarial per-feature verification of F1–F10
  against PRD §7 + ARCHITECTURE §4 budgets, folding in the approved D-010
  `/admin/tenants` CRUD CR (INT-01 contract amend → BE-02 routes → FE-01 UI) and
  the carried gaps, defect-fix loop until clean. Then consolidation +
  `IMPLEMENTATION_LOG.md` → notify user, STOP.

## 2026-06-14/15 — Validation phase (mission item 2)

- **V1 — F6 tenant CRUD** (deferred D-010 CR): INT-01 `2323429`, BE-02 `3793b9c`,
  FE-01 `cd5c4d5`, ORCH-00 `38469bf` (fixed DEF-QA-001 test types). Live-verified:
  per-tenant reconcile drift 0.0000%. **D-014 finding:** Business tier missing.
- **V2 — adversarial sweep** (`wf_3bdbf61e-76d`, 14 verifiers, triage `1f090e6`):
  **41 defects, 11 MVP-blocking** the wave gates missed via workarounds. Headlines:
  F3 beacon pipeline broken (SDK header VD-09, main ingest discards VD-10), F2
  geo/device stubs + enrichment unwired (VD-06/07/08), F4 health always 0
  (VD-20/21), F5 muted/group_by dead (VD-28/29), F6 reports ungated + cron broken
  (VD-35/36), tier model (VD-01), security (VD-S1/S2), WS shape (VD-02).
- **V3 fix-loop (D-015):**
  - **V3a INT-01** `0d84d31`: business tier in contract enum + license.go +
    conformance. First V3a run STALLED ~9h on a foreground process (D-016);
    partial non-building edits discarded, INT-01 kept.
  - **V3a-rest** (`wf_4e8b282a-a47`, hardened, ~49 min) — **QA PASS**: BE-01
    `f1d0a7c` (enrichment wiring, health bridge, edge dedup, ingest stats, mmdb),
    BE-02-A1 `5996f2e` (beacon→EventSink, geo/device queries, QoE rollup),
    BE-02-A2 `782c166` (ingest-health API timeseries, tracker, conformance),
    SDK-01 `63f5e81` (header VD-09, rebuffer_end, bitrate), QA `0845ae8`. Data now
    actually flows: beacon round-trip, geo/device analytics, health>0, qoe summary.
  - **V3b IN FLIGHT** (`wf_f21da966-d85`, hardened): [BE-02-B alerting (muted/
    group_by/node_down/cron) → BE-02-C gating(§7.11 matrix)/WS LiveOverview/fleet
    role/security] | FE-01 (tier copy/WS/params) → QA full re-gate → DOC-01.
- Next: V3b gate → (fix-loop if needed) → consolidation + `IMPLEMENTATION_LOG.md`
  → notify user, STOP for review.

## 2026-06-15 — Validation COMPLETE → MVP ready for review

- **V3b done** (`wf_f21da966-d85`, hardened, QA PASS_WITH_LIMITATIONS): BE-02-B
  `cfd6d79` (alerting muted/group_by/node_down/cron), BE-02-C `982f73e` (tier
  gating §7.11 / WS LiveOverview / fleet role / security S1-S3), FE-01 `9a0ba42`
  (tier copy/WS/params), QA `050ba6f`, DOC `568a22b`.
- **D-017:** QA-3b's "remaining defects" table (12 VDs marked OPEN) was SPURIOUS —
  all fixed in V3a (D-013 recurrence: QA echoed the V2 triage without re-verifying).
  ORCH-00 empirically disproved every one (integration tests + SDK 65/65 on HEAD);
  corrected ARCHITECTURE.md (VD-23/X3-A) + the gate report; commit `2c60350`. The
  feature status in IMPLEMENTATION_LOG is built from ORCH-00's own test runs.
- **Consolidation verified on HEAD:** server build/vet clean, `go test ./...`
  0 failures + integration tests pass; web 150/150 + lint + tsc strict; SDK 65/65,
  3.52 KB. Already a single unified project (one binary + web + SDK).
- **`IMPLEMENTATION_LOG.md` written** — per F1–F10: built / issues / resolutions /
  known limitations, all numeric budgets measured, cross-cutting (tier model,
  security), validation summary, Phase-3 backlog.
- **MVP COMPLETE.** All F1–F10 functional end-to-end in MVP form; the 11 V2
  MVP-blocking defects + majors/security/contract fixed and verified; only genuine
  P3 (test-coverage/cosmetic/Phase-3) + D-002/D-007.5 waivers remain. **Notifying
  the user for review; stopping per mission item 6.**

## 2026-06-15 (session 2) — Wave 3-Plus: Phase-3 tech-debt & accuracy closeout

First post-MVP wave. User resumed after the MVP-review STOP and chose the **tech-debt
& accuracy closeout** track (D-018). One Workflow (`pulse-phase3-techdebt`, run
`wf_fba510ab-717`, 7 agents, ~42 min): INT-01 → [BE-01 → BE-02-A → BE-02-B] ∥ FE-01 →
QA-01 → DOC-01. All COMPLETE + self-committed (D-008, explicit paths).

- **INT-01** `19ea611`: 3 pre-approved CRs — OpenAPI `ProbeResult.segment_ttfb_ms` +
  `HealthStatus.components.kafka` (new `KafkaComponentStatus`); CH migrations
  `0002_concurrency_rollup.sql` (`rollup_concurrency_1d` + `mv_concurrency_1d` doing
  `maxState(viewer_count)` from `server_events`, event_type `stream_stats`) and
  `0003_probe_segment_ttfb.sql` (`ALTER TABLE probe_results ADD COLUMN segment_ttfb_ms`).
- **BE-01** `042d2e4`: GAP-3-001 segment TTFB (domain/prober/CH store), GAP-3-003 prober
  follows master→variant for real bitrate, VD-27 `kafka.Source.Lag()` now reads
  `r.Stats().Lag` (atomic-safe), VD-41 discovery sink-emit test.
- **BE-02-A** `a173b61`: GAP-3-001 api serializer, GAP-3-004 anomaly epsilon floor
  (`effStddev = max(stddev, 0.05·|mean|, 1e-9)`), VD-27 `/healthz` kafka component +
  serve.go wiring (D-005 declared cmd edit).
- **BE-02-B** `95ee06d`: VD-38 true windowed `peak_concurrency` from
  `rollup_concurrency_1d` (`maxMerge`), VD-31 real wall-clock alert-latency test (~201 ms),
  VD-19/VD-24 CH-backed API integration tests.
- **FE-01** `86b9994`: segment_ttfb_ms column + TTFB-chart series, VD-26 IngestPage tests
  (web 150→157).
- **QA-01** `454da25`: VD-18 dimensional 13-mo gate (C9b, 145 ms, 3 geo × 2 device), full
  re-gate PASS_WITH_LIMITATIONS (waivers D-002, D-007.5 only).
- **DOC-01** `7aa877a`: ARCHITECTURE §4 budgets + anomaly/probes/reports docs reconciled.

**ORCH-00 independent gate (D-013/D-017 mandate — never trust a QA open/closed list):**
rebuilt + re-ran on HEAD myself — server build/vet clean, `go test ./...` 18 pkgs 0 fail,
integration VD-38 peak=25/5 drift 0.0000% / VD-19 / VD-24 pass, web 157/157. **QA was
ACCURATE this time** — every claimed PASS reproduced (a positive contrast to the
D-013/D-017 spurious lists). Gate CLOSED → D-019.

Flagged (not part of D-018): 3 untracked VPS/Docker test-kit files
(`deploy/docker-compose.override.yml`, `docs/runbooks/test-on-vps.md`,
`qa/vps-smoke-test.sh`) — a separate workstream to execute the D-002-waived compose path
on a real VPS; left untracked for the user to decide.

Remaining Phase-3 backlog: VD-04 (headless render-time) + VD-14 (player CPU) need a real
browser profiler; mobile SDKs, SSO, white-label PDF, air-gapped licensing, hosted beta,
distributed probe network, real multi-node cluster E2E.

## 2026-06-15 — session 4 · W1 `pulse-cicd`: always-on CI/CD that gates `main` (D-020)

Goal (RESUME-PROMPT Workflow 1): every push/PR to `main` is built + linted + tested; add an
e2e smoke + a GHCR release path; make broken changes unmergeable. Found the skeleton CI was
BROKEN vs. the shipped MVP (Go 1.24 not 1.25, `npm ci` w/o `--legacy-peer-deps`, a malformed
`CGO_ENABLED=0 cd server`, soft-fail lint, no docker-build/e2e/release). So W1 = fix + harden
+ extend, not greenfield.

Workflow `pulse-cicd` (`wf_ca6228d5-6cf`, 18 agents): 4 parallel authors (disjoint files) →
adversarial Verify reproducing each job inside the real CI images (`golang:1.25`,
`node:22-alpine`) with a 2-round self-heal fix-loop → independent Gate. ORCH then committed
centrally by explicit path (agents did NOT commit — avoids the parallel-tree index races of
D-008/D-011).

**Verification (D-013/D-017 — never trust the verify phase alone):** 6/7 jobs reproduced
locally; e2e's "refuted" was a harness artifact (my assert curled `/live/overview` with no
token + against an unseeded mock). ORCH re-ran the full e2e chain directly — seed via
mock-ams `/control/publish` → authed `/live/overview` = **viewers=13, publishers=1**; healthz
200; migrate exit 0; 17 CH tables; clean `down -v`. e2e.yml confirmed correct.

Shipped: `.github/workflows/{ci,e2e,release,ams-version-matrix}.yml`,
`deploy/docker-compose.ci.yml`, `.github/branch-protection.sh`, plus a behaviour-preserving
base/override compose refactor (`ports:`→`expose:`, drop `!override`). Gate **CLOSED**
(PASS_WITH_LIMITATIONS). GitHub-side-only (user's to do): push + open a PR so Actions runs
green, run `.github/branch-protection.sh` (needs `gh` + admin — gh not installed on the VPS),
push a `v*` tag for the GHCR release. `e2e` is intentionally not a required check.

Note: the running demo stack (project `pulse`) is currently UNHEALTHY (pulse container up but
not serving on :8090/:80) — flagged for the next session; not in W1 scope.

## 2026-06-15 — session 4 · demo restored: a real AB→BA deadlock fixed (D-021)

The "unhealthy demo" turned out to be a genuine concurrency bug, not a flaky container. A
SIGQUIT goroutine dump showed 489 goroutines wedged on the aggregator RWMutex — 486 HTTP
handlers in `CurrentSnapshot` (RLock), writers `EvictStale`/`OnServerEvent` blocked on `Lock`
for 152 min — an **AB→BA lock-order deadlock** between the live aggregator (`a.mu`) and
`cluster.Discovery` (`d.mu`): each held its own lock while calling into the other's
(`Discovery.poll`→sink→`OnServerEvent` wants `a.mu`; `OnServerEvent`→`onStreamStats`→
`IsEdgeStream` wants `d.mu`). The heavy W1 build load made the two pollers interleave into
the cycle; then every `CurrentSnapshot` reader piled up and the dashboard went dark.

Fix (the rule: never hold a state lock across a sink call): `Discovery.poll` +
`aggregator.EvictStale` collect events under the lock, emit after releasing it. Added two
regression tests that reproduce the deadlock. **Verified both ways** (D-013/D-017): the tests
FAIL (3 s watchdog) on the un-fixed source and PASS race-clean on the fix; full server unit
suite green; image rebuilt + demo redeployed → `/healthz` 200 on :8090 AND :80, `status:ok`.
Demo live again at http://161.97.172.146/.

## 2026-06-15 — session 4 · W2 `pulse-productionize` subset: deploy hardening (D-022)

User picked "subset now, no infra." Workflow `pulse-productionize-subset` (`wf_e82c50f2-c1e`,
4 agents) authored a self-contained hardened overlay: **Caddy TLS** termination, **ClickHouse
auth restored**, pulse off all host ports (reachable only via the proxy), secrets from env —
plus a real-AMS compose overlay and an operator runbook. (First launch crashed on a `${VAR}`
JS-interpolation bug in my own workflow script; escaped and re-ran.)

Adversarially verified against a live `base + hardened` stack (mock AMS, isolated project):
HTTPS 200 over **TLSv1.3** via Caddy's local CA; CH auth **enforced** (authed → 17 tables;
wrong password → Code 516; `default` user removed); pulse-migrate exit 0 on the authenticated
DSN; pulse has **zero host port bindings**. ORCH re-confirmed config parses, `.env.example`
placeholders-only, every referenced env var exists in `config.go`, demo undisturbed. Gate
**CLOSED** (PASS_WITH_LIMITATIONS). Waived to real infra: Let's-Encrypt public TLS + real AMS
connectivity; **`amsclient` real-wire-format fixture hardening deferred** to a future session.

## 2026-06-16 — session 4 (cont.) · production TLS pre-staged for `beyondkaira.com` (D-023)

User acquired the domain `beyondkaira.com`. Gave Squarespace DNS directions (replace the default
parking A records with `@ → 161.97.172.146`) and pre-staged turnkey public TLS so the SSL go-live
is one command once DNS propagates: `deploy/docker-compose.prod-tls.yml` (Caddy on `0.0.0.0:80/443`)
+ `deploy/config/Caddyfile.prod` (real Let's Encrypt, no `tls internal`). Config-verified both
compositions (`base+hardened+prod-tls` and `+real-ams`) — not brought up (real ACME needs the DNS
change + the demo's :80 freed). The exact go-live steps are in `RESUME-PROMPT.md` → W2b.

## 2026-06-16 — session 5 · W2b production TLS go-live for `beyondkaira.com` (D-024)

User finished the Squarespace DNS (apex + `www` → 161.97.172.146, confirmed via 8.8.8.8/1.1.1.1;
the VPS local resolver was stale so on-box checks used `curl --resolve`). Went live: wrote the
gitignored `deploy/.env` (domain + generated CH password + kept secret key), pre-built the prod
images with the demo still up, then swapped demo → **`pulse-prod`** (`base+hardened+prod-tls`,
fresh authed volumes) via an auto-rollback script. **Real Let's Encrypt** cert issued via
TLS-ALPN-01 in ~12 s. Added a `www → apex` canonical redirect (Caddyfile.prod CR); a graceful
reload didn't provision the new name, a `caddy` restart did → valid `www` LE cert + 301.

Adversarially verified via Workflow `pulse-golive-verify` (8 verifiers, **7/8 PASS**): apex+www
public certs (`verify=0`), HTTP→HTTPS, SPA, ClickHouse auth (wrong pw → 516), no host-port leakage
(`127.0.0.1:8090` refused), authed `/api/v1/live/overview` (unauth 401 / authed 200, node up).
Security-headers: all four present + `Server` stripped; only `Via: 1.1 Caddy` remains (Caddy adds
it at the server layer, unremovable via Caddyfile — accepted, informational). `total_viewers=0` is
honest (mock AMS, no streams). **Live: https://beyondkaira.com (+ www).** Closes the public-TLS
waiver; W2c amsclient hardening + real-AMS connectivity remain.

## 2026-06-16 — session 5 (cont.) · CI diagnosis/fix + W2c amsclient hardening (D-025)

Diagnosed CI by reproducing every `ci.yml` job locally in its matching image (repo is private + no
`gh` on the VPS). Only real failure: **helm** — golden files carried trailing blank lines helm
3.17.0 no longer emits; regenerated the 3 goldens (whitespace-only) → `6c7666c`. The local `server`
red was a container-as-root git-ownership VCS-stamp artifact, not a real failure (re-ran with
`safe.directory` → all packages pass `go test -race`); contracts/web/sdk/compose green;
docker-build covered by the prod image built this session.

Then ran Workflow `pulse-amsclient-hardening` (W2c, `wf_4aab2501-0a4`): mapped amsclient+collector,
fixed 3 latent bugs (node version dropped/VD-40; v2.10 speed-only bitrate; empty-StreamID
corruption) + a Kafka dash-viewer parity gap, and added `amsclient`'s **first** tests (11 +
10 JSON fixtures via the real httptest decode path) plus collector variance tests. Verified by the
workflow's race gate + an independent ORCH re-run (`go test ./... -race` green, 19 pkgs, no data
race) + adversarial diff review. Real-capture validation still pairs with a real AMS (W2b real-ams).

## 2026-06-16 — session 5 (cont.) · real-CI-from-logs fixes + security/AMS hardening LIVE (D-026/D-027)

User pasted the actual GitHub Actions logs, exposing 3 failures my local repro had MASKED: compose
(`:?`-required PULSE_SECRET_KEY with no `.env`), web (wrong `git diff` path after `cd web`), and a
~20% flaky query integration test (mv_qoe_1h rollup lag). All fixed + faithfully validated (query
`-count=20` → 0 fail; compose+web reproduced) + pushed (`22dfd4d`, `b1304da`). Lesson: never trust
a partial/inexact CI repro — it yields false green.

Then ran Workflow `pulse-security-ams-hardening` (5 authors + verify): CORS allowlist, token-in-URL
restriction, SSRF guard (scheme+redirect, not private-IP), rate-limiter eviction, beacon caps,
amsclient body limit, http-warn/redact, and WIRED the previously-dead webhook source (fail-closed
HMAC) + CSP/Permissions-Policy. Full `-race` green + adversarial review; 2 reviewer defects fixed.
**Redeployed the hardened binary + CSP to the live site** (CORS + CSP confirmed live; SPA has no
inline scripts so CSP-compatible). Hit + logged a Docker single-file bind-mount inode-staleness
gotcha (force-recreate caddy). Added `.dockerignore` (root-owned CH test artifacts were breaking the
build context). Wrote `agents/handoffs/AMS-INTEGRATION.md` (operator + next-session AMS guide).
Live: **https://beyondkaira.com hardened.**
