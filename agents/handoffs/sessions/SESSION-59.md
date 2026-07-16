# SESSION-59 — planned at S58 close (D-120)

> Written by SESSION-58 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE for the full ranked candidate list** + `S48-AUDIT-FINDINGS.md`.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-60

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S48 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S49 [2] subtler;
S50/S51 narrowed; S55 [10] BROADENED; S58 [14] narrowed — kept a check the audit wrongly called dead). One scope/PR.

## ★ Context — the HARDER TAIL (3 remain; all clean/mechanical findings shipped)

Shipped: [6] D-110, [1]+[2] D-111, [3] D-112, [4]+[15] D-113, [5] D-114, [7] D-115, [9] D-116, [10] D-117,
[13] D-118, [16] D-119, [14] D-120. **3 remain — all MEDIUM, each needing MORE than a code tweak:**
- **[11] MEDIUM** `query/query.go:1084` — `AnomalyBaselineForMetric` viewer_count case queries `avg(viewers)` /
  `event_time` but the real columns are `viewer_count` / `ts` (per `0001_init.sql`). ClickHouse errors "Unknown
  identifier" → caught → returns `(0,0,0,nil)` → **silent zero baseline** (baseline-driven alerting never fires).
  Fix is a 1-line column rename, BUT **the existing unit tests use a fake conn that returns fixed values regardless
  of the SQL text, so a naive unit test is VACUOUS.** The real work is the TEST SEAM. Two options:
  (a) **SQL-text assertion seam** (preferred, fast): extend the fake conn to record the query string passed to
      `Query`/`QueryRow`, then assert it references `viewer_count` and `ts` (and NOT `viewers`/`event_time`).
      Mutation-provable: revert the columns → the text assertion reddens. Check whether the fake conn already
      captures SQL; if not, add a `lastQuery` field.
  (b) **Real-CH integration test** (`go test -tags integration`, CI's `server` job runs it with real ClickHouse on
      pinned `CH_VER=26.6.1.1193`): seed `server_events` with `viewer_count` rows and assert a non-zero baseline.
      Heavier; use only if the text-seam can't be made non-vacuous. **Verify the exact columns against
      `contracts/db/clickhouse/0001_init.sql` (ts at ~line 47, viewer_count at ~line 96) before editing the query.**
- **⚠ [12] MEDIUM** `contracts/db/clickhouse/0001_init.sql:358` — `peak_concurrency` missing from the
  `SummingMergeTree((viewer_minutes, egress_bytes, recording_bytes))` column list → after a background merge, its
  value is NOT summed (kept from one row) → underreported. **Migrations are forward-only and frozen (runbook §"CH
  DDL stance"); do NOT edit 0001.** Fix = a NEW migration `0005` with `ALTER TABLE {db}.rollup_usage_1d MODIFY ENGINE
  = SummingMergeTree((viewer_minutes, peak_concurrency, egress_bytes, recording_bytes))` (ClickHouse ≥ 22.6). **FIVE
  places** wire migrations — grep for where `0004` is registered (embedded FS list, migrations dir, any golden/DDL
  test, docs) and add `0005` to all. Verify the mutation with the integration-test harness (insert N rows, OPTIMIZE
  FINAL, assert `sum(peak_concurrency)=N`). Confirm the running prod schema BEFORE writing the ALTER (the prod table
  may already differ). Prod deploy of a migration = the migrate one-shot runs on `up -d`; take the pre-upgrade
  backup as always.
- **⚠ [8] MEDIUM** `collector/webhook/webhook.go:160` webhook replay — no freshness check; any captured signed
  webhook can be replayed. Fix needs a new `X-Ams-Timestamp` header + a ±window check + folding the timestamp into
  the HMAC. **This is a CONTRACT change with the AMS/signing-proxy** — verify product-viability FIRST: does AMS (or
  the deployed signing proxy) actually send a timestamp header? If not, this is **operator/contract-gated** — record
  it in `operator-expected.md` and the session log rather than shipping a half-measure. May not be a pure code fix.

**Suggested order: [11] first** (real bug, high value — restores baseline alerting — once the seam is non-vacuous),
then **[12]** (mechanical fix but heavy plumbing), then **[8]** (product/contract gate — may hand off to operator).

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S58 (D-120, PR #113) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-57-g36c16ed`** (rollback image tag `pre-d120` → `v0.4.0-55-ge13eb1f`). `/healthz`
  all-ok, `ams_env_configured:true`.
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` (both green through the bake) is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`. **Integration tests** ([11]/[12] verification): `go test -tags integration ./...` needs real CH+PG — see CI `ci.yml` job "Integration tests" for the env (`PULSE_CLICKHOUSE_DSN`, `PULSE_META_TEST_PG_DSN`, `PULSE_MIGRATIONS_DIR`); replicate its ClickHouse download (pinned `CH_VER=26.6.1.1193`) if running locally.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY** mounted read-only at `/repo`. **Target precisely**; replacement ending in `{` breaks perl `{}` → use `#`. Prefer compiling mutations (RED test) (S55); awk-splice for whole-function reverts (S56); drop a `continue`/one line for guard reverts (S57); for [11] a column-name revert reddens the SQL-text assertion. Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** — CI's `web` job has a **types-drift guard**; regenerate with `npm ci --legacy-peer-deps` (node 22) so `schema.d.ts` matches CI byte-for-byte (S55). **New migration ([12])? FIVE places** (0004 → next 0005) + golden-file diff checks in `ci.yml` (~line 311). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback point is a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` → backup (rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (backgrounded, >2 min) → assert stamp ≠ dev → `up -d` (no `--build`) → smoke. **[12] deploys a migration** — the migrate one-shot runs on `up -d`; back up first, and verify the ALTER landed (SQLite WAL gotcha does NOT apply to CH, but confirm `sum(peak_concurrency)` post-merge). Roll forward ONLY if server/web *source* (or DDL) changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER (S50/S51/S58) or
   BROADER (S55) than the audit's literal scope. Trace an existing test before trusting it (S49 [2]). [8], [12] need
   product/ops verification before coding.
2. Mutation-prove every guard; positive control so the harness can't be vacuous. **For DB-text bugs ([11]) a
   fake-conn returning fixed values is VACUOUS — need a SQL-text assertion seam or real CH.** Prefer compiling
   mutations (RED test) over build-breaking ones (S55).
3. Independent review before merge; compact 1–2 lens OR careful self-review for a purely mechanical mutation-proven
   fix (S53/S54/S56/S57/S58). A SEMANTIC/product decision (S55's `mixed`; possibly [8]) warrants the multi-lens
   adversarial workflow.
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees;
   document new API-returned values in the contract (S55). Migrations are forward-only ([12] → new 0005, never edit 0001).
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly.
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-121** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-60; ROADMAP-V2 §2.30 ledger; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md` (esp. if [8] turns out operator/contract-gated).
5. Write `sessions/SESSION-60.md`.
6. **Roll prod forward** if server/web *source* (or DDL) changed.
