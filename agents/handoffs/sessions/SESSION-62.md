# SESSION-62 — planned at S61 close (D-123)

> Written by SESSION-61 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE** for the full ranked candidate list.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — the agenda is now yours to set

The S48 audit backlog is CLOSED, so no ledger sets the next goal. Before dispatching: **re-read ROADMAP-V2 §2 /
assessment §5 and choose the next-highest-leverage move.** Verify candidate status AND product-viability against the
code before committing (S37/S48 lesson: named goals go stale; re-verify). One scope per PR. **Take the verified CORE**
— the tail proved this repeatedly (S55 BROADENED; S50/S51/S58 narrowed; S59 [11] + S60 [12] DEFERRED as
dead/vestigial; S61 [8] shipped opt-in after product-viability verification).

## ★★ Milestone — S48 subsystem audit COMPLETE (all 16 findings triaged)

14 SHIPPED: [6] D-110, [1]+[2] D-111, [3] D-112, [4]+[15] D-113, [5] D-114, [7] D-115, [9] D-116, [10] D-117,
[13] D-118, [16] D-119, [14] D-120, [8] D-123. 2 DEFERRED: [11] D-121 (dead-code dup of D-087), [12] D-122
(vestigial `rollup_usage_1d.peak_concurrency`, impact refuted, D-018). Ledger `S48-AUDIT-FINDINGS.md` fully closed.

## ★ The next move — pick one (recommended: a FRESH audit)

1. **A FRESH adversarial audit of an un-swept subsystem (RECOMMENDED).** This has been the highest-yield move every
   time: S44 audited the server handler families → 13 confirmed bugs; S48 audited collector/amsclient/reports/cluster/
   clickhouse → 16 confirmed. **Subsystems NOT yet deep-audited by S44/S48:**
   - `server/internal/api` handler families S44 didn't cover (re-check which — S44's scope is in D-106/§2.29).
   - `server/internal/alert/evaluator.go` + `server/internal/alert/channels/` (notification delivery, rule eval).
   - `server/internal/license` (key validation, entitlement gates — revenue-critical).
   - `server/internal/prober` + probe pipeline (WebRTC/HLS probing, TTFB, segment fetch).
   - `server/internal/anomaly` (Welford baselines, ComputeFlags — note the F9 ClickHouse path is deferred per D-087).
   - **`web/src`** — the SPA data layer / auth / API-typing (S43 was the last web-focused pass).
   - **`sdk/beacon-js`** — the beacon SDK (15 KB gate; last touched pre-S40).
   **How:** run the established workflow — ~7 finders (one per lens/subsystem) + refute-by-default verifiers, persist
   CONFIRMED to a new `agents/handoffs/S62-AUDIT-FINDINGS.md`, then work them one-scope-per-PR (verify-at-open →
   re-verify vs code → fix → mutation-prove → 24/24 → review → PR → CI → merge → prod roll → docs) exactly as
   S49→S61. **Scope the finders to the un-swept list above** so you don't re-audit S44/S48 ground.
2. **§2.7 CI-promotion win — DATE-GATED ≥ 2026-07-23.** CHECK THE DATE at open. If eligible, promote `web-e2e` and
   `csp-e2e` off `continue-on-error` in `.github/workflows/ci.yml` (both have been green through the bake). Small,
   clean, no prod roll. If today ≥ 07-23, do this FIRST (quick win) then start the audit.
3. **Operator-gated items** (GHCR 401, AMS licence expiry 07-12-vs-07-27, item 10 team-mgmt UI, S43 rulings) — remain
   operator-gated; record but do not spin on them.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S61 (D-123, PR #117 — code + docs) should be on `origin/main`.
- Prod should print **`v0.4.0-61-g28812db`** (rolled forward at S61 for the webhook change; rollback image tag
  `pre-d123`). `/healthz` all-ok, `ams_env_configured:true`. **Signed-webhook smoke should still be 200** (the
  replay check is default-off).
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.**
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). Gate: `sh -c 'D=$(gofmt -l .); [ -z "$D" ] && go build ./... && go vet ./... && go test ./... || { echo DIRTY: $D; exit 1; }'`. (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut && cd /mut`; mutate; test there. Prefer compiling mutations (RED test). `perl -0pi -e 's/\Q<literal>\E/<replacement>/'` handles metachars (S61). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** — CI's `web` job has a **types-drift guard**; regenerate with `npm ci --legacy-peer-deps` (node 22) so `schema.d.ts` matches CI byte-for-byte (S55). **New CH migration? lineage is at 0010** (next = 0011; FIVE wiring places; do NOT edit 0001 — S60 catch). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback point is a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` → backup (`exec -T backup … once`, rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (backgrounded, >2 min) → assert stamp ≠ dev → `up -d` (no `--build`) → smoke (healthz, version, **signed webhook 200**, limits 512M/0.5cpu, logs clean). Roll forward ONLY if server/web *source* changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER, BROADER, DEFER (dead/
   vestigial code parked by a prior ADR — S59/S60), or SHIP-OPT-IN after a contract check (S61). Trace an existing
   test before trusting it (S49 [2]). A named goal can be stale — re-verify against the code/ROADMAP (S37/S48).
2. Mutation-prove every guard; positive control so the harness can't be vacuous. Cover BOUNDARY conditions, not just
   far-from-edge cases (S61 review catch — a `<`→`<=` mutation must redden a test).
3. Independent review before merge: a genuine SEMANTIC/auth/contract change warrants the multi-lens adversarial
   workflow (S55, S61); a purely mechanical mutation-proven fix can take a careful self-review (S53/S54/S56/S57/S58).
   The review's job includes finding hardening/robustness gaps, not just blockers — address the real ones, defer the
   YAGNI ones with a reason (S61 per-source override).
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees
   (S59/S60). Migrations forward-only (lineage at 0010; never edit 0001).
5. No silent scope caps; persist verified findings to a ledger; state latency/impact honestly. Backward-compatible,
   default-off is the way to ship a security/contract change without breaking live traffic (S61).
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-124** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-63; ROADMAP-V2 ledger; if a new audit, create `S62-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-63.md`.
6. **Roll prod forward** if server/web *source* changed.
