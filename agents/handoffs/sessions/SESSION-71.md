# SESSION-71 — planned at S70 close (D-132)

> Written by SESSION-70 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-72

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S66 declined an
off-by-one; S67 overturned two impls; S68 narrowed [21] to "deny link-local, allow private" and DEFERRED [20]; S69's
review caught a classify regression; S70's review caught a `WarmHysteresis` restart off-by-one AND an alert
`scopeJSONAnomaly` scope-key mirror that diverged from the now-escaping builder — both fixed pre-merge). Do NOT stop
after one session — at close, update all docs, regenerate this plan, record progress + operator-needs, and continue
until the roadmap is complete or a human/operator is genuinely required. **Ultracode is on** — use the adversarial-review
workflow on security or state-machine/semantics surface; token cost is not a constraint.

## Goal — the license cluster [12] + [23] (+ [24]), then the last two LOW to close the S62 audit

`S62-AUDIT-FINDINGS.md` after S70: **19 shipped + [20] deferred (20 resolved); 5 remain — MEDIUM [12] + LOW
[22]/[23]/[24]/[25].** Suggested lead: the license findings, most in `server/internal/license/` — a coherent
single-package PR that clears the **last remaining MEDIUM** ([12]). Re-verify each at open (line refs may have drifted;
disposition may be NARROWER/BROADER/DEFER).

### [12] MEDIUM — `New()` silently discards `activate()` errors (comment claims logging that never happens)
- **loc:** `server/internal/license/` (see ledger ~line 166). `New()` calls `activate()` but drops its returned error;
  a code comment claims it is logged, but no logging happens.
- **Re-verify:** what can `activate()` fail on? Does a discarded error leave the license/entitlement state wrong (e.g.
  defaulting to a permissive tier, or a half-initialized verifier)? Take the verified CORE — the fix is likely "log the
  error (and/or fail closed to the free tier)", but confirm the actual failure modes and what the safe default is.
- **Fix sketch:** capture and log the `activate()` error (structured `slog`), and decide fail-open vs fail-closed
  against the entitlement model (a license that fails to activate should NOT silently grant a paid tier). Test: inject
  an `activate()` failure and assert it is logged and the resulting tier is the safe default.

### [23] LOW — unvalidated tier string in `activate()` bypasses `CheckProbes` / `CheckBeaconIngest`
- **loc:** `server/internal/license/` (ledger ~line 423). An unvalidated tier string in `activate()` skips the
  entitlement gate checks. Coherent with [12] (same `activate()` path).
- **Re-verify + fix:** validate the tier against the known set before trusting it for entitlement decisions; an unknown
  tier should map to the safe default, not bypass the checks.

### [24] LOW — wrong error variable wrapped in pubkey-init fallback (`err` instead of `err2`)
- **loc:** `server/internal/license/` (ledger ~line 431). A diagnostic bug: the fallback wraps the wrong error variable,
  so the surfaced error message is misleading. Small, self-contained; fits the license PR.

### Then close the audit — the last two LOW (standalone)
- **[22] LOW** — `CertChecker.DaysUntilExpiry` returns 0 (not -1) for an already-expired cert, so a `cert_expiry lt 0`
  rule never fires (ledger ~line 407). Small; verify the checker + the alert rule path.
- **[25] LOW** — `continueWebRTCICE` does not stop the `time.After(hold)` timer on context cancellation during the stats
  hold, leaking a runtime timer (ledger ~line 448, prober). Standalone.

### Alternatives (if the license cluster is deferred/split)
- Split [12] (the MEDIUM) into its own PR and batch [23]/[24] with it, or take the LOW cluster [22]+[25] first as a
  quick close-out. Either order finishes the S62 audit within 1–2 sessions.

## Pipeline (unchanged — the S64→S70 loop)

1. **Verify-at-open:** `git log --oneline -3`, HEAD == origin/main, `git status` shows ONLY `deploy/config/Caddyfile.prod`
   dirty (do-not-commit, D-082/D-096). Record **D-133 IN PROGRESS** in `decisions.md` (append EARLY). Branch `s71-d133`.
   **CHECK THE DATE** — §2.7 gate unlocks ≥ 2026-07-23 (today 2026-07-16 → still locked).
2. **Re-verify each finding vs the code** via `mcp__codegraph__codegraph_explore` (take verified CORE). Trace existing
   license tests first (`server/internal/license/*_test.go`).
3. **Fix → mutation-prove** on a throwaway copy. NOTE: `/mut` at root is NOT writable this environment — use
   `/tmp/pulsemut` (`rm -rf /tmp/pulsemut && mkdir -p /tmp/pulsemut && cp -a /repo/server /tmp/pulsemut/server`; the
   `preprocessed_configs`/`access` artifact dirs error on cp but are non-fatal — Go source copies fine). Value-forcing
   mutations; `go test -vet=off` for unreachable mutants.
4. **Full Go suite** (25 pkgs) in docker; **`gofmt -l`** on changed files (CI gofmt gate — before EVERY push). No
   contract/web change expected; if `contracts/openapi/pulse-api.yaml` changes, regen `web/src/lib/api/schema.d.ts`.
5. **Adversarial review workflow** (license entitlement = semantics surface → mandatory): finder lenses →
   refute-by-default verifiers. Fix CONFIRMED before merge. (S65–S70 each had review-found in-scope issues.)
6. **PR → CI poll** (bounded background monitor; occasional `csp-e2e`/`e2e` Playwright flake — re-run the failed job) →
   **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** (only if `server/`|`web/` source changed): `config -q` → tag `pulse-prod-pulse:pre-d133` →
   backup rc0 → STAMPED build backgrounded → assert stamp ≠ dev → `up -d pulse` no --build → 5-check smoke.
8. **Close docs:** `decisions.md` D-133 SHIPPED, CHANGELOG, `S62-AUDIT-FINDINGS.md` marks, ROADMAP §2.31 count,
   RESUME-PROMPT rotation, `operator-expected.md` (carry the [20] product call), SESSION-71 CLOSED, SESSION-72 written.
   Re-arm the `/loop` (ScheduleWakeup, `<<autonomous-loop-dynamic>>`).

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` are NOT on the host PATH. Node at `/home/aytek/.local/bin/node` (npx) for web/redocly.
- **Prod deploy is LOCAL** (this host IS prod): 5-overlay compose, `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml
  -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod now at **`v0.4.0-78-g1076442`**.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
