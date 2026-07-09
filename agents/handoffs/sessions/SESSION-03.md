# SESSION-03 — Test backfill B: contracts + web

> Written by SESSION-02 on 2026-07-08 per ROADMAP §6. Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record, §3.S3) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) before dispatching.

## Mission

Exit = ROADMAP S3: **G4 met** (every OpenAPI operation response-body-validated, harness stays honest)
+ the web half of G5 groundwork — `collector/webhook` ≥65, `reports` ≥65, web `functions` threshold
gated + every 0%-page smoke-tested, SDK coverage baseline. Go total stays ≥68 (currently 69.7 — do
NOT regress), ci.yml FLOOR **62.0 → 66.0**, 0 FAIL / 0 SKIP under `-race` (repo-root mount).

## Preconditions (re-verify cheaply — fix this prompt if stale, note drift in decisions.md)

- `git log --oneline -3` shows `c80badf` (or later handoff commits); tree clean; ci run
  28922883994 (or later) green via `gh run list --branch main -L 3`.
- Coverage post-S2 (verified 2026-07-08 full `-race`, D-059): total **69.7%** (floor 62);
  `collector/webhook` **58.1** (D-057 scout: parseWebhook 27.3%, jsonInt* 0%); `reports` **58.8**
  (ComputeUsage 4.5%, Reconcile/AggregateByTenant/fetchConcurrencyPeaks 0% — re-verify these
  per-function numbers with a fresh `go tool cover -func` before authoring, they predate S2).
- Conformance harness is HONEST since D-059 (missing spec = t.Fatalf, route-not-in-spec = t.Errorf
  in `server/internal/api/api_test.go`) — do NOT weaken it. 22 `conformCheck` call sites across 4
  api test files (api_test, tenant_test, wave3_test, wo4_handlers_test). ⚠️ The D-057 "14/52
  validated / 38-operation uncovered list" is STALE post-WO-4: **first task = re-count** which of
  the 52 operations in `contracts/openapi/pulse-api.yaml` get a `conformCheck`'d response test, and
  derive the real remaining list.
- Web thresholds (verified 2026-07-08): `web/vite.config.ts:49-52` has lines 57 / branches 71 ONLY —
  `functions` UNGATED (48.3% achieved per D-057; re-measure with `npm run test -- --coverage` or the
  repo's coverage script). 0%-line pages per D-057: App, Layout, SettingsPage, OnboardingWizard,
  AnalyticsPage (+ AlertChannelForm) — re-verify from the fresh coverage report.
- SDK `sdk/beacon-js`: no coverage config at all (D-057). Size gate 15 KB must stay green.
- Operator items to surface in the handoff regardless: **O7** (GHCR private — re-verified blocked
  2026-07-08), **O8** (21 open dependabot PRs). If the operator asks S3 to absorb the web-tooling
  majors (vite 8 / vitest 4 / plugin-react 6 / coverage-v8 4), do it BEFORE the threshold work in
  WO-4/WO-5 (majors change the coverage tooling); otherwise leave every PR untouched.
- Binding env: Go ONLY in Docker `golang:1.25`, repo-root mount + `pulse-gomod`/`pulse-gobuild`
  cache volumes, `sg docker -c` prefix (D-028/§14). node 20 + npm 10 are ON the host PATH — web/sdk
  work runs directly on the host. Playwright only via `mcr.microsoft.com/playwright:v1.61.1-noble`.

## Work orders (one Workflow: disjoint-scope authors → TDD red→green → adversarial verify → ORCH gate/commit)

> S2 process note (D-059, reuse it): parallel authors prove red via test-side wrong-expectation runs
> ONLY; source mutations happen in a SEQUENTIAL verify phase (exclusive windows) so a mutation never
> poisons a concurrent build.

### WO-1 — Conformance completion (G4) · scope `server/internal/api` · [L]
- **First:** re-count validated operations (map every `conformCheck` call to its spec operation;
  52 total operations). Produce the definitive uncovered list, then write response-body tests for
  EVERY remaining operation (D-057 named: /alerts/*, /analytics/*, /qoe/*, /reports/*, /admin/*,
  beacon ingest, healthz — verify against the real spec).
- Error-shape responses count too where the spec declares them (401/403/404 schemas).
- **Contract drift rule (D-004, binding):** contracts/ is FROZEN — if a response genuinely
  mismatches the spec, do NOT edit the spec or soften the test; record the drift precisely; ORCH
  files an INT-01 CR and applies it centrally (regenerated web types must stay byte-stable or be
  regenerated in the same batch: `cd web && npm run gen:api && git diff --exit-code`).
- **Verify:** api suite green, 0 SKIP, 22→52 operations conformance-checked (or the exact list of
  spec-declared-but-unimplementable ones documented, e.g. WS upgrade).

### WO-2 — `collector/webhook` 58.1 → ≥65 · scope `server/internal/collector/webhook` · [M]
- parseWebhook (27.3%) table tests: every AMS lifecycle event shape from real captures, malformed
  JSON, missing fields, wrong HMAC (though sig-check lives in the handler — read the code first);
  jsonInt*/helpers (0%) both branches. Failure paths: decode errors, unknown event types.
- **Verify:** package ≥65% fresh coverprofile; -race -count=1 green.

### WO-3 — `reports` 58.8 → ≥65 · scope `server/internal/reports` · [M]
- ComputeUsage (4.5%), Reconcile, AggregateByTenant, fetchConcurrencyPeaks (0%): mock-Conn pattern
  from D-059's `server/internal/query/query_conn_test.go` (fakeConn/fakeRows w/ reflection scan) —
  adapt locally, don't cross-import test internals. Edge cases: empty tenants, zero usage,
  concurrency-peak windowing, error propagation.
- **Verify:** package ≥65% fresh coverprofile; -race -count=1 green.

### WO-4 — Web coverage gates + 0%-page smokes · scope `web/` (not e2e) · [L]
- Add `functions` threshold to `web/vite.config.ts` (start at achieved−3, minimum 45; ratchet
  lines/branches to achieved−3 as well if headroom exists).
- Smoke tests (vitest + testing-library, route-mocked like the existing 14 test files) for each
  0%-line page: render without crash, key elements present, auth-gate behavior where relevant.
- **Threshold-drop guard:** a test or CI assert that fails if the thresholds block is removed or
  lowered (e.g. grep-assert in ci.yml web job, or a config unit test importing vite.config).
- **Verify:** `npm run build && npm run lint && npm run typecheck && npm test` (vitest run mode,
  never watch — D-016); coverage gates pass; `npm run gen:api && git diff --exit-code`.

### WO-5 — SDK coverage baseline · scope `sdk/beacon-js` · [S]
- Add vitest coverage config + thresholds at achieved−3; document the baseline in the handoff.
  Keep `npm run build && npm run size` green (15 KB gate is binding).
- **Verify:** sdk test+coverage+size all green.

## Gates (ORCH, before any commit)

- Full `-race` suite, REPO-ROOT mount, 0 FAIL / 0 SKIP; total ≥68 (do not regress below 69.7
  without written justification in decisions.md).
- Ratchet ci.yml FLOOR → 66.0 **in the same commit batch**; mutation check (FLOOR=99 → fail →
  restore) + re-run the awk gate with the fresh total.
- Reproduce EVERY ci.yml job the changes touch: server (gofmt/vet/build/test/floor/migrate-smoke
  vs CH 24.8/integration — D-059 pattern incl. `--network container:` trick), web, sdk; contracts
  job if WO-1 files a CR.
- Conformance: fresh api run output greps for '--- SKIP' and 'skipping conformance' → both empty.
- No secrets in diffs; commit by explicit path per scope; agents author, ORCH commits (D-008/D-011).

## Closing protocol (ROADMAP §6 — session NOT done without these)

1. Commits per scope (`test(webhook) D-060: …` etc.); push; `gh run watch` → green.
2. decisions.md D-060 (coverage before/after per package, conformance operation count 22-sites→N/52,
   red→green evidence, any INT-01 CRs).
3. RESUME-PROMPT ▶ START HERE → SESSION-04; ROADMAP §3.S3 ✅ + §4 ledger (new floor, web gates) +
   §5 (O7/O8 re-checked).
4. **Write `sessions/SESSION-04.md`** from ROADMAP §3.S4 + actuals (caddy-fronted Playwright CSP
   job, delivery_failure e2e, e2e.yml on main pushes, web-e2e promotion clock (started 2026-07-07),
   500-stream VD-04 measurement, fixture-replay suite, CodeQL). If cut short, SESSION-04.md =
   resume prompt for the S3 remainder.
