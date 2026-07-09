# SESSION-05 — Honest features + security tail

> Written by SESSION-04 on 2026-07-09 per ROADMAP §6. Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `agents/handoffs/ROADMAP.md`
> (plan of record, §3.S5) + `RESUME-PROMPT.md` §7/§8 (TDD + verification, binding) before dispatching.

## Mission

Exit = ROADMAP S5 (G6): **no silently-approximated metric, security tail closed** — honest
rebuffer_ratio/error_rate alerts off real QoE data (e2e-provable NOW under the mock Business
license, do NOT wait for U3), B7 per-source webhook secret (contract CR via INT-01), logtail
wire-or-delete decision, Caddyfile.prod AMS upstream env var, `.env.example` completeness.
**PLUS the D-061 carry-over: a prod rollout is DUE** — prod still runs `1a701d6` (pre-D-061), so
**rule-triggered alert delivery is broken in prod** until the rollout ships (registry-gap fix,
D-061). Roll out per `deploy/runbooks/real-ams-go-live.md` §4 + smoke (§8) early in the session.

## Preconditions (re-verify cheaply — fix this prompt if stale, note drift in decisions.md)

- `git log --oneline -3` shows the D-061 docs commit (or later); tree clean; ci AND e2e runs
  green on main via `gh run list --branch main -L 5` (e2e now runs on main pushes, D-061).
- Standing numbers post-S4 (D-061, verified 2026-07-09 full `-race`): Go total **73.3%** (floor
  **70.0**); collector 66.5, api 75.9, alert 73.3 (new sync source), everything else per D-060
  table ±0.2. Web gates 76/72/45 + guard test; SDK gates 62/73/70 (webrtc.ts 20.1% still the
  known gap). Conformance 51/52 + 1 waived (G4).
- e2e.yml: triggers push:main + PRs + dispatch. Jobs: `e2e` (A1/A3/A2/A4 + VD-04 steps),
  `csp-e2e` (continue-on-error, bake clock from 2026-07-09). The e2e license mint is
  **business**-tier since D-061 (webhook channels gate; A2 beacon unaffected).
- ci.yml server job now also runs qa/mock-ams + qa/licensegen tests; web-e2e is node 22.
- **Two promotion clocks, BOTH ending ~2026-07-23**: web-e2e's streak restarted 2026-07-09 (its
  first main-push run went red on our spec-pickup bug — testIgnore fix, D-061); csp-e2e green
  since its first run 2026-07-09. If S5 runs after those dates and the streaks
  hold (`gh api repos/aytekXR/ams-pulse/actions/runs/<id>/jobs` per run), promote: drop
  continue-on-error + PUT the full contexts list (existing 7 + the new ones) to
  `repos/aytekXR/ams-pulse/branches/main/protection` — enumerate ALL contexts, a partial list
  silently de-requires the rest.
- **CodeGraph (NEW, operator-installed 2026-07-09 — USE IT):** the repo has a local CodeGraph
  index (`.codegraph/`, gitignored except its marker; CLI at `~/.local/bin/codegraph`).
  **Scouts and authors query the graph BEFORE grepping or fan-out file reads**:
  `codegraph explore "<symbols or question>"` (source + call paths in one shot),
  `codegraph query <search>`, `codegraph node <symbol>` (callers/callees),
  `codegraph callers <symbol>` (blast radius before changing a signature). Put this in every
  agent prompt/work order. The session harness may also inject `codegraph_context` hints on
  prompts — follow them. **Closing protocol addition: run `codegraph sync` (incremental) after
  the last commit so the index matches the final tree**; `codegraph status` to confirm; if sync
  errors on a stale lock, `codegraph unlock`.
- **Binding env** unchanged: Go ONLY in Docker golang:1.25, repo-root mount +
  pulse-gomod/pulse-gobuild volumes, `sg docker -c` prefix; Playwright locally ONLY via
  `mcr.microsoft.com/playwright:v1.61.1-noble` (--network host; scratch copy of web/, never
  mount the repo rw into the root container); compose stacks ONLY from a pristine working-tree
  copy (`git ls-files -co --exclude-standard -z | tar ...`) so deploy/.env never auto-loads.
- Operator items to surface regardless: O1/U3 license, O2/U5 prod CSP human check, O3/O4 AMS
  webhook config, O5 LICENSE, O7 GHCR (still 403 2026-07-09), O8 dependabot (21 PRs),
  **O9 NEW: CodeQL blocked** — private repo without GHAS; once the operator makes the repo
  public (or enables GHAS), add `.github/workflows/codeql.yml`: languages `go` +
  `javascript-typescript`, `on: push: branches [main]`, `pull_request`, `schedule: cron
  '31 3 * * 1'`, default queries, `github/codeql-action/init` + `analyze` per language matrix
  entry; do NOT add to required contexts in its first session.

## Work orders (one Workflow: disjoint-scope authors → TDD red→green → adversarial verify → ORCH gate/commit)

> Reuse the D-059/D-060/D-061 process (parallel authors prove red test-side only; sequential
> adversarial verifiers with exclusive mutation/stack windows; ORCH commits by explicit path).

### WO-1 — prod rollout carrying D-056+D-058+D-061 · scope `deploy/` ops (no code) · [M]
- Build + roll the prod image per the runbook (5-overlay combo incl. backup — §14); smoke:
  `/healthz` ok, stamped version prints the new sha, **create a real webhook-channel alert rule
  against a disposable sink and watch it DELIVER in prod** (the D-061 fix's whole point), then
  delete the test rule/channel. Rollback tag the prior image first.
- **Verify:** stamped version; delivery observed; no new WARN/ERROR in `pulse logs`.

### WO-2 — honest rebuffer_ratio/error_rate alerts · scope `server/internal/alert` + `query` read path · [L]
- Replace the HealthScore proxy (`alert/wave2.go:57-71`) with real reads off `rollup_qoe_1h`/
  `viewer_sessions`. TDD vs a seeded CH (integration harness). E2E-provable under the mock
  Business license TODAY: extend e2e.yml A2 to create a rebuffer_ratio rule and assert a firing
  history row from beacon-injected rebuffer events (D-055 pattern, derived poll budget).
- **Verify:** unit + integration + e2e step red→green; mutation (flip the metric source back to
  proxy → e2e asserts fail).

### WO-3 — B7 per-source webhook secret · scope contract CR (INT-01) + `collector/webhook` · [M]
- Contract CR first (OpenAPI + config schema), then plumb source-keyed secrets
  (config already parses per-source `WebhookSecret`, config.go:283) into the handler, TDD
  (right-secret 200 / wrong-secret 401 per source; back-compat single-secret path).
- **Verify:** regenerated types byte-stable (`npm run gen:api` no-drift), webhook pkg -race.

### WO-4 — logtail wire-or-delete · scope `server/cmd/pulse/serve.go` + `collector/logtail` · [S]
- Decide: wire the commented block (serve.go:200-204) + rotation e2e, or delete with D-0NN
  rationale. Logtail pkg is 92.1% covered — wiring is the cheaper honest option if the AMS log
  path exists on prod; investigate first.
- **Verify:** if wired — e2e/staging log line ingested; if deleted — no dangling config/env vars.

### WO-5 — Caddyfile.prod upstream env var + .env.example completeness · scope `deploy/config` + `deploy/.env.example` · [S]
- `{$AMS_UPSTREAM}` in Caddyfile.prod; add the ~8 missing vars (PULSE_ALLOWED_WS_ORIGINS,
  PULSE_BASE_URL, PULSE_CORS_ALLOWED_ORIGINS, PULSE_AMS_APPLICATIONS,
  PULSE_INGEST_TARGET_BITRATE_KBPS, license pair, metrics token) with comments; drop vestigial
  AMS_LOGIN_*.
- **Verify:** `docker compose config -q` on the full 5-overlay prod combo from a pristine copy
  (dummy env), and caddy validate on the edited Caddyfile.prod.

## Gates (ORCH, before any commit)
- Full `-race` repo-root, 0 FAIL / only the 2 domain npx skips; total must hold ≥73 (floor 70;
  ratchet further ONLY if total ≥74).
- After ANY api/license/query change: the LITERAL integration command with /tmp/clickhouse
  (26.6.1.1193) — unit -race does NOT run the vd19/vd24 integration api tests (D-042).
- Pristine-clone repro of every touched ci.yml/e2e.yml job; web/sdk gates untouched.
- No secrets in diffs; commit per scope; agents author, ORCH commits.

## Closing protocol (ROADMAP §6)
1. Commits per scope; push; `gh run watch` ci AND e2e to green.
1b. `codegraph sync` + `codegraph status` — index must reflect the final tree (D-061 protocol).
2. decisions.md D-062 (per-WO evidence; prod rollout smoke transcript; promotion decisions if
   the clocks lapsed).
3. RESUME-PROMPT ▶ START HERE → SESSION-06; ROADMAP §3.S5 ✅ + §4 ledger + §5 (O7/O8/O9 re-checked).
4. Write `sessions/SESSION-06.md` from ROADMAP §3.S6 + actuals (docs P0s: productionize.md
   5-overlay reality, secrets _FILE; alerting.md prune cap + retry/delivery_failure +
   registry-sync semantics (D-061); upgrade/rollback + monitoring runbooks; SECURITY.md;
   CHANGELOG backfill; Helm parity + golden tests; LICENSE pending O5).
