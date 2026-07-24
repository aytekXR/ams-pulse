# Pulse — guidance for AI agents and developers

Pulse: self-hosted analytics, QoE monitoring and alerting for Ant Media Server.
PRD: `docs/prd-report.md` §7 (the rest of that file is market analysis — context, not spec).

## Current state

**Shipped product, pre-marketplace.** All 10 PRD features are implemented and live-validated
against a real AMS 3.0.3 Enterprise (46/50 scenarios); latest release v0.4.0; production runs
behind host nginx on this VPS. The wave plan in `agents/manifest.yaml` is complete — current
work follows `agents/handoffs/ROADMAP-V2.md` and the session protocol in
`agents/handoffs/RESUME-PROMPT.md` (start there, not at the wave plan). Remaining open items
are mostly operator-gated marketplace-submission steps (`docs/operator-expected.md`).

## How to work in this repo

1. **Read your charter first.** Work is partitioned by agent scopes —
   `agents/README.md` (orchestration), `agents/definitions/<ID>.md` (per-agent rules).
   Respect the single-writer scope map in `agents/manifest.yaml` even when working
   solo.
2. **Contracts before code.** Interface shapes live in `contracts/` (OpenAPI, event
   schemas, migrations). Change the contract first, then the implementations.
3. **Architecture rules** are in `docs/ARCHITECTURE.md` §3 — especially: AMS wire
   formats only in `server/pkg/amsclient` + `server/internal/collector`; metrics in
   ClickHouse, config in the meta store, never crossed; the web UI uses only the
   public API.
4. **Numeric acceptance criteria** from the PRD are collected in
   `docs/ARCHITECTURE.md` §4 — treat them as test targets, not aspirations.
5. `TODO(<AGENT-ID>)` markers throughout the skeleton say who implements what, in
   which phase.
6. **Brand/design source of truth is `brandkit/`** (D-071) —
   `brandkit/design-system/tokens.json` is authoritative for all web-UI colors/type/spacing
   (don't invent values); hi-fi screens in `brandkit/ui/`; the WCAG table in
   `brandkit/documentation/design-rationale.md` §2 is binding. Fonts (IBM Plex, OFL) are
   self-hosted only — never a CDN. Adoption plan: `agents/handoffs/ROADMAP-V2.md` §2.15.

## Commands

- `make help` — all targets; `make build|test|lint` delegate per component
- Server: `cd server && go build ./... && go test ./...`
- Web: `cd web && npm install && npm run dev` (proxies to `pulse serve` on :8090)
- SDK: `cd sdk/beacon-js && npm install && npm run build && npm run size` (15 KB gate)
- Stack: `make up` / `make down` (Docker Compose in `deploy/`)

## Starting a build session

ORCH-00 role: pick the next wave from `agents/manifest.yaml`, write work orders to
`agents/handoffs/wave-N/`, dispatch subagents with (charter + work order) as context,
collect completion reports, verify with QA-01, gate before the next wave.
