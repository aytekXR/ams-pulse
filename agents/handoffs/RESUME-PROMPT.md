# Resume prompt — Pulse (next session)

> Written by ORCH-00 at the end of the 2026-06-15 **session 2**. The MVP plus the first
> post-MVP wave (**Wave 3-Plus: Phase-3 tech-debt & accuracy closeout**) are complete,
> independently verified, and committed to `main`. Paste this into a fresh Claude Code
> session started in `/Users/ae/repo/ant-marketplace`.

## ✅ Status: Wave 3-Plus COMPLETE — awaiting user direction

The MVP (F1–F10) was complete and reviewed; this session closed the deferred,
environment-feasible **Phase-3 tech-debt & accuracy** gaps the user selected. All 10 items
landed and were **independently re-verified on HEAD by ORCH-00** (not just QA — per the
D-013/D-017 lesson). Gate **CLOSED** (decision **D-019**, PASS_WITH_LIMITATIONS; only the
D-002 no-Docker and D-007.5 no-Kafka-broker waivers remain).

**What shipped this wave** (Workflow `pulse-phase3-techdebt`, run `wf_fba510ab-717`,
commits `19ea611`→`7aa877a` + the ORCH-00 D-019 orchestration commit):

- **VD-38** — true windowed `peak_concurrency`: new `rollup_concurrency_1d`
  (AggregatingMergeTree) takes `maxState(viewer_count)` from `server_events`; billing reads
  `maxMerge`. Verified peak=25/5 from overlapping snapshots (was a session-count proxy).
- **GAP-3-001** — probe first-segment TTFB (`segment_ttfb_ms`) end-to-end (domain→CH→API→UI).
- **GAP-3-003** — HLS probe now follows a master playlist's first variant → real bitrate.
- **GAP-3-004** — anomaly epsilon floor (`max(stddev, 0.05·|mean|, 1e-9)`): deviations from a
  *constant* baseline now flag; false-alarm bound unchanged (0.2594/node-week).
- **VD-27** — Kafka consumer lag + parse-errors now in `/healthz` (and `Lag()` actually reads
  `r.Stats().Lag`, atomic-safe).
- **VD-18/19/24/26/31/41** — dimensional 13-mo query gate; geo/device + qoe/ingest API
  integration tests; IngestPage UI test; **real wall-clock** alert-latency test; discovery
  sink-emit test.

**Verified green on HEAD:** server `go build/vet` clean + `go test ./...` **18 pkgs, 0 fail**
+ CH integration (VD-38/19/24) pass; web **157/157** + lint + tsc strict; SDK unchanged
(65, 3.52 KB). New migrations: `0002_concurrency_rollup.sql`, `0003_probe_segment_ttfb.sql`.

**Authoritative artifacts:** `IMPLEMENTATION_LOG.md` (per-feature, updated), `DEVLOG.md`
(chronology), `agents/handoffs/decisions.md` (**D-001…D-019** binding; D-018 = the wave plan,
D-019 = the gate close). QA gate: `qa/wave-3-plus/gate-report.md`.

## ⚠️ One thing to decide: untracked VPS/Docker test-kit

Three **untracked** files are sitting in the tree, NOT created by this wave's agents and NOT
committed: `deploy/docker-compose.override.yml`, `docs/runbooks/test-on-vps.md`,
`qa/vps-smoke-test.sh`. They are a coherent kit to bring the full stack up against the mock
AMS **on a real VPS** — i.e. to finally execute the **D-002-waived** Docker Compose path that
this macOS machine can't run. Decide whether to adopt them as a separate "close the D-002
waiver" workstream (then commit via INFRA-01/QA-01 scope) or discard. ORCH-00 left them
untouched per the "don't commit what you didn't author / surface it" rule.

## What to do next session

1. **If the user has feedback / change requests on Wave 3-Plus:** address exactly those.
   Do NOT silently re-open the closed items.
2. **If the user wants to keep going on Phase 3**, the remaining tracks (ask which):
   - **Enterprise feature wave** — SSO (OIDC), white-label (global brand config + branded
     PDF), air-gapped licensing (signed offline license). All buildable + verifiable here;
     map to the PRD's "2 Enterprise logos" exit criterion.
   - **Non-AMS portability spike** — protocol-level beacon against a Wowza/Red5/Flussonic-
     style HLS source (PRD "3× addressable base"; "one non-AMS pilot").
   - **Close the D-002 waiver on a real VPS** — use the untracked kit above (real Docker).
   - **Mobile beacon SDKs** (Android/iOS/Flutter) — biggest item, but native toolchains are
     likely absent here (would be authored + unit-tested, execution waived).
   - **Remaining tech-debt** — VD-04 headless render-time + VD-14 player-CPU need a real
     browser profiler (Playwright/CDP); long-run anomaly false-alarm simulation.
3. **First, always:** `git status` (expect clean apart from the 3 untracked VPS files), and
   if you will run code, re-download ClickHouse if `/tmp/clickhouse` was wiped (see Environment).

## Operating protocol (binding — learned the hard way)

- **Orchestrate with the Workflow tool.** One wave = one Workflow; ORCH-00 writes the plan +
  pre-approved CRs to `decisions.md`, dispatches `INT-01 → [BE-01→BE-02-…] ∥ FE-01 → QA-01 →
  DOC-01`, then **independently gates**.
- **Per-agent commits (D-008):** each agent verifies acceptance THEN commits its own scope by
  **EXPLICIT path** (never `git add -A`/`-u`/`.` — parallel agents share the tree; D-011);
  message `<AGENT-ID> <id>: <summary>` + evidence; no push; on `.git/index.lock` busy,
  bounded wait+retry, never delete. ORCH-00 owns `DEVLOG`/`decisions`/`IMPLEMENTATION_LOG`/
  `RESUME-PROMPT` and **keeps RESUME-PROMPT current every session** (user directive).
- **Anti-stall (D-016 — a run once hung 9 h):** NEVER run a server/ClickHouse in the
  foreground (`pulse serve`, `clickhouse server`). CH only via the Go integration harness
  (`go test -tags integration`). `timeout` on every build/test; `-timeout` on every `go test`;
  `npm run test` (vitest run), never watch. If a command hangs, kill it.
- **QA is NOT authoritative alone (D-013, D-017, re-confirmed D-019):** before trusting any
  "open/closed" claim, REBUILD binaries and RE-RUN that guard test on HEAD. ORCH-00's own test
  runs are the source of truth. (This wave QA was *accurate* — but it was still re-verified.)
- **Single-writer scope map** in `agents/manifest.yaml`. BE-01 → BE-02 strictly sequential
  within a phase (shared `go.mod`/`cmd`); `prober/`→BE-01, `anomaly/`→BE-02 (D-012); SDK/FE/
  INFRA parallel (disjoint trees). Contracts frozen (D-004) — changes only via ORCH-00-approved
  CRs applied by INT-01.

## Hard rules (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`; metrics in
  ClickHouse, config in the meta store, never crossed; web UI consumes ONLY generated
  public-API types; beacon ingest is hostile-input territory.
- `CGO_ENABLED=0`; single binary `pulse serve|migrate|diag`; React 19 + RR7 + Vite 6 + TS
  strict; recharts; no external fonts/CDNs.
- **4 tiers** per PRD §7.11 (free / pro / **business** / enterprise) — `business` in the
  contract enum and `internal/license/license.go` (D-014).

## Environment

- macOS arm64; Go 1.26.4, Node v26, npm 11.12.1; **NO Docker** (D-002).
- `/tmp/clickhouse` (v26.6.1) may be wiped between sessions — re-download BEFORE running BE/QA
  code: `cd /tmp && curl -fsSL https://clickhouse.com/ | sh`.
- `web/pulse_secret.key` (dev key) and `*.db*` ClickHouse artifacts are gitignored — never
  commit. Work on `main`.
