# SESSION-57 — planned at S56 close (D-118)

> Written by SESSION-56 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE for the full ranked candidate list** + `S48-AUDIT-FINDINGS.md`.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-58

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S48 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S49 [2] subtler;
S50/S51 narrowed; S55 [10] BROADENED). One scope per PR.

## ★ Context — the MEDIUM/LOW batch (5 remain; all 6 HIGH + [7],[9],[10],[13],[15] shipped)

Shipped: [6] D-110, [1]+[2] D-111, [3] D-112, [4]+[15] D-113, [5] D-114, [7] D-115, [9] D-116, [10] D-117,
[13] D-118. **5 remain** — the ranked list with per-finding mechanism + fix + test seam lives in
**`RESUME-PROMPT.md` ▶ START HERE** and `agents/handoffs/S48-AUDIT-FINDINGS.md`. Suggested next: **[16]** dup
node_stats (clean; `cluster/discovery.go:145`, `seen` map already exists — add a dedup guard at the top of the poll
loop), then **[14]** beacon 413 (`errors.As(err, &http.MaxBytesError)`). **[11]** needs a SQL-text/real-CH test seam
(fake conn is vacuous). **[12]** needs a migration (FIVE places, next = 0005) and **[8]** needs product-viability
verification (may be operator/contract-gated) — do those last.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S56 (D-118, PR #109) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-53-g500aabb`** (rollback image tag `pre-d118` → `v0.4.0-51-ge5577f7`). `/healthz`
  all-ok, `ams_env_configured:true`.
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` (both green through the bake) is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). Gate: `sh -c 'D=$(gofmt -l .); [ -z "$D" ] && go build ./... && go vet ./... && go test ./... || { echo DIRTY: $D; exit 1; }'`. (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely**; a replacement ending in `{` breaks perl `{}` → use `#` (S46/S53/S54). Prefer mutations that keep compilation valid (RED test, not build error) (S55). For a whole-function revert, an `awk` splice of the original from a mounted file is clean (S56). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** — CI's `web` job has a **types-drift guard**; regenerate with `npm ci --legacy-peer-deps` (lockfile-pinned tool, node 22) so `schema.d.ts` matches CI byte-for-byte (S55). **New migration? FIVE places** (0004 → next 0005; **[12] needs this**). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback point is a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` (NOT a git tag) → backup (`exec -T backup … once`, rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (build takes >2 min → run it backgrounded) → assert stamp ≠ dev → `up -d` (no `--build`) → smoke (healthz, version, signed webhook 200, limits 512M/0.5cpu, logs clean). Roll forward ONLY if server/web *source* changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER (S50/S51) or BROADER
   (S55) than the audit's literal scope. Trace an existing test before trusting it (S49 [2]). [8], [12] need
   product/ops verification before coding.
2. Mutation-prove every guard; positive control so the harness can't be vacuous. For DB-text bugs ([11]) a
   fake-conn returning fixed values is VACUOUS — need a SQL assertion seam or real CH. Prefer compiling mutations
   (RED test) over build-breaking ones (S55); for whole-function reverts, an awk splice of the original is clean (S56).
3. Independent review before merge; a compact 1–2 lens review OR careful self-review for a purely mechanical
   mutation-proven fix (S53/S54/S56). A fix carrying a genuine SEMANTIC/product decision (S55's `mixed`) warrants the
   multi-lens adversarial workflow.
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees;
   when introducing a new API-returned value, document it in the contract (S55).
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly.
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-119** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-58; ROADMAP-V2 §2.30 ledger; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-58.md`.
6. **Roll prod forward** if server/web *source* changed.
