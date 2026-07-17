# SESSION-76 — planned at S75 close (D-137)

> ## ✅ CLOSED (2026-07-17, D-138) — SHIPPED [4]
> `PruneAlertHistory` now uses a single self-contained `DELETE ... WHERE rule_id=? AND id NOT IN (SELECT id ... ORDER BY
> ts DESC LIMIT keep)` (rowid on SQLite) instead of the racy COUNT-then-DELETE — concurrent Postgres prunes can no longer
> over-delete alert history (PR #145, prod `v0.4.0-89-g300251d`). Regression guard = the existing prune suite
> (mutation-proven: ORDER BY flip + dropped rule_id both redden it); PG branch validated in CI; 2-lens review clean (0
> findings). **No operator action.** See `decisions.md` D-138 and `sessions/SESSION-77.md` (lead: [8] web SettingsPage
> silent error handlers). Everything below is the original pre-session plan.


> Written by SESSION-75 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S73-AUDIT-FINDINGS.md`** (8-finding ledger; 4 shipped, ALL HIGH done).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-77

Re-verify each finding against the code before building — take the verified CORE. Do NOT stop after one session — at
close, update all docs, regenerate this plan, record progress + operator-needs, continue until the roadmap is complete
or a human/operator is genuinely required. **Ultracode is on** — adversarial-review the security/state-machine surface.
**Workflow-script gotcha:** no backticks in workflow prompt prose. `gofmt -l` before every push.

## Goal — [4] MEDIUM: `PruneAlertHistory` non-transactional COUNT+DELETE race (store/meta, standalone)

`S73-AUDIT-FINDINGS.md`: 4 MEDIUM remain ([4]/[5]/[7]/[8]). Lead = [4] (self-contained, backend, cleanest):

### [4] MEDIUM — `PruneAlertHistory` COUNT+DELETE race over-deletes on Postgres
- **loc:** `server/internal/store/meta/meta.go:1047` (COUNT) + `:1077` (DELETE). Called unsynchronised after every
  `CreateAlertHistory` INSERT. Two concurrent workers compute a stale Go `excess` and their two DELETEs together drop
  below the `keep` cap. Postgres-only (SQLite MaxOpenConns=1 serialises); permanent alert-history loss, bounded per race.
- **Re-verify at open:** confirm the two-statement COUNT+DELETE is still there and unsynchronised; check whether a
  simpler single-statement DELETE is expressible on BOTH backends (SQLite + Postgres) — `DELETE FROM alert_history WHERE
  rule_id=? AND id NOT IN (SELECT id FROM alert_history WHERE rule_id=? ORDER BY ts DESC, id DESC LIMIT ?)`. Verify the
  column names (`ts`? `created_at`?), the ORDER BY tiebreaker (id), and that `NOT IN (subquery with LIMIT)` is valid on
  SQLite (it is) and Postgres (it is). Watch the rebind (`?` vs `$1`) — the store has a rebind helper; use it.
- **Fix:** replace the COUNT+DELETE pair with the single self-contained DELETE (no intermediate snapshot gap). Pass
  `keep` as the LIMIT bound. Keep the existing cap semantics (`AlertHistoryDefaultKeep` / `alertHistoryCap`).
- **Mutation-test:** insert `keep+N` rows for a rule, prune, assert exactly `keep` remain AND the newest `keep` are kept
  (delete the oldest). Mutation: an off-by-one in the LIMIT or wrong ORDER BY direction → the count/identity assertion
  reddens. (The race itself is hard to unit-test deterministically; the single-statement correctness + keep-newest is
  the testable core. Note the concurrency-safety honestly in the test comment.) Run against BOTH backends if the PG
  integration harness is available (`meta_pg_integration_test.go`, `-run TestPG_...`; may SKIP without postgres:16).
- Self-review may suffice (mechanical SQL change) — but if the DELETE semantics differ between backends, adversarial-review it.

## Later clusters (remaining S73)
- **[5] MEDIUM** `QoEForStream` cross-tenant QoE — ⚠ WIDER (thread Tenant through the aggregator → `AlertScope`/
  `LiveStream`, then the `QoEReader` signature). Bigger; scope carefully; may split into a "tenant-in-live-pipeline" prereq.
- **[7] MEDIUM** admin token in WS URL → `POST /auth/ws-ticket` (short-lived single-use). server + web + OpenAPI (+
  schema.d.ts regen). Avoid the do-not-commit Caddyfile.
- **[8] MEDIUM** web SettingsPage silent error handlers → try/catch+toast (web-only; vitest/component test — a DIFFERENT
  toolchain: `cd web && npm test`; the web CI runs vitest + playwright).

## Pipeline (the S62→S75 loop)
1. **Verify-at-open:** `git log --oneline -3`, HEAD == origin/main, `git status` shows ONLY Caddyfile.prod dirty. Record
   **D-138 IN PROGRESS** in `decisions.md`. Branch `s76-d138`. **CHECK THE DATE** (§2.7 gate ≥ 2026-07-23).
2. **Re-verify vs code** (`mcp__codegraph__codegraph_explore`); take CORE.
3. **Fix → mutation-prove** on `/tmp/pulsemut` (NOT `/mut`). Copy `contracts` too if meta tests read the DDL
   (`cp -a /repo/contracts /tmp/pulsemut/contracts`). `go test -vet=off` for unreachable mutants.
4. **Full Go suite** (25 pkgs) in docker; **`gofmt -l`** on changed files. No web/OpenAPI change expected for [4].
5. **Adversarial review** if the SQL semantics are non-trivial (backend divergence); else careful self-review.
6. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** (server source changed): `config -q` → tag `pulse-prod-pulse:pre-d138` → backup rc0 → STAMPED
   build backgrounded → assert stamp ≠ dev → `up -d pulse` no --build → 5-check smoke.
8. **Close docs:** `decisions.md` D-138 SHIPPED, CHANGELOG, `S73-AUDIT-FINDINGS.md` [4] ✅ DONE, ROADMAP §2.32 count,
   RESUME rotation, `operator-expected.md`, SESSION-76 CLOSED, SESSION-77 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker** (25 pkgs); mutation copy `/tmp/pulsemut` (not `/mut`); copy `contracts` if the meta/api harness
  reads the DDL. `gofmt`/`go` NOT on host PATH. Node at `/home/aytek/.local/bin/node` (npx). Any OpenAPI change → regen
  `web/src/lib/api/schema.d.ts` (`cd web && npm run gen:api`) + it must be committed (web CI drift check).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod at **v0.4.0-87-ge266738**. 5-check smoke: startup
  version stamp, healthz 200, signed webhook 200 (HMAC from PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
