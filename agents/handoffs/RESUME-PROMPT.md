# Resume prompt — Pulse MVP build (paste into a fresh Claude Code session)

> Written by ORCH-00 at the end of the 2026-06-12 session #3 (Wave 1 gate
> CLOSED). Paste everything below the line into a new session started in
> `/Users/ae/repo/ant-marketplace`.

---

Continue the Pulse MVP build as ORCH-00 (orchestrator), using the **Workflow
tool for multi-agent orchestration** — I am explicitly opting in to workflows
for this entire session. One wave = one Workflow invocation.

## Mission (unchanged from the original directive)

Implement the application in `prd-report.md` §7 (Pulse: self-hosted analytics,
QoE monitoring and alerting for Ant Media Server), strictly per PRD:

1. **MVP implementation** — ALL features F1–F10 functional end-to-end in MVP
   form; skip nothing PRD-specified.
2. **Validation** — after implementation, verify every feature against the
   PRD; check inconsistencies, missing behavior, broken flows.
3. **Consolidation** — merge everything into a single unified project.
4. **Documentation** — write `IMPLEMENTATION_LOG.md`: per feature, what was
   done, issues hit, how resolved.
5. **Progress tracking** — keep appending to `DEVLOG.md` as work proceeds.
6. **Final output** — notify me when the MVP is ready for review; no further
   iteration before my review.

## Where the build stands

- **Done:** understand phase; Wave 0 (CI real+green); **Wave 1 CLOSED** —
  contract freeze (32 paths/46 ops/66 schemas, 0 lint issues), data plane,
  product plane, frontend (F1, F2-core, F5-core), QA gate
  PASS_WITH_LIMITATIONS, fix-loop (all 5 defects fixed + CR-1..4), re-gate
  PASS_WITH_LIMITATIONS with fresh measurements (stream 1 061 ms ≤10 s,
  viewer error 0% ≤±2%, alert 15 s <30 s). Gate report:
  `qa/wave-1/gate-report.md` (incl. re-gate section). All committed on `main`.
- **Next: dispatch Wave 2.** Work orders are already written and committed:
  `agents/handoffs/wave-2/WO-201.md` … `WO-208.md`. Do NOT rewrite them —
  read them and dispatch.
- Then Wave 3-MVP (F9+F10 minimal per D-001), validation sweep, consolidation
  + `IMPLEMENTATION_LOG.md`, notify user.

## How to run a wave (established protocol)

Read first: `CLAUDE.md`, `agents/manifest.yaml`,
`agents/handoffs/decisions.md` (D-001..D-008 — all binding), `DEVLOG.md`
(tail), `docs/ARCHITECTURE.md` §3–4.

One Workflow per wave; each agent gets: charter
(`agents/definitions/<ID>.md`) + work order + prerequisite reports +
structured output schema `{status, reportPath, gaps[], cmdEditsDeclared[],
changeRequests[], summary}` (QA: per-criterion verdicts + defects with
owner/severity/repro). Agents write completion reports to
`agents/handoffs/wave-2/WO-XXX-report.md`. QA-01 VERIFIES, NEVER FIXES.
After the gate, ORCH-00 decides fix-loop vs proceed; commit at gate close;
append DEVLOG as you go.

**NEW — D-008 commit protocol (user directive, binding):** every agent
COMMITS its own changes once its work-order acceptance criteria pass:
- verify first (acceptance green), then commit; partial/blocked work is
  reported, not committed;
- stage by EXPLICIT paths inside your scope only — never `git add -A`/`-u`/`.`
  (parallel agents share the working tree);
- message `<AGENT-ID> WO-XXX: <summary>` + verification evidence in the body;
  NO push;
- if `.git/index.lock` is busy: bounded wait+retry, never delete the lock.
Put this block in every agent prompt.

**Wave 2 ordering (D-003/D-005/D-007):**

```
parallel:
  [BE-01 (WO-202) → BE-02 (WO-203) → BE-02 (WO-204)]   # sequential chain, shared go.mod/cmd
  [SDK-01 (WO-201)]
  [FE-01 (WO-205)]
  [INFRA-01 (WO-206)]
→ QA-01 (WO-207 gate: beacon→dashboard round trip; billing reconciles ±1%)
→ DOC-01 (WO-208) — only on PASS / PASS_WITH_LIMITATIONS
```

No INT-01 step (D-007.1 — the D-004 freeze covers wave 2; contract changes go
through ORCH-00 CRs, as exercised in D-006). Beacon ingest
(`server/internal/collector/beacon/`) is BE-02's, exception recorded in
D-007.2. In the workflow script, make BE-02 (WO-203) read
`agents/handoffs/wave-2/WO-202-report.md` first; WO-204 follows WO-203.

**After Wave 2 passes:** write Wave 3-MVP work orders (F9 anomaly detection:
statistical baselines; F10 synthetic probes: single probe runner —
minimal-but-working per D-001; contracts already cover /anomalies and /probes
endpoints + meta tables). Dispatch, gate, commit. Then the validation sweep:
per-feature adversarial verification of F1–F10 against PRD acceptance
criteria (numeric budgets in ARCHITECTURE §4 as test targets), defect-fix
loop until clean. Then consolidation + `IMPLEMENTATION_LOG.md` + notify the
user and STOP for review.

## Binding decisions (full text in `agents/handoffs/decisions.md`)

- **D-001:** all F1–F10 in MVP; F9/F10 minimal-but-working in Wave 3-MVP.
- **D-002:** NO Docker. Compose/Helm authored + lint-validated only. E2E =
  local process stack: `pulse` binary + ClickHouse single binary
  (`/tmp/clickhouse`, v26.6.1 — if /tmp was cleared, re-download:
  `cd /tmp && curl -fsSL https://clickhouse.com/ | sh`) + QA's mock AMS
  (`qa/mock-ams/`).
- **D-003:** BE-01 → BE-02 strictly sequential per wave (shared go.mod);
  SDK/FE/INFRA parallel.
- **D-004:** single contract freeze (done, wave 1) covers waves 1+2+3-MVP;
  changes only via ORCH-00-approved CRs.
- **D-005:** `cmd/pulse` shared sequentially; every cmd/ edit declared.
- **D-006:** wave-1 fix-loop ruling; CR-1/2 (AlertRule name/enabled) DONE;
  CR-3 (source-test endpoint) contract-only — **server implementation lands
  in wave 2 (BE-02; it is NOT in WO-203's numbered list — add it when
  dispatching, it closes the FE onboarding workaround)**.
- **D-007:** wave-2 structure (no INT step; beacon ingest → BE-02; BE-02
  split 203/204; geo = mmdb reader only, no bundled DB; Kafka verified via
  in-process fake — gate waiver class).
- **D-008:** per-agent commits after self-verification (rules above).

## Hard rules (CLAUDE.md / ARCHITECTURE §3 — enforce in every prompt)

- Contracts before code; single-writer scope map in `agents/manifest.yaml`.
- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`.
- Metrics in ClickHouse, config in the meta store, never crossed.
- Web UI consumes ONLY the public API (generated types). Beacon ingest is
  hostile-input territory (token auth, rate limits, size caps, validation).
- Numeric budgets (ARCHITECTURE §4) are test targets: stream ≤10 s, counts
  ±2%, dashboard <2 s @500, 13-month query <3 s, SDK <15 KB gzip, ingest
  degradation ≤15 s, alert <30 s, statements <60 s ±1%, node discovery
  ≤2 min, ~1–2 GB/M sessions.
- CGO_ENABLED=0; single binary `pulse serve|migrate|diag`; React 19 + RR7 +
  Vite 6 + TS strict; recharts; no external fonts/CDNs.

## Environment notes

- macOS arm64; Go 1.26.4, Node v26, npm 11.12.1; NO Docker.
- `/tmp/clickhouse` may be wiped between sessions — re-download BEFORE
  dispatching (BE/QA agents need it), see D-002 above.
- `web/pulse_secret.key` is a generated local dev key, gitignored — never
  commit it; same for ClickHouse artifact dirs (already in .gitignore).
- Carried-forward known items: D-W1-006 (AMS version-matrix Go tests — QA-01,
  wave-2/validation); BE-02 wave-1 gaps G1–G8 are closed by WO-203/204 except
  G6 (Postgres meta path, wave 3) and G7 (WS delta E2E — fold into WO-207
  verification).
- Commit protocol per D-008. Work on `main`.

Start now: verify `git status` is clean, re-download ClickHouse if missing,
read the wave-2 work orders, and dispatch the Wave 2 workflow.
