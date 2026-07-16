# SESSION-54 — planned at S53 close (D-115)

> Written by SESSION-53 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE for the full ranked candidate list** + `S48-AUDIT-FINDINGS.md`.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-55

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S48 finding is an AGENT finding —
re-verify against the code before building; take the verified CORE, not the audit's literal suggested scope**
(S49 [2] subtler than its summary; S50/S51 narrowed scope). One scope per PR.

## ★ Context — the MEDIUM/LOW batch (8 remain; all 6 HIGH + [7],[15] shipped)

Shipped: [6] D-110, [1]+[2] D-111, [3] D-112, [4]+[15] D-113, [5] D-114, [7] D-115. **8 remain** — the ranked list
with per-finding mechanism + fix + test seam lives in **`RESUME-PROMPT.md` ▶ START HERE** and
`agents/handoffs/S48-AUDIT-FINDINGS.md`. Suggested next: **[9]** restpoller `prevStatus` leak (clean), then
**[10]** egress-method disclosure, **[13]** clickhouse per-item PrepareBatch, **[16]** dup node_stats, **[14]**
beacon 413. **[11]** needs a SQL-text/real-CH test seam (fake conn is vacuous). **[12]** needs a migration (FIVE
places) and **[8]** needs product-viability verification (may be operator/contract-gated) — do those last.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S53 (D-115, PR #103) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-47-gd32b165`** (rollback tag `pre-d115`). `/healthz` all-ok, `ams_env_configured:true`.
- Operator queue: GHCR anon → 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If today ≥ 07-23, promoting `web-e2e`/`csp-e2e`
  off `continue-on-error` is a clean win worth taking before/alongside the audit work.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts`; mutate `/mut`; test there. **Target precisely** — identical sibling text over-matches; replacement ending in `{` breaks perl `{}` delimiters → use `#` (S46/S53). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check** — a data-flow change on a touched line can surface a pre-existing alert (S47). Fix the real weakness.
- **Contract change? `cd web && npm run gen:api`.** **New migration? FIVE places** (0004 last shipped → next 0005; **[12] needs this**). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; tag `pre-dNNN` → backup (rc 0) → STAMPED `build --build-arg …` → assert stamp → `up -d` (no `--build`) → smoke. Roll forward ONLY if server/web *source* changed. Admin token for live smoke in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE (S50/S51). Trace an existing test
   before trusting it (S49 [2]). Some findings ([8], [12]) need product/ops verification before coding.
2. Mutation-prove every guard; positive control so the harness can't be vacuous. For DB-text bugs ([11]) a
   fake-conn returning fixed values is VACUOUS — need a SQL assertion seam or real CH.
3. Independent review before merge; a compact 1–2 lens review OR careful self-review for a purely mechanical
   mutation-proven fix (S48/S50/S51/S52/S53).
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees.
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly.

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-116** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-55; ROADMAP-V2 §2.30 ledger; mark shipped findings ✅ in `S48-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-55.md`.
6. **Roll prod forward** if server/web *source* changed.
