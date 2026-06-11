# Pulse Agent-Based Development Workflow

How this skeleton becomes a shipping product: specialized AI agents (and/or human
developers — the protocol is identical) execute scoped work orders against the
contracts in `/contracts`. This file is the orchestration spec; per-agent charters
live in `definitions/`; the machine-readable registry is `manifest.yaml`.

## 1. Agent roster

| ID | Name | Scope (write access) | Charter |
|---|---|---|---|
| ORCH-00 | Orchestrator | `agents/handoffs/`, task routing | [definitions/ORCH-00-orchestrator.md](definitions/ORCH-00-orchestrator.md) |
| INT-01 | Integration & Contracts Agent | `contracts/` | [definitions/INT-01-integration.md](definitions/INT-01-integration.md) |
| BE-01 | Backend Data-Plane Agent | `server/` (collector, stores, cluster, amsclient) | [definitions/BE-01-backend-dataplane.md](definitions/BE-01-backend-dataplane.md) |
| BE-02 | Backend Product-Plane Agent | `server/` (api, query, alert, reports, license) | [definitions/BE-02-backend-productplane.md](definitions/BE-02-backend-productplane.md) |
| FE-01 | Frontend Agent | `web/` | [definitions/FE-01-frontend.md](definitions/FE-01-frontend.md) |
| SDK-01 | Beacon SDK Agent | `sdk/` | [definitions/SDK-01-beacon-sdk.md](definitions/SDK-01-beacon-sdk.md) |
| INFRA-01 | Infrastructure Agent | `deploy/`, `.github/`, Makefile | [definitions/INFRA-01-infra.md](definitions/INFRA-01-infra.md) |
| QA-01 | QA & Verification Agent | `*/testdata`, test files, `qa/` reports | [definitions/QA-01-qa.md](definitions/QA-01-qa.md) |
| DOC-01 | Documentation Agent | `docs/`, README files | [definitions/DOC-01-docs.md](definitions/DOC-01-docs.md) |

Naming convention: `<DOMAIN>-<NN>` — domain prefix (ORCH/INT/BE/FE/SDK/INFRA/QA/DOC),
two-digit instance number. New agents (e.g. a Phase 3 `SDK-02` mobile-beacon agent)
follow the same scheme and get a charter file before first dispatch.

## 2. Communication model

Agents never talk to each other directly. Three artifacts mediate everything:

1. **Contracts (`/contracts`)** — the only shared interface truth. An agent needing a
   shape that doesn't exist files a contract-change request to INT-01; it does NOT
   invent the shape locally.
2. **Work orders (`agents/handoffs/<phase>/WO-<id>.md`)** — issued by ORCH-00 to one
   agent. Format: objective, PRD references, contract references, files in scope,
   acceptance criteria (testable), out-of-scope list.
3. **Completion reports (`agents/handoffs/<phase>/WO-<id>-report.md`)** — returned by
   the agent: what changed, how acceptance criteria were verified (commands + output),
   new TODOs created, contract-change requests raised.

Rules:
- **Single-writer:** each path has exactly one owning agent (manifest.yaml `scope`).
  Cross-boundary needs become contract requests or work orders — never direct edits.
- **Contracts freeze per phase:** INT-01 finalizes the phase's contract surface
  *before* implementation work orders dispatch; mid-phase contract changes go through
  ORCH-00 and re-issue affected work orders.
- **QA verifies, never fixes:** QA-01 files defect reports routed by ORCH-00 back to
  the owning agent.

## 3. Execution order

Standing rhythm per phase: `INT-01 (contracts) → implementation agents (parallel) →
QA-01 (verify) → DOC-01 (document) → phase gate review`.

### Wave 0 — Foundations (once)
- INFRA-01: make CI real (contract validation, Go build/test, npm builds, size gate);
  pin Docker images. ⛔ Gate: CI green on the skeleton.

### Wave 1 — MVP data plane (PRD Phase 1, weeks 1–10)
- INT-01: finalize `ams-server-event` schema + ClickHouse migration 0001 + meta
  migration 0001 + live/alerts portions of the OpenAPI spec.
- BE-01 ∥ BE-02 ∥ FE-01 (parallel, against frozen contracts):
  - BE-01: amsclient, restpoller + logtail + webhook sources, ClickHouse store, `pulse migrate`.
  - BE-02: config, meta store, api server + auth + WS hub, query (live + basic
    historical), alert evaluator + email/slack channels, license (Free tier).
  - FE-01: app shell, settings/onboarding, live dashboard (F1), basic analytics (F2),
    alerts UI (F5).
- QA-01: AMS version-matrix integration suite; F1/F5 latency budget checks.
- DOC-01: install runbook (the 15-minute guide is a launch asset).
- ⛔ Gate (= PRD Phase 1 exit): compose-up to working dashboard on a live AMS;
  alert latency < 30 s demonstrated; install < 15 min walkthrough verified by QA-01.

### Wave 2 — Revenue-grade v1 (PRD Phase 2, weeks 11–18)
- INT-01: beacon-event schema final; qoe/reports/fleet API surface; rollup migrations.
- SDK-01 ∥ BE-01 ∥ BE-02 ∥ FE-01:
  - SDK-01: beacon SDK (webrtc + hls + transport), size gate, integration docs.
  - BE-01: beacon ingest hardening, kafka source, cluster discovery (F7), rollups.
  - BE-02: full F2 queries, usage/billing reports (F6), Telegram/PagerDuty/webhook
    channels, Data API + Prometheus endpoint (F8), tier enforcement.
  - FE-01: QoE (F3) + ingest health (F4) + reports (F6) + fleet (F7) pages.
- INFRA-01: Helm chart, release pipeline (signed images, versioned artifacts).
- ⛔ Gate (= PRD Phase 2 exit): QoE round-trip beacon→dashboard demoed; billing
  reconciliation within 1%; Mux-comparison demo script passes.

### Wave 3 — Enterprise & expansion (PRD Phase 3, weeks 19–30)
- Mobile beacons (new SDK-02), anomaly detection (F9), synthetic probes (F10), SSO,
  white-label PDF, air-gapped licensing, multi-server portability spike.
- Work orders defined at Wave 2 gate; do not pre-plan in detail (PRD scope discipline).

## 4. Orchestration strategy

- **ORCH-00 is the only dispatcher.** It decomposes phase goals into work orders,
  sequences them by contract dependency, runs implementation agents in parallel within
  a wave, and owns phase-gate go/no-go against the PRD exit criteria.
- **Definition of done is uniform:** code + tests passing in CI + completion report +
  zero contract violations (CI-checked) + docs TODO filed to DOC-01 if user-facing.
- **Escalation:** anything ambiguous in the PRD, any cross-agent dispute, any
  acceptance criterion that proves untestable → ORCH-00 logs a decision in
  `agents/handoffs/decisions.md` (append-only) rather than letting agents guess.
- **Human checkpoints:** phase gates and contract freezes are human-approved; agent
  work inside a wave is autonomous.

## 5. Mapping to Claude Code execution

Each work order = one subagent run (`Agent` tool) with the charter file + work order
as its prompt context. A wave = one `Workflow` invocation: INT-01 first, then
`parallel()` over implementation work orders, then QA-01 verification, with completion
reports written to `agents/handoffs/`. Worktree isolation per agent is unnecessary if
single-writer scopes are respected, but use it when two work orders must touch
`server/` concurrently (BE-01 ∥ BE-02).
