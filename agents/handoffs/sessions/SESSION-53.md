# SESSION-53 — planned at S52 close (D-114)

> **✅ CLOSED 2026-07-16 (D-115, PR #103).** Took the cleanest MEDIUM — finding [7]: `collector/ingest/health.go`
> `onIngestStats` guarded a missing timestamp with `if now.IsZero()`, but `time.UnixMilli(0)` is 1970 (not the Go
> zero time), so a `TS==0` event stamped `LastSeen=1970` → `SweepStale` falsely evicted the publisher. Fix:
> `if ev.TS <= 0`. Full Go suite 24/24; mutation-proven (revert → 1970 stamp visible in the sweep log → RED);
> self-reviewed (mechanical); prod `v0.4.0-47-gd32b165`. **8 MEDIUM/LOW findings remain** → SESSION-54. Evidence:
> `decisions.md` D-115. (CI-promotion gate still shut — 07-16 < 07-23.)

> Written by SESSION-52 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `agents/handoffs/S48-AUDIT-FINDINGS.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-54

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. The S48 audit produced a 16-finding
list; **each is an AGENT finding — re-verify against the code before building** (S49 [2] was subtler than its
summary; S50/S51 required taking the verified CORE not the audit's literal suggested scope). One scope per PR.

## ★★ MILESTONE — all 6 HIGH S48-audit findings shipped; 9 MEDIUM/LOW remain

Shipped so far: [6] tenant leak (D-110); [1]+[2] cross-app StreamID (D-111); [3] amsclient escaping (D-112);
[4]+[15] reports period/tz (D-113); [5] cluster edge-stream (D-114). **The remaining 9 are all MEDIUM/LOW** (7
MEDIUM, 2 LOW), in `agents/handoffs/S48-AUDIT-FINDINGS.md`. Work them in coherent clusters, one scope per PR, mark
each ✅ DONE in that ledger as it ships.

## S53 candidate clusters (verify each against the code first)

1. **MEDIUM — collector guards (coherent, small, clean tests).**
   - **[7] `ingest/health.go:172`** — `time.UnixMilli(0).UTC().IsZero()` returns FALSE (it's 1970, not the Go zero
     time), so the `now.IsZero()` fallback never fires for `ev.TS==0` → `pub.LastSeen` stamped 1970 → the next
     `SweepStale` evicts the publisher with a false "source gone" warning. Fix: `if ev.TS <= 0`. **Cleanest first
     pick** — one predicate, mutation test = feed `TS:0` then `SweepStale`, assert survives.
   - **[9] `restpoller.go:455`** — `detectEnded` only removes `p.prevStatus` entries for `status=="broadcasting"`,
     so non-broadcasting (idle/created) streams that disappear from AMS leak forever (unbounded map growth). Fix:
     collect ALL disappeared keys for deletion, keep the `broadcasting` guard only for event emission.
   - **[14] LOW `beacon.go:352`** — 413-vs-400 uses `len(body) >= maxBodyBytes-1` instead of
     `errors.As(err, &http.MaxBytesError)`; a 65535-byte body that then ECONNRESETs wrongly returns 413.
   > These are three different files/subsystems (ingest, restpoller, beacon) — keep as **separate PRs** unless they
   > share a test seam. [7] is the cleanest single-scope start.
2. **MEDIUM — reports egress-method disclosure ([10], `accounting.go:350`).** `UsageReport.EgressMethod` hardcoded
   to `bitrate_x_watch_time` even when per-row used `ams_rest_stats_byte_counter` (set at `:302`) → CSV/PDF header
   misstates the F6 methodology. Self-contained; introduce a `reportEgressMethod` var set in the bytes branch.
3. **MEDIUM/LOW — clickhouse + cluster.**
   - **[11] `query.go:1084`** — `AnomalyBaselineForMetric` viewer_count case queries `avg(viewers)`/`event_time`
     but the columns are `viewer_count`/`ts` → silent zero baseline. Verify against the DDL; unit tests use a fake
     conn that returns fixed values regardless of SQL text, so this needs a real-CH integration test or a SQL-text
     assertion seam.
   - **[13] `clickhouse.go:550`** — `insertBeaconEvents` does `PrepareBatch` per item (partial commit + wrong
     metrics on mid-batch failure). Fix: hoist `PrepareBatch` out, one `Send()` after all appends (mirror
     `insertServerEvents`). `mockConn`/`mockBatch` scaffolding exists in `drain_test.go`.
   - **[16] LOW `discovery.go:145`** — two ClusterNodeDTOs resolving to the same key (both empty NodeID+IP → "")
     each emit a node_stats event. Fix: dedup guard at the top of the poll loop.
   - **⚠ [12] `0001_init.sql:358`** — `peak_concurrency` missing from the SummingMergeTree column list →
     under-reported after merges. **NEEDS A MIGRATION (FIVE places, next = 0005)** + an `ALTER TABLE … MODIFY
     ENGINE`. Heavier; verify the ALTER path and the five-place migration checklist before starting.
4. **MEDIUM — webhook replay ([8], `webhook.go:160`). ⚠ Verify product-viability FIRST.** Needs a new
   `X-Ams-Timestamp` header + an AMS/signing-proxy convention to include the timestamp in the HMAC. If it requires
   an AMS-side or signing-proxy change, it is **operator/contract-gated** — record it as such rather than forcing a
   unilateral protocol change that AMS won't send.

> Suggested order: **[7]** (cleanest), then **[9]**/**[10]**/**[13]**/**[16]** as separate small PRs, then **[11]**
> (needs a CH test seam), then **[12]** (migration) and **[8]** (product-gated) last. Do NOT bundle unrelated
> subsystems in one PR.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S52 (D-114, PR #101) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-45-g0ab487f`** (rollback tag `pre-d114`). `/healthz` all-ok, `ams_env_configured:true`.
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely** — identical sibling text over-matches (S45/S48: unique anchor or Python edit); replacement ending in `{` breaks perl `{}` delimiters → use `#` (S46). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check** — a data-flow change on a touched line can surface a pre-existing alert (S47 CWE-916). Fix the real weakness; don't game the scanner.
- **Contract change? `cd web && npm run gen:api`.** **New migration? FIVE places** (0004 audit_log last shipped → next 0005; **[12] needs this**). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; tag `pre-dNNN` → backup (rc 0) → STAMPED `build --build-arg VERSION/COMMIT/BUILD_DATE` → assert stamp ≠ dev → `up -d` (no `--build`) → smoke. Build >2 min → longer Bash timeout. Roll forward ONLY if server/web *source* changed. Admin token for live smoke in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND candidate-status/mechanism before building (S38…S52). Take the audit's verified
   CORE, not its literal suggested scope (S50/S51). An existing test may pass **trivially** — trace it (S49 [2]).
   Some findings (e.g. [8] webhook replay, [12] migration) need product/ops verification before coding.
2. Mutation-prove every guard/e2e; drive the real code path with a positive control. Prefer a pure helper for
   deterministic logic (S51). For DB-text bugs ([11]) a fake-conn returning fixed values is VACUOUS — need a SQL
   assertion seam or real CH.
3. Independent adversarial review before merge; a compact 1–2 lens review (or careful self-review for a purely
   mechanical mutation-proven fix) suffices (S48/S50/S51/S52).
4. Positive allowlists over blocklists (D-098). Respect the documented contract/design even when an audit says
   otherwise (S47 idempotent-204; S49 bare-`stream_id` keying).
5. No silent scope caps; persist verified findings to a ledger (S48-AUDIT-FINDINGS.md). State latency honestly (S51).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-115** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-54; ROADMAP-V2 §2.30 ledger; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-54.md`.
6. **Roll prod forward** if server/web *source* changed.
