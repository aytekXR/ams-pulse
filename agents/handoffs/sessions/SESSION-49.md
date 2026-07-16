# SESSION-49 — planned at S48 close (D-110)

> Written by SESSION-48 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `agents/handoffs/S48-AUDIT-FINDINGS.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-50

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. The S48 audit produced a 16-finding
list; **each is an AGENT finding — re-verify against the code before building** (S38/S43/S46/S47: several "leads"
were deliberate designs or subtler than stated; S48 finding 6 was re-verified true before shipping).

## ★★ Context — working the S48 subsystem-audit backlog (15 findings remain)

The SESSION-48 fresh audit of the un-swept subsystems found **16 CONFIRMED** issues; S48 shipped the top one (the
cross-tenant audience leak, D-110). **15 remain**, all in `agents/handoffs/S48-AUDIT-FINDINGS.md` with fix +
mutation notes. Work them in coherent clusters, one scope per PR, mark each ✅ DONE in that ledger as it ships.

## S49 candidate clusters (verify each against the code first)

1. **HIGH — cross-app StreamID collision (2 findings, ONE root cause).** `collector/dedup.go` `dedupKey` omits
   `App`, and `collector/aggregator/aggregator.go:562` `snapRemoveStream` deletes by bare `StreamID` — so two apps
   with the same bare stream id on one node collide (dropped `publish_start`; surviving stream evicted from the
   snapshot). Coherent cluster; both fixes + mutation tests are described in the ledger. **Strong first pick** —
   verify the two-apps-same-streamID scenario is real for AMS (it is: `app/streamId` is the AMS identity), and that
   the existing `TestRestPoller_MultiApp_NoFalseEnd` / `TestAggregator_CrossAppStreamID_NoCollision` don't already
   cover the drop (the ledger says they don't assert app-B's presence).
2. **HIGH — `amsclient` streamID not URL-path-escaped** (`pkg/amsclient/client.go:475`, WebRTCClientStats). A
   stream id with a slash/space hits the wrong AMS endpoint silently. Verify against `pkg/amsclient` (AMS wire
   formats live here per ARCHITECTURE §3) and check the other escaped/unescaped call sites (the audit refuted the
   `app`/`nodeID` ones as already-safe — confirm why streamID differs).
3. **HIGH — scheduled-report period off-by-one** (`reports/scheduler.go:169`) — first day of the current month
   included in the previous-month report. Verify the date-range math + timezone (finding 15 notes a local-vs-UTC
   `nextCronTime` call at :233 — likely the same file, bundle if related).
4. **HIGH — cluster edge-stream status ignored** (`cluster/discovery.go:264`, `IsEdgeStream`) — a downed edge node
   permanently suppresses origin viewer counts. Verify the node-status model.
5. **MEDIUM/LOW cluster** — webhook replay protection, clickhouse column/precision issues (findings 11–13),
   beacon guards (7, 14), etc. Lower priority; batch where coherent.

> Suggested order: cluster **1** (highest-value, coherent, clear tests), then **2**, then **3/4**. Do NOT bundle
> unrelated subsystems in one PR.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S48 (D-110, PR #93) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-37-g5e822e7`** (rollback tag `pre-d110`). `/healthz` all-ok.
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1` (test cache ignores the runtime-read spec).
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely** — identical sibling text over-matches (S45/S48: use a unique anchor or a Python edit); replacement ending in `{` breaks perl `{}` delimiters → use `#` (S46). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check** — a data-flow change on a touched line can surface a pre-existing alert (S47 CWE-916). Fix the real weakness; don't game the scanner.
- **Contract change? `cd web && npm run gen:api`.** **New migration? FIVE places.** **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED.** **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: tag `pre-dNNN` → backup (rc 0) → STAMPED build → assert stamp → `up -d` (no `--build`) → smoke. Build >2 min → longer Bash timeout. Roll forward ONLY if server/web *source* changed. Admin token for live smoke in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND candidate-status/mechanism before building (S38/S43/S46/S47/S48).
2. Mutation-prove every guard/e2e; drive the real code path with a positive control so the harness can't be vacuous.
3. Independent adversarial review before merge for non-trivial code; for a mechanical fix that mirrors proven
   siblings + is mutation-proven (S48 tenant), a careful self-review can substitute.
4. Positive allowlists over blocklists (D-098). Respect the documented contract even when an audit says otherwise (S47).
5. No silent scope caps; persist verified findings to a ledger so they survive compaction (S48-AUDIT-FINDINGS.md).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-111** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-50; ROADMAP-V2 ledgers; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-50.md`.
6. **Roll prod forward** if server/web *source* changed.
