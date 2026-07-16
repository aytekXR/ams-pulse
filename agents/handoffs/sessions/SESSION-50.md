# SESSION-50 — planned at S49 close (D-111)

> Written by SESSION-49 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `agents/handoffs/S48-AUDIT-FINDINGS.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-51

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. The S48 audit produced a 16-finding
list; **each is an AGENT finding — re-verify against the code before building** (S38/S43/S46/S47: several "leads"
were deliberate designs or subtler than stated; **S49 finding [2] was subtler than its one-line ledger summary** —
the existing test passed trivially and the guard is a proportionate, not total, fix). One scope per PR.

## ★★ Context — working the S48 subsystem-audit backlog (13 findings remain)

The SESSION-48 fresh audit found **16 CONFIRMED** issues. Shipped so far: S48 the cross-tenant audience leak
(D-110); **S49 the cross-app StreamID collision cluster — findings [1]+[2], one root cause (D-111).** **13 remain**,
all in `agents/handoffs/S48-AUDIT-FINDINGS.md` with fix + mutation notes. Work them in coherent clusters, one scope
per PR, mark each ✅ DONE in that ledger as it ships.

## S50 candidate clusters (verify each against the code first)

1. **HIGH — `amsclient` streamID not URL-path-escaped** ([3], `pkg/amsclient/client.go:475`, `WebRTCClientStats`).
   A stream id with `#`/`?`/space/slash hits the wrong AMS endpoint silently (the `#` case: `url.Parse` splits the
   fragment → GET goes to the single-broadcast-detail endpoint → AMS returns `null` → `json.Decode(null)` yields a
   nil slice + nil error → the `err==nil` gate at `restpoller.go:420` drops the stats with no log). AMS wire formats
   live in `pkg/amsclient` per ARCHITECTURE §3. **Verify:** the audit REFUTED the `app`/`nodeID` escaping leads as
   already-safe — confirm why `streamID` differs, and whether the sibling `ListBroadcasts`/`ListVods` path-builders
   (`:436/:455/:576/:589`) need the same `url.PathEscape`. Existing pattern: `TestListBroadcasts_UsesPerAppPathParams`
   in `client_test.go`. **Strong first pick** — self-contained, one package, a clear httptest-captures-the-path test.
2. **HIGH — scheduled-report period off-by-one** ([4], `reports/scheduler.go:169`) — `to` is the **first day of the
   current month**, and the ClickHouse rollup query uses an inclusive `bucket <= ?`, so July-1 rows land in the June
   statement (over-counts viewer-minutes/egress/peak). **Bundle [15]** (`scheduler.go:233` + `reports_wave2.go:130/183`)
   — `nextCronTime`/`NextCronTime` seeded with `time.Now()` (local tz) while the rest of `runSchedule` is UTC, so a
   non-UTC-deployed server fires schedules off by the tz offset. Same file family; coherent reports-scheduler cluster.
   **Verify** the date-range math + the inclusive-bound claim against the actual query, and the tz seeding.
3. **HIGH — cluster edge-stream status ignored** ([5], `cluster/discovery.go:264`, `IsEdgeStream`) — a downed edge
   node keeps its stale non-zero `ActiveStreams`, so `IsEdgeStream` stays true forever → the aggregator permanently
   suppresses origin viewer counts. **Verify** the node-status model (`poll()` marks `Status="down"` at `:209` but
   never clears `ActiveStreams`) and that the `n.Status != "down"` guard is the right seam.
4. **MEDIUM/LOW cluster** — beacon guards ([7] `time.IsZero` for TS==0, [14] 413 byte-heuristic), webhook replay
   protection [8], restpoller prevStatus leak [9], reports egress-method disclosure [10], clickhouse column/precision
   [11]/[12]/[13], cluster duplicate-key node_stats [16]. Batch where coherent; lower priority.

> Suggested order: cluster **1** (amsclient, self-contained, clear test), then **2** (reports-scheduler, [4]+[15]),
> then **3** (cluster edge-stream). Do NOT bundle unrelated subsystems in one PR.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S49 (D-111, PR #95) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-39-gc08ad6a`** (rollback tag `pre-d111`). `/healthz` all-ok, `ams_env_configured:true`.
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1` (test cache ignores the runtime-read spec).
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely** — identical sibling text over-matches (S45/S48: use a unique anchor or a Python edit); replacement ending in `{` breaks perl `{}` delimiters → use `#` (S46). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check** — a data-flow change on a touched line can surface a pre-existing alert (S47 CWE-916). Fix the real weakness; don't game the scanner.
- **Contract change? `cd web && npm run gen:api`.** **New migration? FIVE places** (0004 audit_log last shipped → next 0005). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; tag `pre-dNNN` → backup (rc 0) → STAMPED `build --build-arg VERSION/COMMIT/BUILD_DATE` → assert stamp ≠ dev → `up -d` (no `--build`) → smoke. Build >2 min → longer Bash timeout. Roll forward ONLY if server/web *source* changed. Admin token for live smoke in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND candidate-status/mechanism before building (S38/S43/S46/S47/S48/**S49**). An existing
   test that "covers" the scenario may pass **trivially** — trace it before trusting it (S49 [2]).
2. Mutation-prove every guard/e2e; drive the real code path with a positive control so the harness can't be vacuous.
3. Independent adversarial review before merge for non-trivial code; for a mechanical fix that mirrors proven
   siblings + is mutation-proven (S48 tenant), a careful self-review can substitute.
4. Positive allowlists over blocklists (D-098). Respect the documented contract/design even when an audit says
   otherwise (S47 idempotent-204; **S49** bare-`stream_id` snapshot keying is deliberate → guard, don't rekey).
5. No silent scope caps; persist verified findings to a ledger so they survive compaction (S48-AUDIT-FINDINGS.md).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-112** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-51; ROADMAP-V2 §2.30 ledger; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-51.md`.
6. **Roll prod forward** if server/web *source* changed.
