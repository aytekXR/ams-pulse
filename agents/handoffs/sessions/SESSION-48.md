# SESSION-48 — planned at S47 close (D-109)

> Written by SESSION-47 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> Read `RESUME-PROMPT.md` + `ROADMAP-V2.md` §2 + `decisions.md` D-106…D-109 before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-49

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and **choose the
next-highest-leverage move.** Verify candidate status AND product-viability against the code before committing
(S38/S43 overturned their leads; S39–S47 confirmed theirs). **This is now the primary task** — the S44 audit
backlog that drove S44–S47 is exhausted, so SESSION-48 opens WITHOUT a queued work item and must select one.

## ★★ Context — the S44 13-bug audit backlog is CLOSED

S44's 8-finder adversarial audit found 13 confirmed defects; all are shipped: **S44** security cluster (D-106),
**S45** reports-scheduler (D-107), **S46** entitlement + WS-auth (D-108), **S47** audit-integrity + hardening +
a CodeQL CWE-916 (D-109). There is no remaining queued finding. Pick the next track deliberately.

## Candidate tracks for S48 (verify each before committing — do NOT assume)

1. **§2.7 CI-promotion date gate — CHECK THE DATE at open.** `web-e2e` / `csp-e2e` have run green for several
   rounds behind `continue-on-error`. The promotion window opens **≥ 2026-07-23**. If today ≥ 07-23, promoting
   them off `continue-on-error` (so they gate real regressions) is a clean, high-value, low-risk win. If today
   < 07-23, it is still gated — do not force it. This is the single clearest "known win when eligible."
2. **A fresh adversarial audit of an UN-audited subsystem.** The S44 audit targeted the API handler families and
   found 13 real bugs — strong evidence that a rigorous audit of code NOT yet swept finds real work. Un-audited
   surface worth a finder sweep: `internal/collector/*` (beacon/ingest/webhook/restpoller/kafka/sessions/
   aggregator), `pkg/amsclient`, `internal/reports` (statement/PDF/CSV beyond the S44 CSV fix), `internal/cluster`,
   `internal/store/clickhouse`. Run the same pattern (fan-out finders → refute-by-default verifiers → mutation-prove
   only CONFIRMED). ⚠ Verify each finding against the code AND against product-viability before building — several
   S38/S43 leads were deliberate designs, not bugs.
3. **ROADMAP-V2 §2 open items / assessment §5.** Re-read both for any feature or hardening item that is now
   unblocked (operator gates permitting) and higher-leverage than an audit.

> Ordering suggestion: at open, (a) check the CI-promotion date — if eligible, take it (small, clean); then
> (b) re-scan §2 / §5; if nothing higher-leverage, (c) run a fresh adversarial audit of an un-audited subsystem.
> Do NOT bundle unrelated work; one scope per PR.

## ⛔ At open — verify, do not assume (D-095 standing rule)

- `git log --oneline origin/main -4` — S47 (D-109, PR #91) + its docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-35-g56167eb`** (S47 roll-forward; rollback tag `pre-d109`). `/healthz` all-ok,
  `ams_env_configured: true`.
- Operator queue **live**: GHCR anon → 401; **AMS trial-expiry doc discrepancy (07-12 vs 07-27)** — operator-only.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.**

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required** (mounted repo → dubious git ownership). **Go test caching does NOT track the runtime-read OpenAPI spec** — after any `contracts/` edit re-run the api package with `-count=1`.
- **Mutation-prove on a COPY** mounted read-only at `/repo`: `cp -a /repo/server /mut/server && cp -a /repo/contracts /mut/contracts` inside the container (tests resolve the meta DDL at `../../../contracts`); mutate `/mut`; test there. **Target mutations precisely** — identical sibling text over-matches (S45); a replacement ending in `{` unbalances perl `{}` delimiters → use `#`-delimiters (S46). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check** — a data-flow change on a touched line can surface a PRE-EXISTING alert (S47 saw a CWE-916 password-hash alert when a struct-literal line changed). Fix the real weakness; don't game the scanner.
- **Contract change? `cd web && npm run gen:api`** (→ `web/src/lib/api/schema.d.ts`). **New migration? FIVE places** (0004 audit_log last shipped → next 0005). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: tag `pre-dNNN` → backup (rc 0) → STAMPED build (`--build-arg`) → assert stamp ≠ dev → `up -d` WITHOUT `--build` → smoke. Build >2 min → longer Bash timeout. Roll forward ONLY if server/web *source* changed. An admin token for live functional smoke is in gitignored `oguz-testing.md` (never commit; use side-effect-free rejected requests).

## Binding lessons (carry into every wave)

1. Verify product-viability AND candidate-status before building (S38/S43); verify the audit's *mechanism*, not
   just its conclusion (S46 WS route mismatch; S47 1a/1b where the "404" premise contradicted the contract).
2. A gate with no test is not a gate; **mutation-prove every guard/e2e**; audit on the committed-write path before
   any re-fetch (S40). Where a fix genuinely isn't unit-discriminable (S47 finding-2 ordering), say so honestly —
   don't let a test comment overclaim.
3. Independent adversarial review before merge for non-trivial code (S40/S44/S45/S46/S47) — and fix/accept the
   findings it surfaces, including test-accuracy ones.
4. Positive allowlists over blocklists for authz (D-098).
5. No silent scope caps; don't invent scope. Respect the documented contract even when an audit says otherwise
   (S47 idempotent-204).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-110** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-49; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md`.
5. Write `sessions/SESSION-49.md`.
6. **Roll prod forward** if server/web *source* changed.
