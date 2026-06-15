# Resume prompt — Pulse MVP (next session)

> Written by ORCH-00 at the end of the 2026-06-15 session. The MVP build is
> **complete and was handed to the user for review.** Paste this into a fresh
> Claude Code session started in `/Users/ae/repo/ant-marketplace`.

## ✅ Status: MVP COMPLETE — awaiting user review

All features **F1–F10 are functional end-to-end in MVP form** (PRD `prd-report.md`
§7), validated, fixed, consolidated, and documented. Per mission item 6 the build
**STOPPED for user review — no further iteration was to happen before that review.**

**Authoritative artifact:** `IMPLEMENTATION_LOG.md` (per-feature: built / issues /
resolutions / known limitations + a verification table + measured budgets). Read it
first. Full chronology in `DEVLOG.md`; every ruling in
`agents/handoffs/decisions.md` (**D-001…D-017** — all binding); adversarial
validation in `agents/handoffs/validation/`.

**Verified green on `main`** (latest commit `badf5ca`): server `go build/vet` clean +
`go test ./...` 0 failures + integration tests; web 150/150 + lint + tsc strict; SDK
65/65 @ 3.52 KB gzip. Tree clean, nothing pushed.

**Remaining (all documented, MVP-acceptable):** genuine P3 items
(test-coverage / cosmetic / Phase-3) + the **D-002** (no Docker — Compose/Helm
authored & lint-validated, not executed) and **D-007.5** (no Kafka broker) waivers.
See the "Known limitations & Phase-3 backlog" section of `IMPLEMENTATION_LOG.md`.

## What to do next session

1. **If the user has feedback or change requests:** address exactly those points.
   Do NOT silently re-open or re-litigate the closed F1–F10 work.
2. **If the user wants to proceed past MVP:** the Phase-3 backlog is enumerated in
   `IMPLEMENTATION_LOG.md` (mobile SDKs, SSO, white-label PDF, real multi-node
   cluster E2E, distributed probe network, the listed measurement/coverage gaps).
3. **First, always:** `git status` (expect clean), and if you will run code,
   re-download ClickHouse if `/tmp/clickhouse` was wiped (see D-002 below).

## Operating protocol (if work resumes)

- **Orchestrate with the Workflow tool** (the session opted into workflows). One
  phase = one Workflow; ORCH-00 dispatches, QA verifies, ORCH-00 gates.
- **Per-agent commit protocol (D-008, binding):** each agent verifies its acceptance
  THEN commits its own scope by **EXPLICIT path** (never `git add -A`/`-u`/`.` —
  parallel agents share the tree; D-011 incident); message `<AGENT-ID> <id>:
  <summary>` + evidence; no push; on `.git/index.lock` busy, bounded wait+retry,
  never delete. ORCH-00 commits orchestration files (DEVLOG/decisions/handoffs) and
  **keeps `RESUME-PROMPT.md` current every session** (user directive — commit as you
  go, don't leave work dangling).
- **Anti-stall (D-016, learned the hard way — a run hung 9 h):** agents must NEVER
  run a server/ClickHouse in the foreground (`pulse serve`, `clickhouse server`) —
  background + hard `timeout` + kill, or prefer in-process Go tests. Put a `timeout`
  on every build/test command; `npm run test` (vitest run), never watch mode; never
  bare `go test ./...` without `-timeout`. Bake these rules into every agent prompt.
- **QA verification is NOT authoritative on its own (D-013, D-017 — happened TWICE):**
  QA agents twice marked already-fixed defects "open" (stale binary; echoed triage
  without re-checking). Before trusting any "remaining/open defect" list, REBUILD
  binaries and RE-RUN that defect's guard test on HEAD. ORCH-00's own test runs are
  the source of truth.
- **Single-writer scope map** in `agents/manifest.yaml`. BE-01 → BE-02 strictly
  sequential within a phase (shared `go.mod`/`cmd`); SDK/FE/INFRA parallel (disjoint
  trees). Contracts are frozen (D-004) — changes only via ORCH-00-approved CRs
  applied by INT-01.

## Hard rules (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`;
  metrics in ClickHouse, config in the meta store, never crossed; web UI consumes
  ONLY the generated public-API types; beacon ingest is hostile-input territory.
- `CGO_ENABLED=0`; single binary `pulse serve|migrate|diag`; React 19 + RR7 + Vite 6
  + TS strict; recharts; no external fonts/CDNs.
- Tier model is now **4 tiers** per PRD §7.11 (free / pro / **business** / enterprise)
  — `business` is in the contract enum and `internal/license/license.go` (D-014).

## Environment

- macOS arm64; Go 1.26.4, Node v26, npm 11.12.1; **NO Docker** (D-002).
- `/tmp/clickhouse` (v26.6.1) may be wiped between sessions — re-download BEFORE
  running BE/QA code: `cd /tmp && curl -fsSL https://clickhouse.com/ | sh`.
- `web/pulse_secret.key` (dev key) and `*.db*` ClickHouse artifacts are gitignored —
  never commit. Work on `main`.
