# SESSION-67 — planned at S66 close (D-128)

> Written by SESSION-66 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-68

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (S66 declined the
ledger's off-by-one and a NIT after re-verifying; the review found a sink the finding didn't name). One coherent
scope per PR.

## ★ Context — the S62 audit backlog (10 shipped, 15 remain — prober subsystem swept)

Shipped: S63 alert-channels; S64 reports_wave2; S65 prober DASH ([3]/[4]); S66 prober RTMP ([13]). **All 6 HIGH done;
the prober subsystem (HLS/DASH/RTMP) is fully swept.** **15 remain: 0 HIGH, 11 MEDIUM, 4 LOW.** Continue in coherent
clusters, one scope per PR.

## ★ SESSION-67 scope: alert-evaluator correctness cluster ([7]+[8]+[9] MEDIUM)

Three correctness bugs in the alert-evaluation loop. **[7] and [8] are in `server/internal/alert/evaluator.go`; [9] is
in `server/internal/alert/license_expiry.go`** — same subsystem, same `processEvaluation` state machine, and the tests
share one harness (`TickOnce` + a notification sink). Bundle as one PR. **⚠ Re-verify each against the code — [8] and
[9] in particular could be intended behavior; take the verified CORE, and if one is actually correct, DEFER it with a
documented reason (the S48/S64 pattern).**

- **[7] MEDIUM `evaluator.go:757` — `evalNodeMetric` missing D-088 presence guards.** It reads `n.CPUPCT`/`MemPCT`/
  `DiskPCT` without checking `CPUPCTReported`/`MemPCTReported`/`DiskPCTReported`. AMS 3.x nodes never emit these →
  they sit at 0.0 with `Reported=false`, so a `node_cpu lt 50` rule fires a FALSE alert every tick. **This is almost
  certainly a real bug** — `evalAnomalyNodes` (wave3.go:281-296) already carries the exact guard with a D-088 comment.
  **Fix:** `if !n.CPUPCTReported { continue }` (and MemPCT/DiskPCT equivalents) before reading each field, mirroring
  `evalAnomalyNodes`. **Mutation/test:** snapshot node with `CPUPCTReported=false, CPUPCT=0`, a `node_cpu lt 50` rule,
  `TickOnce` → assert NO firing notification (reddens if the guard is removed). Positive control: `CPUPCTReported=true,
  CPUPCT=10` → fires.
- **[8] MEDIUM `evaluator.go:713` — `evalStreamOffline` hardcodes `value=0.0` and bypasses `compare`.** It sets the
  evalResult value to literal 0 for both online/offline and sets `ok` directly to `!active` instead of
  `compare(value, rule.Operator, rule.Threshold)`. Effects: the firing notification always shows value 0.0 (should be
  1.0 = offline), and a non-default operator/threshold is ignored. **⚠ RE-VERIFY** the default rule shape and whether
  any rule other than the implied `eq 1` is actually reachable/configurable for stream_offline; if the operator can
  only ever configure the binary form, this may be NARROWER than the audit frames it. **Fix (if confirmed):** compute
  `binaryVal` (0.0 online / 1.0 offline) and use `ok: compare(binaryVal, rule.Operator, rule.Threshold)` like the
  other evaluators — same firing behavior for the standard `eq 1` rule but correct value + honours the operator.
- **[9] MEDIUM `license_expiry.go:40` — `evalLicenseExpiry` returns nil for a perpetual/no-key license.** Returning
  nil means `processEvaluation` never gets a `conditionMet=false` for the `license` groupKey, so a previously-fired
  expiry alert is **stuck in 'firing' forever** (never resolves) after the admin switches to a perpetual license.
  **⚠ RE-VERIFY** the `processEvaluation` resolve path — confirm nil truly skips the key (vs. some other resolve
  trigger). **Fix (if confirmed):** return `[]evalResult{{groupKey: "license", value: math.MaxFloat64, ok: false}}`
  so the key always gets a not-met signal → firing→resolved. **Mutation/test:** fire with `Days:5,HasExpiry:true` →
  firing; swap to `HasExpiry:false`; `TickOnce` → assert a RESOLVED notification + resolved history row (reddens if
  nil is restored).

### Plan
1. **Verify-at-open:** open `evaluator.go` (`evalNodeMetric` ~757, `evalStreamOffline` ~713, `processEvaluation` state
   machine, `compare`) and `license_expiry.go` (`evalLicenseExpiry` ~40). Confirm the `evalAnomalyNodes` guard in
   `wave3.go:281-296` is the reference for [7]. **Trace the existing evaluator test harness first** (`TickOnce` + sink)
   — reuse it (S49). Decide CORE/NARROWER/DEFER per finding.
2. **Fix** the confirmed subset; DEFER any that turn out to be intended behavior with a documented reason.
3. **Test (mutation-proven):** one test per confirmed finding, each with a positive control, using the shared harness.
   [7]: presence-guard suppresses the false alert. [8]: value=1.0 + operator honoured when offline. [9]: firing→resolved
   on perpetual-license swap.
4. **Full Go suite** in docker (24/24, `-buildvcs=false`). The `alert` package is one of the slower ones (~4s).
5. **`gofmt -l .`** before push (CI gofmt gate).
6. **Review:** these are correctness/semantic fixes to alerting logic (operator-facing behavior), not pure mechanical
   guards — a careful **self-review** is defensible if the mutations are clean and the tests cover the state-machine
   transitions; escalate to the adversarial workflow if [8]/[9]'s `compare`/resolve semantics prove subtle.
7. **PR → CI → squash-merge → verify `origin/main`.**
8. **Roll prod forward** (server *source* changed) — full deploy sequence + smoke.
9. **Docs at close:** D-129 in `decisions.md` (append EARLY); mark the shipped findings ✅ (and any DEFERRED with
   reason) in `S62-AUDIT-FINDINGS.md`; ROADMAP-V2 §2.31 (recount shipped/remain); RESUME-PROMPT ▶ START HERE →
   SESSION-68; CHANGELOG Fixed; `operator-expected.md` (note if [7]'s false-alert fix changes what operators see);
   SESSION-67 CLOSED; write SESSION-68.

## ★ Then (subsequent sessions), the remaining MEDIUM/LOW clusters

- **anomaly** — [18] `scopeJSON` builds JSON by raw concatenation without escaping the ID fields (wrong stream
  attribution for IDs containing `"`); hysteresis.
- **license / api** — [21] SSRF probe-URL scheme/host validation; **[20] audit-log admin-scope gate — RE-VERIFY vs
  D-105 "reads-open" ruling FIRST; likely DEFER** (any authenticated user reading the audit log may be the same
  deliberate product decision S43 made — do NOT "fix" it without an operator ruling).
- The 4 LOW findings (see ledger) once the MEDIUMs are cleared.

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S66 (D-128, PR #126 `5a070cc`) + the S66 close-docs PR should be on `origin/main`.
- Prod should print **`v0.4.0-70-g5a070cc`** (S66 shipped code — prober RTMP). `/healthz` all-ok. Signed-webhook smoke
  200 (replay check default-off). Email STARTTLS fail-closed (D-125) — expected.
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If ≥ 07-23, bundle the `web-e2e`/`csp-e2e` promotion.
- Operator queue: GHCR 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). (Memory: `ci-gofmt-gate`.) After a Write-authored test, gofmt may still reformat comment alignment (S66) — run `gofmt -w` on new files.
- **Mutation-prove on a COPY**: `cp -a /repo/server /mut && cd /mut`; mutate; test there. **DDL-backed HTTP harness (`setupEnterpriseServer`) also needs `contracts` copied — else `metaDDLPath` SKIPs (S64).** `alert` tests use in-memory fakes (no DDL). Put `$`-heavy perl in a mounted `.pl` file so bash doesn't interpolate `$` (S65/S66). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** (types-drift guard; node 22, `npm ci --legacy-peer-deps`, S55). **New CH migration? lineage at 0010** (next = 0011; do NOT edit 0001 — S60). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback = a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` → backup (`exec -T backup … once`, rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (backgrounded, >2 min) → assert stamp ≠ dev → `up -d pulse` (no `--build`) → smoke (healthz, version, **signed webhook 200** via `X-Ams-Signature` HMAC, limits 512M/0.5cpu, logs clean). Roll forward ONLY if server/web *source* changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER, BROADER (S65/S66's
   reviews widened scope to a missed sink), DEFER (dead/vestigial, intended behavior, or duplicate-of-a-ruling —
   S59/S60/S64, and **[20] vs D-105**; **re-verify [8]/[9]**), SHIP-OPT-IN after a contract check (S61), or
   DOWNGRADE-SEVERITY (S63/[11]). Trace an existing test before trusting it (S49). An audit claim can be stale,
   overstated, or INCOMPLETE — re-verify.
2. Mutation-prove every guard/fix; positive control so the harness can't be vacuous. Cover BOUNDARY conditions (S61).
   When a metric can't be measured by count, measure the right dimension (S66: bytes via `runtime.MemStats`, not alloc
   count). For state-machine fixes ([9]) assert the actual TRANSITION (firing→resolved), not just the end state.
3. Independent review before merge: a genuine SEMANTIC/security/auth/contract change warrants the multi-lens
   adversarial workflow (S65/S66 — it repeatedly found real sinks the finding didn't name); a purely mechanical,
   mutation-proven fix can take a careful self-review (S64). Alerting-behavior changes are semantic — judge per finding.
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees
   (S59/S60; **[20]**). Migrations forward-only (lineage 0010; never edit 0001).
5. No silent scope caps; persist verified findings to the ledger; state impact honestly (S65/S66 documented declined
   items). Default-off / backward-compatible ships safely (S61).
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-129** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-68; ROADMAP-V2 §2.31 ledger; mark shipped/deferred findings in `S62-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md` (note [7]'s false-alert fix if operator-visible; else no operator action).
5. Write `sessions/SESSION-68.md`.
6. **Roll prod forward** (server source changed) + smoke.
