# SESSION-87 — planned at S86 close (D-148) — F6 PHASE 2: tenant-scoped QoE alert rules ([5])

> **✅ CLOSED 2026-07-18 (D-149).** Shipped F6 Phase 2: `domain.AlertScope.tenant` (in ScopeJSON → NO migration;
> backward-compatible) + `QoEReader.QoEForStream` `tenant` param threaded from `scope.Tenant` by the evaluator. A rule
> scoped `{"tenant":"acme"}` reads only acme's QoE rows; no API handler change (scope passes opaquely). **★ S73 finding
> [5] CLOSED → S73 audit 8/8 shipped.** Mutation-proven; full suite + web green; prod-rolled `v0.4.0-114-ge295795` (5-check
> smoke green). PR #171. Verify-first shrank the change (no migration; read-level fix). → **SESSION-88 = F6 Phase 3 ([20]
> audit-read — possibly a product call).** See RESUME ▶ START HERE.

> Written by SESSION-86 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`
> (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-112-g75031e7** (F6 Phase 1 live).
> F6 is an operator-directed feature ("start F6", D-148). Phase 1 (live tenant resolution + BUG-009) is DONE. This is
> Phase 2 — thread the resolved tenant into the alert evaluator so QoE alert rules can be scoped to one tenant ([5]).

## ⚡ STANDING DIRECTIVE — still in force
Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and choose the next-highest-leverage move. **F6 is the active operator
priority** — continue it phase by phase unless the operator redirects (Lead B: if they name a different priority, do that).
Verify status + viability against the code before committing. **Ultracode is on** (apply to quality of real work).
**Workflow gotcha:** no backticks in workflow prompt prose. `gofmt -l` before every push.

## THE GOAL: [5] tenant-scoped QoE alert rules (F6 Phase 2)

**The gap (D-141, S73 [5] defer-by-ruling):** the alert evaluator has NO tenant. `AlertScope`/`AlertRuleRow`/`LiveStream`
carry no tenant, so a QoE alert rule for a stream that two tenants happen to reuse (same app + stream name) blends both
tenants' rebuffer/error numbers. Phase 1 gave us server-side tenant resolution (`internal/tenant`); Phase 2 uses it in the
alert path.

## Lead — the Phase 2 increment (contracts first, then code)
1. **Verify-at-open:** re-read `decisions.md` D-141 ([5] ruling) + `agents/handoffs/S73-AUDIT-FINDINGS.md` finding [5] +
   the alert evaluator (`server/internal/alert/`) + the `AlertRule`/`AlertScope`/`AlertRuleRow` shapes (`internal/domain`
   + `internal/store/meta`) + how `QoEForStream` is evaluated. Confirm the exact blend path against the code (the S73
   finding may name specific files/lines). Record **D-149 IN PROGRESS**. Branch `s87-d149-f6p2`.
2. **Design the contract (contracts before code):** add an optional `tenant` scope to the alert rule — a nullable
   `tenant` field on `AlertRule` (OpenAPI + meta DDL migration + `schema.d.ts`). A rule with `tenant=""` behaves exactly
   as today (all tenants — backward compatible); a rule with `tenant=acme` fires only on acme's streams. **UX note for the
   operator:** how the rule targets a tenant is a product choice — implement the sensible default (an optional field) and
   flag it in `operator-expected.md` for their review.
3. **Implement:** thread the resolved tenant into the evaluator. Reuse the Phase-1 `internal/tenant` resolver (the alert
   evaluator will need a tenant resolver wired the same way `query.Service` got `SetTenantResolver`). When evaluating a
   tenant-scoped rule against a stream, resolve the stream's tenant and skip the stream if it doesn't match the rule's
   tenant. **Fail-closed** like Phase 1 (a tenant-scoped rule never fires on a stream that resolves to a different/no
   tenant). Watch the migration: a nullable column with a backward-compatible default; existing rules keep `tenant=NULL`
   → all-tenants.
4. **Validate:** Go full 25-pkg suite via docker + **mutation-prove** the tenant-scoping guard (a mutation that drops the
   tenant check must be killed by a test showing a cross-tenant alert fire). Web (if the rule UI is touched) full
   `npm test`/build/typecheck/lint + `schema.d.ts` regen.
5. **Adversarial review (mandatory — data-isolation + alerting correctness).**
6. **PR → CI → squash-merge --delete-branch → verify origin/main.** (Two-PR cadence: arc, then docs-close.)
7. **Roll prod forward** (server source changes → stamped rebuild + 5-check smoke; a meta DDL migration runs on deploy —
   verify `pulse-migrate` applies it cleanly and existing alert rules still evaluate).
8. **Close docs:** D-149, CHANGELOG (user-facing: tenant-scoped alert rules), ROADMAP §2.37 (Phase 2 ✅), RESUME →
   SESSION-88 (Phase 3 = [20] audit-read, or next operator priority), `operator-expected.md`, SESSION-87 CLOSED,
   SESSION-88 written. Re-arm the `/loop`.

## F6 phase map (track across sessions)
- **Phase 1 ✅ (D-148, S86):** server-side tenant resolution on the live endpoints; `?tenant=` filter; BUG-009 closed.
- **Phase 2 (this, [5]):** tenant-scoped QoE alert rules.
- **Phase 3 ([20]):** audit-log read model (tenant-scoped admin reads).
- After Phase 3: re-survey — F6 core is then complete; remaining multi-tenant polish is demand-driven.

## Environment gotchas (carried — unchanged from SESSION-86)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`.
  `gofmt`/`go` NOT on host PATH. Mutation copy `/tmp/pulsemut`; restore via `cp` (NEVER `git checkout`, D-096).
  Node at `/home/aytek/.local/bin/node`. `internal/tenant` (Matcher + CachedResolver) is the shared resolver — reuse it.
- **Web:** from `web/`, `npm test`; `npm run gen:api` regenerates `src/lib/api/schema.d.ts`. A new query param needs a
  `param_conformance_test.go` registry entry + floor bumps (D-147 pattern).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. **D-058 stamped build:** `docker compose $DC build
  --build-arg VERSION=$(git describe --tags --always) --build-arg COMMIT=$(git rev-parse --short HEAD) --build-arg
  BUILD_DATE=$(date -u ...) pulse` THEN `docker compose $DC up -d pulse` (never mix `--build` into `up -d`). Prod
  **v0.4.0-112-g75031e7**; rollback tags `pulse-prod-pulse:pre-d148` / `:pre-d148-fix`. Read-only rootfs — new writes →
  `/var/lib/pulse` or `/tmp`. 5-check smoke: version stamp, healthz 200, signed webhook 200 (HMAC from
  PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines. Admin token (side-effect-free GET only, never commit):
  gitignored `oguz-testing.md`; API base `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
