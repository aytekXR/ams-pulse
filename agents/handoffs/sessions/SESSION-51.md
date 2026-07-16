# SESSION-51 — planned at S50 close (D-112)

> Written by SESSION-50 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `agents/handoffs/S48-AUDIT-FINDINGS.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-52

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. The S48 audit produced a 16-finding
list; **each is an AGENT finding — re-verify against the code before building** (S38/S43/S46/S47: several "leads"
were deliberate designs or subtler than stated; S49 [2] was subtler than its ledger summary; S50 [3] required
scoping to the ONE publisher-controlled path segment, NOT the audit's broader "escape app everywhere"). One scope
per PR.

## ★★ Context — working the S48 subsystem-audit backlog (12 findings remain)

The SESSION-48 fresh audit found **16 CONFIRMED** issues. Shipped so far: S48 the cross-tenant audience leak
(D-110); S49 the cross-app StreamID collision cluster [1]+[2] (D-111); **S50 the amsclient streamID URL-escaping [3]
(D-112).** **12 remain**, all in `agents/handoffs/S48-AUDIT-FINDINGS.md` with fix + mutation notes. Work them in
coherent clusters, one scope per PR, mark each ✅ DONE in that ledger as it ships.

## S51 candidate clusters (verify each against the code first)

1. **HIGH — reports-scheduler date/timezone cluster ([4] + [15], `internal/reports/scheduler.go`).** Two coherent
   findings in one file family:
   - **[4] period off-by-one (`scheduler.go:169`)** — `to := time.Date(year, month, 1, ...)` is the first day of the
     **current** month, and the ClickHouse rollup query uses an inclusive `bucket <= ?`, so July-1 rows land in the
     June statement (over-counts viewer-minutes/egress/peak). Ledger fix: `to = firstOfThisMonth.AddDate(0,0,-1)`
     (last day of prev month). **Verify** the inclusive-bound claim against the actual query text and that the same
     `to` flows into `StatementOptions`/filename/header.
   - **[15] local-vs-UTC `nextCronTime` (`scheduler.go:233` + `reports_wave2.go:130/183`)** — `nextCronTime`/
     `NextCronTime` seeded with `time.Now()` (local tz) while `runSchedule` uses `time.Now().UTC()`, so a non-UTC
     server computes `next_run_at` off by the tz offset. Ledger fix: seed with `.UTC()`. **Verify** `nextCronTime`
     is tz-agnostic given a UTC seed (existing tests: `cron_dom_test.go`).
   - These are the same file/subsystem → **one coherent PR.** Mutation targets: a fake-conn arg-capture test for the
     upper bound = `2026-06-30` (not `07-01`); a `cron_dom_test.go` case seeding a non-UTC `FixedZone` time and
     asserting the UTC next-run. **Strong first pick** — self-contained, table-driven, no prod-data dependency.
2. **HIGH — cluster edge-stream status ignored ([5], `cluster/discovery.go:264`, `IsEdgeStream`).** A downed edge
   node keeps its stale non-zero `ActiveStreams`, so `IsEdgeStream` stays true forever → the aggregator permanently
   suppresses origin viewer counts (`aggregator.go:344` skipViewerCount). **Verify** the node-status model: `poll()`
   marks `Status="down"` at `:209` but never clears `ActiveStreams`; the fix adds `n.Status != "down"` to the loop
   predicate. `mockClusterClient.setNodes` in `discovery_test.go` is reusable.
3. **MEDIUM/LOW cluster** — beacon guards ([7] `time.IsZero` for TS==0, [14] 413 byte-heuristic), webhook replay [8],
   restpoller prevStatus leak [9], reports egress-method disclosure [10], clickhouse column/precision [11]/[12]/[13],
   cluster duplicate-key node_stats [16]. Batch where coherent; lower priority.

> Suggested order: cluster **1** (reports-scheduler [4]+[15], self-contained, clear tests), then **2** (cluster
> edge-stream [5]). Do NOT bundle unrelated subsystems in one PR.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S50 (D-112, PR #97) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-41-g60f2a13`** (rollback tag `pre-d112`). `/healthz` all-ok, `ams_env_configured:true`.
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely** — identical sibling text over-matches (S45/S48: unique anchor or Python edit); replacement ending in `{` breaks perl `{}` delimiters → use `#` (S46). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check** — a data-flow change on a touched line can surface a pre-existing alert (S47 CWE-916). Fix the real weakness; don't game the scanner.
- **Contract change? `cd web && npm run gen:api`.** **New migration? FIVE places** (0004 audit_log last shipped → next 0005). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; tag `pre-dNNN` → backup (rc 0) → STAMPED `build --build-arg VERSION/COMMIT/BUILD_DATE` → assert stamp ≠ dev → `up -d` (no `--build`) → smoke. Build >2 min → longer Bash timeout. Roll forward ONLY if server/web *source* changed. Admin token for live smoke in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND candidate-status/mechanism before building (S38/S43/S46/S47/S48/S49/**S50**). An
   audit's *suggested fix scope* can over-reach — S50 escaped only the publisher-controlled segment, not `app`
   (which the same audit had refuted). Take the verified core, not the whole suggestion.
2. Mutation-prove every guard/e2e; drive the real code path with a positive control so the harness can't be vacuous.
3. Independent adversarial review before merge for non-trivial code; for a mechanical mutation-proven fix a compact
   review (or careful self-review) suffices (S48 tenant self-review; S50 2-lens).
4. Positive allowlists over blocklists (D-098). Respect the documented contract/design even when an audit says
   otherwise (S47 idempotent-204; S49 bare-`stream_id` snapshot keying).
5. No silent scope caps; persist verified findings to a ledger so they survive compaction (S48-AUDIT-FINDINGS.md).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-113** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-52; ROADMAP-V2 §2.30 ledger; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-52.md`.
6. **Roll prod forward** if server/web *source* changed.
