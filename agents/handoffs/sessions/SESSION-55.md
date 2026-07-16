# SESSION-55 — planned at S54 close (D-116)

> **✅ CLOSED 2026-07-16 (D-117, PR #107).** Took finding [10] — `reports/accounting.go` `ComputeUsage` returned the
> report-level `egress_method` hardcoded to `bitrate_x_watch_time` even when per-row egress came from AMS byte
> counters, so the F6 CSV/PDF disclosure header lied. Re-verified beyond the audit's literal fix: the daily path can
> be **mixed** (`Totals.EgressGB` blends both), so "any→byte-counter" is just the mirror over-claim. Shipped a 3-way
> report-level disclosure (`bitrate_x_watch_time` / `ams_rest_stats_byte_counter` / new **`mixed`**), tracked across
> the included rows. Full Go suite 24/24; mutation-proven ×3 (incl. a tenant-exclusion regression guard); 3-lens
> review (0 confirmed); prod `v0.4.0-51-ge5577f7`. **6 MEDIUM/LOW findings remain** → SESSION-56. Evidence:
> `decisions.md` D-117. (CI-promotion gate still shut — 07-16 < 07-23.)

> Written by SESSION-54 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE for the full ranked candidate list** + `S48-AUDIT-FINDINGS.md`.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-56

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S48 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope.** One scope per PR.

## ★ Context — the MEDIUM/LOW batch (7 remain; all 6 HIGH + [7],[9],[15] shipped)

Shipped: [6] D-110, [1]+[2] D-111, [3] D-112, [4]+[15] D-113, [5] D-114, [7] D-115, [9] D-116. **7 remain** — the
ranked list with per-finding mechanism + fix + test seam lives in **`RESUME-PROMPT.md` ▶ START HERE** and
`agents/handoffs/S48-AUDIT-FINDINGS.md`. Suggested next: **[10]** reports egress-method disclosure (clean), then
**[13]** clickhouse per-item PrepareBatch, **[16]** dup node_stats, **[14]** beacon 413. **[11]** needs a
SQL-text/real-CH test seam (fake conn is vacuous). **[12]** needs a migration (FIVE places) and **[8]** needs
product-viability verification (may be operator/contract-gated) — do those last.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S54 (D-116, PR #105) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-49-g6d60f53`** (rollback tag `pre-d116`). `/healthz` all-ok, `ams_env_configured:true`.
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate that the local `go build && go vet` misses; a formatting nit fails CI (~30 s) and costs a force-push round-trip (S54). Add it to the gate: `sh -c 'D=$(gofmt -l .); [ -z "$D" ] && go build ./... && go vet ./... && go test ./... || { echo DIRTY: $D; exit 1; }'`. (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely**; replacement ending in `{` breaks perl `{}` delimiters → use `#` (S46/S53/S54). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`.** **New migration? FIVE places** (0004 → next 0005; **[12] needs this**). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED.** **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; tag `pre-dNNN` → backup (rc 0) → STAMPED `build --build-arg …` → assert stamp → `up -d` (no `--build`) → smoke. Roll forward ONLY if server/web *source* changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE (S50/S51). Trace an existing test
   before trusting it (S49 [2]). Some findings ([8], [12]) need product/ops verification before coding.
2. Mutation-prove every guard; positive control so the harness can't be vacuous. For DB-text bugs ([11]) a
   fake-conn returning fixed values is VACUOUS — need a SQL assertion seam or real CH.
3. Independent review before merge; a compact 1–2 lens review OR careful self-review for a purely mechanical
   mutation-proven fix (S48/S50–S54).
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees.
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly.
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-117** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-56; ROADMAP-V2 §2.30 ledger; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-56.md`.
6. **Roll prod forward** if server/web *source* changed.
