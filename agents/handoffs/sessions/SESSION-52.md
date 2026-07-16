# SESSION-52 — planned at S51 close (D-113)

> Written by SESSION-51 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `agents/handoffs/S48-AUDIT-FINDINGS.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-53

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. The S48 audit produced a 16-finding
list; **each is an AGENT finding — re-verify against the code before building** (S38/S43/S46/S47: several "leads"
were deliberate designs or subtler than stated; S49 [2] subtler than its summary; S50/S51 required taking the
verified CORE, not the audit's literal suggested scope). One scope per PR.

## ★★ Context — working the S48 subsystem-audit backlog (10 findings remain)

The SESSION-48 fresh audit found **16 CONFIRMED** issues. Shipped so far: S48 the cross-tenant audience leak
(D-110); S49 cross-app StreamID collision [1]+[2] (D-111); S50 amsclient streamID URL-escaping [3] (D-112); **S51
reports-scheduler date/tz [4]+[15] (D-113).** **10 remain**, all in `agents/handoffs/S48-AUDIT-FINDINGS.md` with fix
+ mutation notes. Work them in coherent clusters, one scope per PR, mark each ✅ DONE in that ledger as it ships.

## S52 candidate clusters (verify each against the code first)

1. **HIGH (the LAST HIGH) — cluster edge-stream status ignored ([5], `cluster/discovery.go:264`, `IsEdgeStream`).**
   `IsEdgeStream` returns true for any node where `Role=="edge" && ActiveStreams>0`, with **no `Status` check**. When
   an edge node disappears, `poll()` marks it `Status="down"` (`:209`) but never clears `ActiveStreams`, so the
   stale non-zero count keeps `IsEdgeStream` true forever → the aggregator (`aggregator.go:344`) sets
   `skipViewerCount=true` for origin `node_stats` → **origin viewer counts are permanently suppressed** (frozen/zero).
   Ledger fix: add `n.Status != "down"` to the loop predicate (self-documenting at the point of use). **Verify** the
   node-status model + that `mockClusterClient.setNodes` in `discovery_test.go` lets a test drive down-marking after
   `StaleTimeout`. **Strong pick** — self-contained, one predicate, clear mutation test.
2. **MEDIUM cluster — collector guards (coherent, small).** [7] `ingest/health.go:172` `time.IsZero()` never fires
   for `ev.TS==0` (`time.UnixMilli(0)` is 1970, not Go zero) → publisher stamped 1970 → false "source gone" eviction;
   fix `if ev.TS <= 0`. [9] `restpoller.go:455` `detectEnded` leaks `p.prevStatus` entries for non-broadcasting
   streams that disappear (map grows unbounded); fix decouples eviction from event emission. [14] `beacon.go:352`
   413-vs-400 uses a byte-count heuristic instead of `errors.As(&http.MaxBytesError)`. Verify each; batch the
   coherent ones but keep beacon vs restpoller vs ingest as separate scopes if they don't share a test seam.
3. **MEDIUM — reports egress-method disclosure ([10], `accounting.go:350`).** `UsageReport.EgressMethod` hardcoded to
   `bitrate_x_watch_time` even when per-row used `ams_rest_stats_byte_counter` → CSV/PDF header misstates the F6
   methodology. Self-contained reports fix; verify against the per-row assignment at `:302`.
4. **MEDIUM — webhook replay protection ([8], `webhook.go:160`).** No timestamp/nonce freshness check → a captured
   signed webhook replays forever. **Verify product-viability carefully**: this needs a new `X-Ams-Timestamp` header
   + AMS-side signing convention — may be an operator/contract decision, not a pure code fix. If it requires an AMS
   or signing-proxy change, record it as operator-gated rather than forcing a unilateral protocol change.
5. **MEDIUM/LOW — clickhouse [11]/[12]/[13] + cluster [16].** [11] `query.go:1084` wrong column names (`viewers`/
   `event_time` vs `viewer_count`/`ts`) silently zero the anomaly baseline. [12] `0001_init.sql:358` `peak_concurrency`
   missing from the SummingMergeTree column list (needs a migration — FIVE places). [13] `clickhouse.go:550`
   per-item `PrepareBatch` → partial commit + wrong metrics. [16] `discovery.go:145` duplicate node_stats on
   same resolved key. ⚠ [12] touches contracts/migrations — heavier; verify the ALTER path.

> Suggested order: cluster **1** ([5], last HIGH, clean), then **2** (collector guards) or **3** (egress disclosure).
> [8] and [12] need product/ops verification before building. Do NOT bundle unrelated subsystems in one PR.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S51 (D-113, PR #99) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-43-g7c206a9`** (rollback tag `pre-d113`). `/healthz` all-ok, `ams_env_configured:true`.
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely** — identical sibling text over-matches (S45/S48: unique anchor or Python edit); replacement ending in `{` breaks perl `{}` delimiters → use `#` (S46). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check** — a data-flow change on a touched line can surface a pre-existing alert (S47 CWE-916). Fix the real weakness; don't game the scanner.
- **Contract change? `cd web && npm run gen:api`.** **New migration? FIVE places** (0004 audit_log last shipped → next 0005; [12] would need this). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; tag `pre-dNNN` → backup (rc 0) → STAMPED `build --build-arg VERSION/COMMIT/BUILD_DATE` → assert stamp ≠ dev → `up -d` (no `--build`) → smoke. Build >2 min → longer Bash timeout. Roll forward ONLY if server/web *source* changed. Admin token for live smoke in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND candidate-status/mechanism before building (S38/S43/S46/S47/S48/S49/S50/S51). Take
   the audit's verified CORE, not its literal suggested scope (S50 escaped only streamID not app; S51 normalized
   inside nextCronTime not at 3 call sites). An existing test may pass **trivially** — trace it (S49 [2]).
2. Mutation-prove every guard/e2e; drive the real code path with a positive control so the harness can't be vacuous.
   Prefer extracting a pure helper for deterministic date/math logic (S51 `previousCalendarMonthUTC`).
3. Independent adversarial review before merge; a compact 2-lens review (or careful self-review for a purely
   mechanical mutation-proven fix) suffices (S48/S50/S51).
4. Positive allowlists over blocklists (D-098). Respect the documented contract/design even when an audit says
   otherwise (S47 idempotent-204; S49 bare-`stream_id` keying).
5. No silent scope caps; persist verified findings to a ledger so they survive compaction (S48-AUDIT-FINDINGS.md).
   State latency honestly (S51 [15] latent on UTC prod).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-114** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-53; ROADMAP-V2 §2.30 ledger; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-53.md`.
6. **Roll prod forward** if server/web *source* changed.
