# SESSION-60 — planned at S59 close (D-121)

> **⏸️ CLOSED 2026-07-16 (D-122). DEFERRED S48-audit finding [12] — no migration shipped.** Took [12]
> (`0001_init.sql:358` — `rollup_usage_1d` SummingMergeTree omits `peak_concurrency` from the sum-list). The
> mechanism is real (the column isn't summed) BUT the impact is **REFUTED**: a whole-repo grep confirms **nothing
> reads `rollup_usage_1d.peak_concurrency`** — every peak READ comes from an AggregatingMergeTree via `maxMerge`
> (billing `accounting.go:389-412` → `rollup_concurrency_1d`; analytics `query.go:285` → `rollup_audience_1h/1d`; web
> `ReportsPage.tsx:866` shows the API value fed from those). `accounting.go:209-210` documents the column as an unread
> "session-count proxy, not true concurrency." This is a human-approved, integration-tested design — **D-018 CR-VD38**
> created `0002_concurrency_rollup.sql` for exactly this (`TestAccountant_CHIntegration`: TRUE windowed max, drift
> 0.0000%, D-019) and states "Do NOT edit `0001_init.sql`." **Ruling: DEFER** — the audit's fix would be inert (no
> reader), semantically wrong if ever read (summing `toUInt32(1)`/session = session-count, not peak), and risky (live
> `ALTER … MODIFY ENGINE` on the billing table). Also caught: the CH migration lineage is already at **0010**, not 0004
> ("next=0005" was the meta-store audit_log lineage). No code/DDL change (live read-path already pinned at
> `accounting.go:209-211`); **no prod roll** (prod stays `v0.4.0-57-g36c16ed`). **1 finding remains: [8] webhook
> replay (product/contract-gated)** → SESSION-61. Evidence: `decisions.md` D-122. (CI-promotion gate still shut —
> 07-16 < 07-23.)

> Written by SESSION-59 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE for the full ranked candidate list** + `S48-AUDIT-FINDINGS.md`.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-61

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S48 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S49 [2] subtler;
S50/S51/S58 narrowed; S55 [10] BROADENED; **S59 [11] DEFERRED** — re-verification found it a dead-code duplicate of
D-087's already-parked latent bug, so no fix shipped). One scope per PR.

## ★ Context — the FINAL TAIL (2 actionable findings; [11] deferred, all others shipped)

Shipped: [6] D-110, [1]+[2] D-111, [3] D-112, [4]+[15] D-113, [5] D-114, [7] D-115, [9] D-116, [10] D-117,
[13] D-118, [16] D-119, [14] D-120. **Deferred: [11] D-121** (dead-code dup of D-087 — see below). **2 remain —
both MEDIUM, each needing MORE than a code tweak:**

- **⚠ [12] MEDIUM** `contracts/db/clickhouse/0001_init.sql:358` — `rollup_usage_1d` is
  `SummingMergeTree((viewer_minutes, egress_bytes, recording_bytes))`; `peak_concurrency` (UInt32, `toUInt32(1)` per
  session in `mv_usage_1d`, comment "summed per key") is **missing from the sum-columns list** → after a background
  merge it is NOT summed (kept from one merged row) → `sum(peak_concurrency)` collapses toward 1 → **underreported**
  billing/peak figures. **Migrations are forward-only and frozen (runbook §"CH DDL stance"); do NOT edit 0001.**
  Fix = a NEW migration `0005`:
  `ALTER TABLE {db}.rollup_usage_1d MODIFY ENGINE = SummingMergeTree((viewer_minutes, peak_concurrency, egress_bytes, recording_bytes))`
  (ClickHouse ≥ 22.6). **Plumbing (grep where `0004` is registered — expect ~FIVE places):** the embedded-FS
  migration list, the migrations directory, any golden/DDL-parity test (`ci.yml` ~line 311), the SQLite-vs-PG parity
  path if applicable, and docs. **Verify the mutation with the integration harness** (`go test -tags integration` —
  insert N rows with `peak_concurrency=1` for one ORDER BY key, `OPTIMIZE TABLE … FINAL`, assert
  `sum(peak_concurrency)=N`; before the fix → 1). **Confirm the running prod schema BEFORE writing the ALTER**
  (`SHOW CREATE TABLE rollup_usage_1d` via the prod clickhouse container — the deployed engine may already differ
  from 0001, which changes whether the ALTER is a no-op or a real change). **Prod deploy of a migration = the migrate
  one-shot runs on `up -d`; take the pre-upgrade backup as always, and verify the ALTER landed post-`up`.** This is
  the mechanical-but-heavy win — **do it first.**

- **⚠ [8] MEDIUM** `collector/webhook/webhook.go:160` webhook replay — no freshness check; any captured signed
  webhook can be replayed indefinitely. Fix needs a new `X-Ams-Timestamp` header + a ±window check folded into the
  HMAC. **This is a CONTRACT change with the AMS/signing-proxy — verify product-viability FIRST:** does AMS (or the
  deployed signing proxy) actually SEND a timestamp header? Check `docs/AMS-INTEGRATION.md`, the signing-proxy
  config, and what AMS's webhook actually emits. If it does NOT, this is **operator/contract-gated** — record the
  blocker in `operator-expected.md` + the session log and DO NOT ship a half-measure (a timestamp check that rejects
  every real webhook because AMS never sends the header would break live ingest). May not be a pure code fix.

**Suggested order: [12] first** (mechanical, heavy plumbing — a clean autonomous win, DDL change ⇒ roll prod
forward), then **[8]** (product/contract gate — may hand off to operator; if gated, that's a legitimate stop for a
human dependency per the standing directive).

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S59 (D-121, PR #NNN — combined code-pin + docs) should be on `origin/main`.
- Prod should print **`v0.4.0-57-g36c16ed`** (UNCHANGED — S59 shipped a comment-only pin, no roll). `/healthz`
  all-ok, `ams_env_configured:true`. **[12] WILL roll prod forward** (DDL change via the migrate one-shot).
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` (both green through the bake) is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`. **Integration tests** ([12] verification): `go test -tags integration ./...` needs real CH+PG — see CI `ci.yml` job "Integration tests" for the env (`PULSE_CLICKHOUSE_DSN`, `PULSE_META_TEST_PG_DSN`, `PULSE_MIGRATIONS_DIR`); replicate its ClickHouse download (pinned `CH_VER=26.6.1.1193`) if running locally.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). Gate: `sh -c 'D=$(gofmt -l .); [ -z "$D" ] && go build ./... && go vet ./... && go test ./... || { echo DIRTY: $D; exit 1; }'`. (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely**; replacement ending in `{` breaks perl `{}` → use `#`. For **[12]** the mutation is the migration itself: run the integration assert with `peak_concurrency` OUT of the engine list (result = 1) vs IN (result = N). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** — CI's `web` job has a **types-drift guard**; regenerate with `npm ci --legacy-peer-deps` (node 22) so `schema.d.ts` matches CI byte-for-byte (S55). **New migration ([12])? FIVE places** (0004 → next 0005) + golden-file diff checks in `ci.yml` (~line 311). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback point is a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` → backup (`exec -T backup … once`, rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (backgrounded, >2 min) → assert stamp ≠ dev → `up -d` (no `--build`) → smoke (healthz, version, signed webhook 200, limits 512M/0.5cpu, logs clean). **[12] deploys a migration** — the migrate one-shot runs on `up -d`; back up first, then verify the ALTER landed (`SHOW CREATE TABLE rollup_usage_1d` shows `peak_concurrency` in the engine args; `sum(peak_concurrency)` post-merge is correct). Roll forward ONLY if server/web *source* (or DDL) changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER (S50/S51/S58),
   BROADER (S55), or **DEFER** (S59 [11] — a dead-code dup of a standing decision D-087). Trace an existing test
   before trusting it (S49 [2]). **[8], [12] need product/ops verification before coding**; [8] may be a legitimate
   human-dependency stop.
2. Mutation-prove every guard; positive control so the harness can't be vacuous. **For DB-engine/DDL bugs ([12]) the
   mutation is the migration** — assert `sum()` before/after the merge in the integration harness. Respect documented
   deferrals: don't fix dead code an ADR already parked (S59).
3. Independent review before merge; compact 1–2 lens OR careful self-review for a purely mechanical mutation-proven
   fix (S53/S54/S56/S57/S58). A migration touching billing figures ([12]) or a security/contract change ([8])
   warrants the multi-lens adversarial workflow.
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees
   (S59). Migrations are forward-only ([12] → new 0005, never edit 0001).
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly. A DEFERRAL is
   recorded as ⏸️ DEFERRED with its rationale (not silently dropped, not falsely marked DONE).
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-122** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-61; ROADMAP-V2 §2.30 ledger; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md` (esp. if [8] turns out operator/contract-gated).
5. Write `sessions/SESSION-61.md`.
6. **Roll prod forward** if server/web *source* (or DDL) changed ([12] = yes; verify the migration landed).
