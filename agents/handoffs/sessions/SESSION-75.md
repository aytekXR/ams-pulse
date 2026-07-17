# SESSION-75 — planned at S74 close (D-136)

> Written by SESSION-74 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S73-AUDIT-FINDINGS.md`** (the 8-finding ledger; 3 shipped).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-76

Re-verify each finding against the code before building — take the verified CORE, not the audit's literal scope. Do NOT
stop after one session — at close, update all docs, regenerate this plan, record progress + operator-needs, continue
until the roadmap is complete or a human/operator is genuinely required. **Ultracode is on** — adversarial-review the
security surface; token cost not a constraint. **Workflow-script gotcha:** no backticks in workflow prompt prose.
`gofmt -l` before every push.

## Goal — [1] HIGH: `query.IngestTimeseries` cross-tenant leak (the last S73 HIGH; self-contained)

`S73-AUDIT-FINDINGS.md`: 5 remain ([1] HIGH + [4]/[5]/[7]/[8] MEDIUM). Lead = [1] (self-contained, HIGH, same class as
the S48/D-110 fix — a proven pattern):

### [1] HIGH — `IngestTimeseries` has no `AND tenant = ?` → cross-tenant ingest-metrics leak
- **loc:** `server/internal/query/query.go:935` (params) + `:975-996` (WHERE) + handler `server/internal/api/server.go:1378`.
- **Re-verify at open** (line refs may have drifted): confirm `IngestTimeseriesParams` still lacks `Tenant`, the WHERE
  still omits the tenant predicate, every SIBLING query (`AudienceAnalytics` :268, `GeoBreakdown`, `DeviceBreakdown`,
  `QoeSummary`) DOES filter tenant, and the handler builds the params without tenant. Confirm how the sibling handlers
  read the tenant (`q.Get("tenant")`) so [1]'s handler matches.
- **Fix:** add `Tenant string` to `IngestTimeseriesParams`; in `IngestTimeseries()` append `if p.Tenant != "" { where +=
  " AND tenant = ?"; args = append(args, p.Tenant) }` (mirror :268 exactly, incl. the empty-tenant behavior — verify the
  siblings' `!= ""` semantics so single-tenant/default deployments are unaffected); pass `q.Get("tenant")` in the handler.
- **Mutation-test:** since query.go hits ClickHouse, prefer a UNIT test of the WHERE/args construction if the code allows
  (e.g. a testable query-builder), OR a fake-DB/string-assertion that the built SQL contains `AND tenant = ?` and args
  includes the tenant when set. Mutation: drop the tenant clause → the test reddens. Check for an existing query-test
  harness first (query has few tests — `server/internal/query/*_test.go`). If ClickHouse-integration-only, follow the
  clickhouse integration-test pattern (may SKIP without a live CH — note that honestly).
- **Adversarial-review** (tenant-isolation security surface → mandatory).

## Later clusters (remaining S73)
- **[4] MEDIUM** `PruneAlertHistory` Postgres COUNT+DELETE race → single-statement `DELETE ... WHERE id NOT IN (SELECT
  ... ORDER BY ts DESC LIMIT keep)` (store/meta). Standalone.
- **[5] MEDIUM** `QoEForStream` cross-tenant QoE — ⚠ WIDER than the finder claimed (no Tenant field in AlertScope/
  AlertRuleRow/LiveStream; thread tenant through the live pipeline first). Scope carefully; may split.
- **[7] MEDIUM** admin token in WS URL → a short-lived `POST /auth/ws-ticket` (server + web; avoid the do-not-commit
  Caddyfile). Bigger; server+web+web-CI (schema.d.ts regen if OpenAPI changes).
- **[8] MEDIUM** web SettingsPage silent error handlers → try/catch+toast (web-only CI: vitest/component test).

## Pipeline (the S62→S74 loop)
1. **Verify-at-open:** `git log --oneline -3`, HEAD == origin/main, `git status` shows ONLY Caddyfile.prod dirty. Record
   **D-137 IN PROGRESS** in `decisions.md`. Branch `s75-d137`. **CHECK THE DATE** (§2.7 gate ≥ 2026-07-23).
2. **Re-verify vs code** (`mcp__codegraph__codegraph_explore`); take CORE.
3. **Fix → mutation-prove** on `/tmp/pulsemut` (NOT `/mut`). `go test -vet=off` for unreachable mutants.
4. **Full Go suite** (25 pkgs) in docker; **`gofmt -l`** on changed files. If `contracts/openapi/pulse-api.yaml` changes
   (adding a `tenant` param to /qoe/ingest?), regen `web/src/lib/api/schema.d.ts` via `cd web && npm run gen:api` +
   redocly lint — the `web` CI drift check fails otherwise (S68/S69 gotcha).
5. **Adversarial review** (tenant-isolation surface): finder lenses → refute-by-default verifiers.
6. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** (server source changed): `config -q` → tag `pulse-prod-pulse:pre-d137` → backup rc0 → STAMPED
   build backgrounded → assert stamp ≠ dev → `up -d pulse` no --build → 5-check smoke.
8. **Close docs:** `decisions.md` D-137 SHIPPED, CHANGELOG, `S73-AUDIT-FINDINGS.md` [1] ✅ DONE, ROADMAP §2.32 count,
   RESUME rotation, `operator-expected.md`, SESSION-75 CLOSED, SESSION-76 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker** (25 pkgs); mutation copy `/tmp/pulsemut` (not `/mut`). `gofmt`/`go` NOT on host PATH. Node at
  `/home/aytek/.local/bin/node` (npx) for web/redocly. **CodeQL** flags `InsecureSkipVerify` in PRODUCTION Go only.
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod at **v0.4.0-85-g28b8dfc**. 5-check smoke: startup
  version stamp, healthz 200, signed webhook 200 (HMAC from PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
