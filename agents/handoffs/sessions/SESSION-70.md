# SESSION-70 — planned at S69 close (D-131)

> ## ✅ CLOSED (2026-07-16, D-132) — SHIPPED [16] + [17] + [18]
> The anomaly-flag cluster shipped (PR #134, prod `v0.4.0-78-g1076442`): [16] `ComputeFlags` (the `GET /anomalies` read)
> no longer arms the shared hysteresis cooldown, so an on-read poll can't suppress the tick-path `InsertAnomalyFlagEvent`
> (a `setHysteresis bool`, false from the read path; ADR-0009 §4 aligned — the read is now a true point-in-time snapshot);
> [17] a fired flag suppresses exactly `HysteresisTicks` ticks (arm to `+1`; decrement loop extracted to
> `decrementHysteresis()`); [18] scope keys are JSON-escaped (`jsonEscapeStr`, byte-identical for normal IDs) and
> `parseScopeJSON` round-trips via `encoding/json`. 6/6 mutants killed; suite 25/25; 3-lens adversarial review found 3
> CONFIRMED (1 MAJOR `WarmHysteresis` restart off-by-one + a MINOR alert `scopeJSONAnomaly` mirror divergence, both fixed
> pre-merge by arming `WarmHysteresis` to `+1` and exporting the canonical `anomaly.ScopeJSON` for the alert mirror to
> delegate to), 1 refuted. **No operator action.** See `decisions.md` D-132 and `sessions/SESSION-71.md` for the next
> scope (license cluster [12]/[23]/[24], then LOW [22]/[25] to close the audit). Everything below is the original
> pre-session plan (historical).


> Written by SESSION-69 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-71

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S66 declined an
off-by-one; S67 overturned two impls; S68 narrowed [21] from "deny all RFC-1918" to "deny link-local, allow private"
and DEFERRED [20]; S69's review caught a classify regression, fixed pre-merge). Do NOT stop after one session — at
close, update all docs, regenerate this plan, record progress + operator-needs, and continue until the roadmap is
complete or a human/operator is genuinely required. **Ultracode is on** — use the adversarial-review workflow on
security or state-machine/semantics surface; token cost is not a constraint.

## Goal — the anomaly cluster [16] + [17] + [18] (one file, coherent PR)

`S62-AUDIT-FINDINGS.md` after S69: **16 shipped + [20] deferred (17 resolved); 8 remain — MEDIUM [12]/[16]/[17]/[18] +
LOW [22]/[23]/[24]/[25].** Suggested lead: the three anomaly findings, all in
`server/internal/anomaly/anomaly.go` — a coherent single-file PR. [16] and [17] both touch the hysteresis mechanism
(`detectFlagsLocked` / the `UpdateBaselines` decrement loop), so fix them together; [18] is independent (scopeJSON).

### [16] MEDIUM — shared hysteresis map: ComputeFlags (HTTP path) suppresses tick-path flag persistence
- **loc:** `server/internal/anomaly/anomaly.go:541`. `detectFlagsLocked` unconditionally does `d.hysteresis[hk] =
  d.hysteresisTicks` on a new fire, regardless of caller. `ComputeFlags` (HTTP `GET /anomalies`) and `checkFlags`
  (`UpdateBaselines` tick) share the map, so an HTTP poll arms the cooldown and the next tick skips
  `InsertAnomalyFlagEvent` → the ClickHouse flag-events audit trail misses the anomaly.
- **Fix sketch:** add a `setHysteresis bool` param to `detectFlagsLocked`; pass `true` from `checkFlags`, `false` from
  `ComputeFlags`; guard line 541 with `if setHysteresis`. Test: ComputeFlags(anomalous) then one UpdateBaselines(still
  anomalous) must call InsertAnomalyFlagEvent exactly once.
- **⚠ RE-VERIFY (verifier softened this):** the "permanent via 60s polling" scenario is WRONG — repeat polling during
  the cooldown hits `continue` at line 539 and does NOT re-arm. Real impact = transient anomalies (resolve within the
  600s window) + a genuine concurrent-race (ComputeFlags in the decrement→checkFlags gap). Take that CORE; don't
  overstate.

### [17] MEDIUM — off-by-one hysteresis cooldown (decrement-before-detect, rem<=1 delete → N-1 suppressed ticks)
- **loc:** `anomaly.go:336` (delete `if rem <= 1`) + `:541` (initial set). The decrement loop runs before checkFlags
  and deletes the key at rem==1, so only `HysteresisTicks-1` ticks are suppressed vs the documented N.
- **Fix sketch:** set initial `d.hysteresis[hk] = d.hysteresisTicks + 1` (smallest localized change) OR change the
  delete condition to `rem <= 0`. Test: fire a flag, call UpdateBaselines exactly HysteresisTicks times (spike held),
  assert ComputeFlags returns 0 (still suppressed). **Verifier: PRD false-alarm budget still met** — this is a
  doc/contract fix, not a budget violation. Confirm [16] and [17] fixes compose (both touch line 541).

### [18] MEDIUM — scopeJSON unescaped concat → wrong stream attribution for IDs containing `"`
- **loc:** `anomaly.go:677`. `scopeJSON` builds JSON by `'"stream_id":"' + streamID + '"'` with no escaping;
  `extractJSONString` stops at the first raw `"`, so `live"stream` → `live`, mis-attributing anomaly events.
- **Fix sketch:** escape the ID fields (or build the object with `encoding/json.Marshal`) and make `parseScopeJSON`
  round-trip. Table test: for `foo"bar`, `foo\bar`, assert `parseScopeJSON(scopeJSON("","",id)).StreamID == id`.

### Alternatives (if the anomaly cluster is deferred/split)
- **[12] MEDIUM** — `New()` silently discards `activate()` errors (comment claims logging that never happens). Standalone.
- **LOW cluster:** [22] `CertChecker.DaysUntilExpiry` returns 0 (not -1) for an already-expired cert → a `cert_expiry
  lt 0` rule never fires (ledger ~line 362); [23]/[24]/[25].

## Pipeline (unchanged — the S64→S69 loop)

1. **Verify-at-open:** `git log --oneline -3`, HEAD == origin/main, `git status` shows ONLY `deploy/config/Caddyfile.prod`
   dirty (do-not-commit, D-082/D-096). Record **D-132 IN PROGRESS** in `decisions.md` (append EARLY). Branch `s70-d132`.
   **CHECK THE DATE** — §2.7 gate unlocks ≥ 2026-07-23 (today 2026-07-16 → still locked).
2. **Re-verify each finding vs the code** via `mcp__codegraph__codegraph_explore` (take verified CORE). Trace existing
   anomaly tests first (`anomaly_flagstore_test.go`, `anomaly_test.go`).
3. **Fix → mutation-prove** on a throwaway `/mut` copy (anomaly tests likely don't need `contracts/`, but copy it if the
   package's tests read the meta DDL). Value-forcing mutations; `go test -vet=off` for unreachable mutants.
4. **Full Go suite** (25 pkgs) in docker; **`gofmt -l`** on changed files (CI gofmt gate — before EVERY push). No
   contract/web change expected here, so no openapi-typescript regen needed (if that changes, regen
   `web/src/lib/api/schema.d.ts` + redocly lint — see S68/S69).
5. **Adversarial review workflow** (state-machine surface for [16]/[17] → mandatory): finder lenses → refute-by-default
   verifiers. Fix CONFIRMED before merge.
6. **PR → CI poll** (bounded background monitor; expect an occasional `csp-e2e` Playwright flake — re-run the failed job,
   it is unrelated to Go changes) → **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** (server source changed): `config -q` → tag `pulse-prod-pulse:pre-d132` → backup rc0 → STAMPED
   build backgrounded → assert stamp ≠ dev → `up -d pulse` no --build → 5-check smoke.
8. **Close docs:** `decisions.md` D-132 SHIPPED, CHANGELOG, `S62-AUDIT-FINDINGS.md` marks, ROADMAP §2.31 count,
   RESUME-PROMPT rotation, `operator-expected.md` (carry the [20] product call), SESSION-70 CLOSED, SESSION-71 written.
   Re-arm the `/loop` (ScheduleWakeup, `<<autonomous-loop-dynamic>>`).

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` are NOT on the host PATH. Node at `/home/aytek/.local/bin/node` (npx) for web/redocly.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
