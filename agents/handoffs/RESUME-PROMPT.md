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

## Where the build stands (updated 2026-06-14, session 4)

- **Done:** understand phase; Wave 0 (CI real+green); **Wave 1 CLOSED**;
  **Wave 2 CLOSED** (impl 8 commits f327da9…06cc6b4 + fix-loop `77e32c3`/
  `558377c`; live billing reconcile drift 0.0000%); **Wave 3-MVP CLOSED**
  (D-013) — F9 anomaly + F10 probes. Wave-3 commits: `31e0a13` BE-01,
  `e9e4a99` BE-02, `d63a28b`+`844abbf` FE-01, `05e0fd6` QA gate, `2b55235` DOC.
  Verdict PASS_WITH_LIMITATIONS (D-002 waiver only). Measured: F9 false-alarm
  0.2594/node-week (<1 target); F10 round-trip ttfb=1ms bitrate=66.7kbps; tier
  gates live; 17 Go pkgs / 109 web / 56 SDK green. **All F1–F10 now in MVP form.**
  NOTE (D-013): the wave-3 gate report's "carried" D-W2-001/D-W2-002 are SPURIOUS
  (QA-3 tested a stale binary) — both remain CLOSED; corrected in-report.

- **Validation underway (mission item 2):**
  - **V1 DONE** — F6 `/admin/tenants` CRUD landed (D-010 CR): INT-01 `2323429`
    contract, BE-02 `3793b9c` routes, FE-01 `cd5c4d5` UI, ORCH-00 `38469bf`
    fixed the one blocker (DEF-QA-001 test types). Live-verified: per-tenant
    reconcile drift 0.0000%, full CRUD + tier gates.
  - **D-014 finding** — the **Business tier is missing** (PRD §7.11 = 4 tiers
    Free/Pro/Business/Enterprise; impl enum = `free|pro|enterprise`), so F5
    PagerDuty/webhook, F6 reports/multi-tenant, F8 API/Prometheus are mis-gated
    to enterprise. CR pre-approved; fix in V3.
  - **V2 IN FLIGHT** (`pulse-val-2-adversarial`, run `wf_3bdbf61e-76d`) — 10
    feature verifiers + 4 cross-cutting critics (architecture, tier model,
    contract conformance, security) → triage. Binaries rebuilt fresh first.
    **FIRST ACTION next session:** read its result + `agents/handoffs/validation/
    V2-triage-report.md`.
  - **NEXT: V3 fix-loop** for the triaged findings (tier model D-014 + whatever
    V2 surfaces), owner-routed (INT/BE/FE/QA), then re-verify until clean.

- **THEN:** consolidation (single unified project) + write `IMPLEMENTATION_LOG.md`
  (per F1–F10: what was done, issues hit, how resolved) → **notify the user and
  STOP for review** (mission item 6 — no further iteration before user review).

- **Then: validation sweep** — per-feature adversarial verification of F1–F10
  against PRD acceptance criteria (numeric budgets in ARCHITECTURE §4), with a
  defect-fix loop until clean. **Fold in the deferred D-010 items here:**
  the approved `/admin/tenants` CRUD CR (INT-01 contract amend → BE-02 routes →
  FE-01 UI) and the non-blocking wave-2 gaps (GAP-2-001..005, INFRA 206-x).

- **Then: consolidation** (single unified project) + write `IMPLEMENTATION_LOG.md`
  (per feature: what was done, issues, resolutions) + **notify the user; STOP
  for review** (no further iteration before user review, per mission item 6).

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
