# SESSION-04 — E2E phase 2 + CI hardening

> Written by SESSION-03 on 2026-07-08 per ROADMAP §6. Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record, §3.S4) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) before dispatching.

## Mission

Exit = ROADMAP S4 (the rest of G5): **CI catches everything it can** — caddy-fronted CSP
Playwright job (closes the CI half of A7), delivery_failure e2e, e2e.yml on main pushes,
web-e2e promotion decision, VD-04/A10 500-stream render measurement recorded in ARCHITECTURE §4,
AMS wire fixture-replay regression suite, CodeQL. Go total stays ≥73 (do NOT regress);
**ratchet ci.yml FLOOR 66.0 → 70.0** if the fresh total holds ≥73.

## Preconditions (re-verify cheaply — fix this prompt if stale, note drift in decisions.md)

- `git log --oneline -3` shows the D-060 docs commit (or later); tree clean; CI run 28975573189
  (or later) green via `gh run list --branch main -L 3`.
- Standing numbers post-S3 (D-060, verified 2026-07-08 full `-race`): Go total **73.2%** (floor
  66); webhook 94.3, reports 90.9, api 75.6 (51/52 ops conformance-validated + GET /live/ws
  waived; error shapes covered). Web gates 76/72/45 + `web/src/test/coverage-gate.test.ts` guard
  (pins gates AND the exact coverage-exclude set — extend the expected set in that test if you
  legitimately add an exclude). SDK gates 62/73/70 (webrtc.ts 20.1% lines is the known gap —
  fair game if S4 wants it, not required).
- e2e.yml currently triggers on PRs only (verify with `grep -A3 '^on:' .github/workflows/e2e.yml`);
  D-055/D-056 deepened it (A1 alert→history, A3 health 100→50, A2 beacon→qoe under the mock Pro
  license). CSP spec is skipped in `web/e2e/` (5 specs; Caddy not fronting `vite preview`).
- web-e2e job: `continue-on-error: true`, node 20. Promotion clock started 2026-07-07 →
  ~2026-07-21 is the 2-weeks-green mark. Check the actual streak
  (`gh run list --workflow ci -L 30 --json conclusion,createdAt` + per-job conclusions) — promote
  to required (drop continue-on-error + add to branch-protection contexts via `gh api`) only if
  the streak holds; else document the failures and keep the clock running.
- Branch protection (D-058): contexts contracts/server/web/sdk/docker-build/helm/compose, strict,
  1 review, enforce_admins=false. Adding required contexts is a `gh api` PUT on
  `repos/aytekXR/ams-pulse/branches/main/protection` — the agent CAN do this (owner-authed gh).
- **Binding env:** Go ONLY in Docker `golang:1.25`, repo-root mount + `pulse-gomod`/`pulse-gobuild`
  cache volumes, `sg docker -c` prefix (D-028/§14). node 20 + npm 10 on host PATH. **Playwright
  ONLY via `mcr.microsoft.com/playwright:v1.61.1-noble`** (host lacks browser libs, no sudo).
  CH pins: service container `clickhouse/clickhouse-server:24.8`, integration binary
  `/tmp/clickhouse` = 26.6.1.1193 (already on this host). ⚠️ mock-ams wire bitrates are divided
  by 1000 in normalize.go:79 — wire 2000000 → health 100, 400000 → 50 (D-055 hard-won).
- Operator items to surface in the handoff regardless: **O7** (GHCR private — still blocked
  2026-07-08(d)), **O8** (21 dependabot PRs; absorb the web-tooling majors ONLY if the operator
  asks — vitest 4 / coverage-v8 4 majors now also touch the NEW web+sdk coverage gates, so do the
  threshold re-baseline in the same batch if absorbed).

## Work orders (one Workflow: disjoint-scope authors → TDD red→green → adversarial verify → ORCH gate/commit)

> Reuse the D-059/D-060 process: parallel authors prove red via test-side wrong-expectation runs
> ONLY; source/config mutations happen in a SEQUENTIAL verify phase (exclusive windows). D-060
> addendum: run timing-sensitive Go repros CONCURRENTLY with other load as a free flake check.

### WO-1 — Caddy-fronted Playwright CSP job · scope `.github/workflows/e2e.yml` + `web/e2e/` + `deploy/` (compose ci overlay only) · [L]
- Bring up the full compose (base + ci overlay + caddy with a CI-only HTTP config — no TLS/ACME
  in CI; the CSP header must be served exactly as prod's Caddyfile sets it) in the e2e job (or a
  new job), run Playwright against the caddy origin inside
  `mcr.microsoft.com/playwright:v1.61.1-noble` (CI) / same image locally.
- Un-skip the CSP spec: assert the `Content-Security-Policy` response header AND zero
  `securitypolicyviolation` console events across the 5 existing specs' pages. Closes A7's CI
  half (U5 human check on prod stays in the operator ledger).
- **Verify:** e2e workflow green on a PR AND the CSP assertions demonstrably bite (mutation:
  drop the CSP header in the CI caddy config → spec fails → restore).

### WO-2 — delivery_failure e2e · scope `.github/workflows/e2e.yml` step + api test if needed · [M]
- E2E-TEST-PLAN phase-2 leftover: create a webhook alert channel pointing at a dead URL
  (unroutable port on localhost), fire an alert (D-055 `ingest_bitrate_floor` pattern), assert an
  `alert_history` row with the delivery_failure state within the retry-exhaustion window (read
  `alert/` retry code (D-049) for the real bound — derive the poll budget, don't guess).
- **Verify:** step red→green proven by pointing the channel at a live sink (delivers → no
  delivery_failure row) vs the dead URL.

### WO-3 — e2e.yml on main pushes + web-e2e promotion + node 22 · scope `.github/workflows/` + branch protection · [S]
- Add `push: branches: [main]` to e2e.yml `on:` (D-056 lesson: e2e only ran on PRs, so a broken
  main overview assert went unseen).
- web-e2e: node 20→22 (align with web job). Promotion decision per the precondition bullet —
  promote (drop continue-on-error, add context to protection via `gh api`) or document the streak.
- **Verify:** `gh run watch` a main-push e2e run to green; protection JSON echoed in the handoff.

### WO-4 — VD-04/A10: 500-stream render measurement · scope `web/e2e/` + `deploy/` (mock-ams ci compose) + `docs/ARCHITECTURE.md` §4 · [M]
- Playwright against the compose stack with mock-ams scaled to 500 streams
  (`/control/set_viewers` + stream count; check mock-ams's real control surface first), measure
  dashboard + streams-table render time (existing 500-row virtualization spec is route-mocked —
  this one goes through the REAL api). Record numbers in ARCHITECTURE §4 against the VD-04
  acceptance criterion; if the criterion fails, that's a FINDING (file it, don't tune the test).
- **Verify:** measurement reproducible (2 runs within reasonable variance), numbers committed.

### WO-5 — AMS wire fixture-replay regression suite · scope `server/internal/collector` (+`pkg/amsclient` fixtures) · [M]
- Table-driven replay of `agents/handoffs/real-ams-captures/` through normalize/aggregate,
  pinning D-029/D-031 semantics: bps→kbps, FPS-redistribution, `terminated_unexpectedly`,
  WebRTC single-track. Any intentional future wire change must break these loudly.
- **Verify:** package -race green; mutation check (flip the /1000 in a COPY of the logic under
  test via the verify-phase window → replay fails → restore).

### WO-6 — CodeQL · scope `.github/workflows/codeql.yml` (new) · [S]
- Go + JS/TS default setup as a workflow file; language matrix; on push/PR + weekly cron.
  Do NOT add it to required contexts this session (let it bake).
- **Verify:** first scan completes on the PR/push (`gh run watch`); zero config errors; triage
  any findings into the handoff (fix trivial ones in-scope, file the rest).

## Gates (ORCH, before any commit)

- Full `-race` suite, REPO-ROOT mount, 0 FAIL / 0 unexpected SKIP (the 2 domain SchemaFixtures
  npx-guard skips are expected in Docker); total ≥73 → ratchet FLOOR 66.0→**70.0** in the same
  batch + awk mutation check (FLOOR=99 → fail → restore).
- Reproduce EVERY ci.yml/e2e.yml job the changes touch on a pristine `git clone` at HEAD
  (D-060 pattern: server incl. migrate smoke vs CH 24.8 `--network container:` + integration w/
  /tmp/clickhouse; web+sdk in node:22; docker-build stamped; e2e via the compose stack).
- Web/SDK coverage gates must still pass untouched (76/72/45, 62/73/70) — WO-1/WO-4 add e2e
  specs, which vitest excludes (`e2e/**`), so unit coverage must not move; if it does, investigate.
- No secrets in diffs; commit by explicit path per scope; agents author, ORCH commits (D-008/D-011).

## Closing protocol (ROADMAP §6 — session NOT done without these)

1. Commits per scope (`e2e D-061: …`, `ci D-061: …` etc.); push; `gh run watch` → green
   (ci AND e2e AND codeql).
2. decisions.md D-061 (per-WO evidence, VD-04 numbers, promotion decision + streak data,
   CodeQL triage, any floor ratchet).
3. RESUME-PROMPT ▶ START HERE → SESSION-05; ROADMAP §3.S4 ✅ + §4 ledger + §5 (O7/O8 re-checked).
4. **Write `sessions/SESSION-05.md`** from ROADMAP §3.S5 + actuals (honest rebuffer/error-rate
   alerts off rollup_qoe_1h — e2e-provable NOW under the mock Pro license, do not wait for U3;
   B7 per-source webhook secret contract CR via INT-01; logtail wire-or-delete decision;
   Caddyfile AMS upstream env var; .env.example completeness). If cut short, SESSION-05.md =
   resume prompt for the S4 remainder.
