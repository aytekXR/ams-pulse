# Resume prompt — Pulse MVP build (paste into a fresh Claude Code session)

> Written by ORCH-00 at the end of the 2026-06-12 session (Wave 0 gate PASS).
> Paste everything below the line into a new session started in
> `/Users/ae/repo/ant-marketplace`.

---

Continue the Pulse MVP build as ORCH-00 (orchestrator), using the **Workflow tool
for multi-agent orchestration** — I am explicitly opting in to workflows for this
entire session. One wave = one Workflow invocation.

## Mission (unchanged from the original directive)

Implement the application in `prd-report.md` §7 (Pulse: self-hosted analytics, QoE
monitoring and alerting for Ant Media Server), strictly per PRD:

1. **MVP implementation** — ALL features F1–F10 functional end-to-end in MVP form;
   skip nothing PRD-specified.
2. **Validation** — after implementation, verify every feature against the PRD;
   check inconsistencies, missing behavior, broken flows.
3. **Consolidation** — merge everything into a single unified project.
4. **Documentation** — write `IMPLEMENTATION_LOG.md`: per feature, what was done,
   issues hit, how resolved.
5. **Progress tracking** — keep appending to `DEVLOG.md` as work proceeds.
6. **Final output** — notify me when the MVP is ready for review; no further
   iteration before my review.

## Where the build stands

- **Done:** understand phase; Wave 0 (build/test/lint/size-gate real and green) —
  gate PASS, see `agents/handoffs/wave-0/gate-report.md`. All committed on `main`
  (latest commit at hand-off: wave-0 results + gate report).
- **Next: dispatch Wave 1.** Work orders are already written and committed:
  `agents/handoffs/wave-1/WO-101.md` … `WO-106.md`. Do NOT rewrite them — read
  them and dispatch.
- Then Wave 2, Wave 3-MVP (F9+F10 minimal), validation sweep, consolidation.

## How to run a wave (established protocol)

Read first: `CLAUDE.md`, `agents/README.md`, `agents/manifest.yaml`,
`agents/handoffs/decisions.md` (D-001..D-005), `DEVLOG.md` (tail),
`docs/ARCHITECTURE.md` §3–4.

For each wave, invoke ONE Workflow whose agents each receive: their charter file
(`agents/definitions/<ID>.md`) + their work order + pointers to frozen contracts
and any prerequisite completion reports. Agents write completion reports to
`agents/handoffs/wave-N/WO-XXX-report.md`. End every wave with a QA-01
verification agent (VERIFY, NEVER FIX — defects go in the gate report), then
ORCH-00 decides: fix-loop within the wave (dispatch targeted fix agents, re-gate)
or proceed. Commit after each wave gate. Append DEVLOG entries as you go.

**Wave 1 ordering (binding decisions D-003/D-005):**

```
INT-01 (WO-101, contract freeze — runs ALONE first)
  → then in parallel:
      [BE-01 (WO-102) → BE-02 (WO-103) sequential — shared go.mod + cmd/pulse]
      [FE-01 (WO-104)]
  → QA-01 (WO-105 gate)
  → DOC-01 (WO-106)
```

In the workflow script: `agent(WO-101)` first and alone; then `parallel([backend
chain as one sequential async fn, FE-01])`; then QA-01; then DOC-01. BE-02 must
read `WO-102-report.md` before starting (its prereq). Give each agent structured
output (schema) with at least `{status, reportPath, gaps[], cmdEditsDeclared?}`.

**Wave 1 gate (manifest, adapted per D-002):** local process stack up → live
dashboard shows mock-AMS streams ≤10 s; alert <30 s; rules survive restart;
install path <15 min; counts ±2%.

**After Wave 1 passes:** write Wave 2 work orders (same style as wave-1's; scope
from `agents/manifest.yaml` wave 2: F3 beacon SDK+ingest, F4 ingest health,
F2-full geo/device, F6 reports, F7 fleet, F8 public API+Prometheus, extra alert
channels Telegram/PagerDuty/webhook, Helm chart) and dispatch the same way.
Then Wave 3-MVP: F9 anomaly detection + F10 synthetic probes, minimal-but-working
(D-001). Then the validation sweep: per-feature adversarial verification against
PRD acceptance criteria (use the numeric budgets in ARCHITECTURE §4 as test
targets), defect-fix loop until clean. Then consolidation +
`IMPLEMENTATION_LOG.md` + notify the user.

## Binding decisions (full text in `agents/handoffs/decisions.md`)

- **D-001:** all F1–F10 in MVP; F9/F10 minimal-but-working in Wave 3-MVP.
- **D-002:** NO Docker on this machine. Compose/Helm artifacts are authored +
  lint-validated only. E2E verification = local process stack: `pulse` binary +
  ClickHouse single binary + QA-01's mock AMS (`qa/mock-ams/`, built in WO-105).
  ClickHouse v26.6.1 binary was at `/tmp/clickhouse` — if /tmp was cleared,
  re-download (`curl https://clickhouse.com/ | sh`) before the QA gate.
- **D-003:** BE-01 and BE-02 run SEQUENTIALLY within a wave (shared `server/go.mod`;
  BE-01 first — it owns `internal/domain`). FE-01/SDK-01/INFRA-01 may run parallel.
- **D-004:** single full contract freeze in Wave 1 (WO-101) covering waves 1+2+3-MVP.
  Later waves file change-requests instead of editing contracts.
- **D-005:** `server/cmd/pulse` is sequentially shared: BE-01 leaves assembly hooks,
  BE-02 may extend them, every cmd/ edit declared in completion reports.

## Hard rules (from CLAUDE.md / ARCHITECTURE §3 — enforce in every work order)

- Contracts before code; single-writer scope map in `agents/manifest.yaml`.
- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`.
- Metrics in ClickHouse, config in the meta store, never crossed.
- Web UI consumes ONLY the public API (generated types from OpenAPI).
- Numeric budgets (ARCHITECTURE §4) are test targets: stream visible ≤10 s,
  alert <30 s, counts ±2%, dashboard <2 s @500 streams, 13-month query <3 s,
  SDK <15 KB gzip, statements <60 s ±1%, node status ≤2 min.
- CGO_ENABLED=0 (SQLite via modernc.org/sqlite); single Go binary
  `pulse serve|migrate|diag`; React 19 + RR7 + Vite 6 + TS strict; recharts;
  no external fonts/CDNs.

## Environment notes

- macOS arm64; Go 1.26.4, Node v26, npm 11.12.1 installed; NO Docker.
- Known carried-forward gaps (non-blocking, owners assigned): missing
  eslint.config.js in web/ (FE-01, WO-104) and sdk/ (SDK-01, wave 2); zero test
  files in web//sdk/ (land with feature work); 55 redocly warnings (resolved by
  WO-101 freeze).
- Commit at each wave gate with descriptive messages. Work on `main`.

Start now: verify `git status` is clean, read the wave-1 work orders, and
dispatch the Wave 1 workflow.
