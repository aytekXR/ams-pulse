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
  test junk removed. Checkpoint commit before fix-loop dispatch.
