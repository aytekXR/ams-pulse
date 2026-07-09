# SESSION-02 — Test backfill A: highest blast radius (Go core)

> Written by SESSION-01 on 2026-07-08 per ROADMAP §6. Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record, §3.S2) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) before dispatching.

## Mission

Exit = ROADMAP S2: kill the biggest Go coverage holes and make the conformance harness honest —
total coverage **≥ 64%**, ci.yml FLOOR **58.0 → 62.0**, all new tests red→green documented,
0 FAIL / 0 unexpected SKIP under `-race` (repo-root mount). No package this session touches ends
below its per-WO target.

## Preconditions (re-verify cheaply — fix this prompt if stale, note drift in decisions.md)

- `git log --oneline -3` shows `1a701d6` (or later handoff commits); tree clean; last ci runs green
  (`gh run list --branch main -L 3`). Tag `v0.1.0` exists; `main` is protected (direct owner pushes
  still work — enforce_admins=false).
- Coverage baseline (verified 2026-07-08 full `-race` run, post-D-058): **total 59.4%**;
  `internal/domain` 0.0, `cmd/pulse` 13.6, `internal/query` 18.5, `internal/api` 55.9,
  `collector/webhook` 58.1 (S3 scope), `reports` 58.8 (S3 scope), `store/clickhouse/migrations`
  **no test files**, `store/clickhouse` 61.8 unit, `meta` 61.9.
- Harness traps (verified 2026-07-08): `openAPISpec()` `t.Skipf` on missing spec
  (`server/internal/api/api_test.go:83-85`); `conformCheck` FindRoute failure only `t.Logf`
  (`api_test.go:~183-188`).
- Flake: `TestDiscovery_NewNodeVisible` budget `testInterval*3` (60ms) at
  `server/internal/cluster/discovery_test.go:~116` — measured 68.8ms once under whole-suite `-race`
  (D-041); loosen like D-039, do NOT blind-bump (D-042: justify the new bound from the poll cycle).
- D-058 leftovers this session inherits: serve.go beacon wiring smoke (License + IngestListenAddr —
  the VD-15 fail-open regression needs a pinning test, see WO-3); operator O7 (GHCR package
  visibility) may still be OPEN — if resolved, verify `docker pull ghcr.io/aytekxr/ams-pulse:0.1.0`
  + `cosign verify` (commands in release.yml header) and record in decisions.md; surface O1–O8 in
  the handoff regardless.
- Dependabot PRs exist (caddy digest bump e2e-green; vite/vitest majors e2e-red) — do NOT merge
  inside this session unless the operator asks; they're O8.

## Work orders (one Workflow: disjoint-scope authors → TDD red→green → adversarial verify → ORCH gate/commit)

### WO-1 — `internal/query` 18.5 → ≥70 · scope `server/internal/query` · [L]
- **Now:** powers every dashboard chart + API read; unit coverage nearly zero — AudienceAnalytics,
  Geo/DeviceBreakdown, QoeSummary, IngestTimeseries, QueryProbeResults, applyRetention, FleetNodes
  all 0% (D-057 scout; integration covers only ~3 of ~12 methods).
- **Change:** table-driven unit tests with a mock Conn (pattern: existing store/clickhouse unit
  tests); cover row-mapping, empty/zero results, error propagation, retention windowing math.
- **TDD:** each method gets its failing test first (mock returns canned rows → assert mapped
  domain values); failure-path tests assert error wrapping, not just nil-checks.
- **Verify:** `go test ./internal/query/... -race -count=1` in golang:1.25, repo-root mount;
  package ≥70% in the gate run.

### WO-2 — `store/clickhouse/migrations` 0 → ≥60 + A11 · scope `server/internal/store/clickhouse/migrations` · [M]
- **Now:** NO test files. Runner logic (splitStatements, stripLeadingComments, substitute) is pure.
- **Change:** unit tests for the pure fns (edge cases: comments-only file, multi-statement,
  `IF NOT EXISTS` idempotency markers); `Run` against the integration harness (`-tags integration`,
  `/tmp/clickhouse`) **including re-run idempotency (A11: migrate twice, second run = no-op, no error)**.
- **Verify:** unit non-tagged + `-tags integration` run both green; A11 asserted explicitly.

### WO-3 — `cmd/pulse` 13.6 → ≥40 (serve wiring smoke incl. D-058 pins) · scope `server/cmd/pulse` · [M]
- **Now:** serve/migrate/diag wiring mostly untested; D-058 added 3 uncovered wiring behaviors that
  MUST get pinning tests: (a) beacon dedicated listener starts when PULSE_INGEST_LISTEN_ADDR set;
  (b) `beacon.Config.License` is non-nil (VD-15 — the fail-open regression found live in D-058);
  (c) version stamping (`versionString` already tested — extend to diag output if cheap).
- **Change:** in-process boot smoke with `:memory:` meta + mock CH/AMS endpoints (D-016: never a
  foreground server without bounded shutdown; use httptest + context timeouts). If a full serve
  boot is too entangled, extract the beacon-construction into a testable helper and pin THAT —
  refactor-for-testability is in-scope, behavior changes are not.
- **Verify:** `go test ./cmd/pulse/... -race`; package ≥40%.

### WO-4 — `internal/api` 55.9 → ≥65 + harness honesty · scope `server/internal/api` · [L]
- **Now:** 15 uncovered handlers (update/delete alert rules+channels, sources, users, license
  activate, bootstrapIfFirstRun, checkPassword, wsPushLoop/wsBroadcast — D-057 scout list); the
  conformance harness silently skips (preconditions above).
- **Change:** handler tests via the existing httptest server pattern (`wave2_test.go` /
  `license_gates_test.go`); **harness honesty first**: `openAPISpec()` `t.Skipf`→`t.Fatalf`,
  `conformCheck` FindRoute `t.Logf`→`t.Errorf` — then fix whatever those flush out (routes missing
  from the spec are CONTRACT work: if a real drift surfaces, stop and file it as an INT-01 CR in
  decisions.md, do NOT silently edit the spec — D-004).
- **Verify:** `go test ./internal/api/... -race` 0 SKIP (repo-root mount!); package ≥65%.

### WO-5 — `internal/domain` 0 → covered + discovery de-flake · scope `server/internal/domain` + `server/internal/cluster/discovery_test.go` · [S]
- **Change:** domain: table test for the Time/typed helpers (trivial, but 0% is a ledger eyesore).
  Discovery flake: widen the latency budget with a justification comment tied to the poll interval
  (e.g. 5× cycle) — the assert must still catch a hung discovery (don't delete it).
- **Verify:** `go test ./internal/domain/... ./internal/cluster/... -race -count=5` (flake check).

## Gates (ORCH, before any commit)

- Full `-race` suite, REPO-ROOT mount, 0 FAIL / **0 unexpected SKIP** (api SKIP count must be 0).
- Coverage: total ≥64.0 → then ratchet ci.yml FLOOR to 62.0 **in the same commit batch** and
  re-run the floor gate command locally (mutation check: set FLOOR=99 → must fail → restore).
- Reproduce every ci.yml step the changes touch (server job at minimum; docker-build if cmd/pulse
  files move; contracts job if WO-4 files a CR).
- No conformance skips: grep the api test output for "skipping conformance" → must be empty.
- No secrets in diffs; commit by explicit path per scope; agents author, ORCH commits (D-008/D-011).

## Closing protocol (ROADMAP §6 — session NOT done without these)

1. Commits per scope (`test(query) D-059: …` etc.); push; `gh run watch` → green.
2. decisions.md D-059 (coverage before/after per package, red→green evidence, any INT-01 CRs).
3. RESUME-PROMPT ▶ START HERE → SESSION-03; ROADMAP §3.S2 ✅ + §4 ledger (new floor) + §5 (O7/O8
   status re-checked).
4. **Write `sessions/SESSION-03.md`** from ROADMAP §3.S3 + actuals (re-verify the 38-operation
   conformance list, webhook/reports coverage numbers, web threshold reality). If cut short,
   SESSION-03.md = resume prompt for the S2 remainder.
